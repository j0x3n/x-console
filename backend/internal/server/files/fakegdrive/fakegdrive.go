// Package fakegdrive is a small in-memory Google Drive and Google OAuth server
// for tests. It knows the requests files.GDrive sends and nothing more.
package fakegdrive

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

// File is one stored file or folder.
type File struct {
	ID       string
	Name     string
	MimeType string
	Parent   string
	Data     []byte
	Modified time.Time
}

// Server is the fake. ClientID, Secret and Code are what it accepts;
// RefreshToken is what it hands out for Code.
type Server struct {
	*httptest.Server
	ClientID     string
	Secret       string
	Code         string
	RefreshToken string
	Email        string

	mu       sync.Mutex
	files    map[string]*File
	sessions map[string]*session
	next     int
	clock    time.Time
	revoked  []string
	expired  bool
	// Scope is what the token answer says was granted.
	Scope string
}

type session struct {
	name, parent string
	data         []byte
}

// New starts a fake with one authorized client.
func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{ClientID: "client", Secret: "secret", Code: "code", RefreshToken: "refresh", Email: "me@gmail.com",
		files: map[string]*File{}, sessions: map[string]*session{}, clock: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Scope: files.GoogleDriveScope + " " + files.GoogleBrowseScope}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Endpoints points files.GDrive at the fake.
func (s *Server) Endpoints() files.GoogleEndpoints {
	return files.GoogleEndpoints{Auth: s.URL + "/o/oauth2/v2/auth", Token: s.URL + "/token", Revoke: s.URL + "/revoke", API: s.URL}
}

// Expire makes the refresh token stop working, as Google does after 7 days
// for apps in testing.
func (s *Server) Expire() {
	s.mu.Lock()
	s.expired = true
	s.mu.Unlock()
}

// Revoked returns the tokens sent to the revoke endpoint.
func (s *Server) Revoked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.revoked...)
}

// Names returns the names of the files in the folder called folder, sorted.
func (s *Server) Names(folder string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, f := range s.files {
		if p := s.files[f.Parent]; p != nil && p.Name == folder {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Folders returns the names of the folders in the root.
func (s *Server) Folders() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, f := range s.files {
		if f.MimeType == "application/vnd.google-apps.folder" && f.Parent == "root" {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Sessions returns the number of unfinished upload sessions.
func (s *Server) Sessions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

func fail(w http.ResponseWriter, code int, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": code, "message": reason,
		"errors": []map[string]string{{"reason": reason}}}})
}

func (s *Server) id() string {
	s.next++
	return fmt.Sprintf("id%d", s.next)
}

func (s *Server) tick() time.Time {
	s.clock = s.clock.Add(time.Second)
	return s.clock
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.URL.Path == "/token":
		s.token(w, r)
		return
	case r.URL.Path == "/revoke":
		r.ParseForm()
		s.revoked = append(s.revoked, r.PostForm.Get("token"))
		return
	}
	if r.Header.Get("Authorization") != "Bearer access-"+s.RefreshToken {
		fail(w, http.StatusUnauthorized, "authError")
		return
	}
	p := r.URL.Path
	switch {
	case p == "/drive/v3/about":
		json.NewEncoder(w).Encode(map[string]any{"user": map[string]string{"emailAddress": s.Email}})
	case p == "/drive/v3/files" && r.Method == http.MethodGet:
		s.list(w, r)
	case p == "/drive/v3/files" && r.Method == http.MethodPost:
		var in struct {
			Name     string   `json:"name"`
			MimeType string   `json:"mimeType"`
			Parents  []string `json:"parents"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f := &File{ID: s.id(), Name: in.Name, MimeType: in.MimeType, Parent: in.Parents[0], Modified: s.tick()}
		s.files[f.ID] = f
		json.NewEncoder(w).Encode(map[string]string{"id": f.ID})
	case p == "/upload/drive/v3/files" && r.Method == http.MethodPost && r.URL.Query().Get("uploadType") == "resumable":
		var in struct {
			Name    string   `json:"name"`
			Parents []string `json:"parents"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		id := s.id()
		s.sessions[id] = &session{name: in.Name, parent: in.Parents[0]}
		w.Header().Set("Location", s.URL+"/upload/session/"+id)
	case strings.HasPrefix(p, "/upload/session/"):
		s.chunk(w, r, strings.TrimPrefix(p, "/upload/session/"))
	case strings.HasPrefix(p, "/drive/v3/files/") && strings.HasSuffix(p, "/copy"):
		src := s.files[strings.TrimSuffix(strings.TrimPrefix(p, "/drive/v3/files/"), "/copy")]
		if src == nil {
			fail(w, http.StatusNotFound, "notFound")
			return
		}
		var in struct {
			Name    string   `json:"name"`
			Parents []string `json:"parents"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f := &File{ID: s.id(), Name: in.Name, Parent: in.Parents[0], Data: append([]byte(nil), src.Data...), Modified: s.tick()}
		s.files[f.ID] = f
		json.NewEncoder(w).Encode(map[string]string{"id": f.ID})
	case p == "/drive/v3/files/root":
		json.NewEncoder(w).Encode(map[string]any{"id": "root", "name": "我的云端硬盘", "mimeType": "application/vnd.google-apps.folder"})
	case strings.HasPrefix(p, "/drive/v3/files/"):
		f := s.files[strings.TrimPrefix(p, "/drive/v3/files/")]
		if f == nil {
			fail(w, http.StatusNotFound, "notFound")
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("alt") == "media" {
				http.ServeContent(w, r, f.Name, f.Modified, strings.NewReader(string(f.Data)))
				return
			}
			json.NewEncoder(w).Encode(s.item(f))
		case http.MethodDelete:
			delete(s.files, f.ID)
			w.WriteHeader(http.StatusNoContent)
		}
	default:
		fail(w, http.StatusNotFound, "notFound")
	}
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f := r.PostForm
	w.Header().Set("Content-Type", "application/json")
	if f.Get("client_id") != s.ClientID || f.Get("client_secret") != s.Secret {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"invalid_client"}`)
		return
	}
	switch f.Get("grant_type") {
	case "authorization_code":
		if f.Get("code") != s.Code {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"invalid_grant"}`)
			return
		}
		s.expired = false
		json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + s.RefreshToken, "refresh_token": s.RefreshToken, "expires_in": 3600, "scope": s.Scope})
	case "refresh_token":
		if s.expired || f.Get("refresh_token") != s.RefreshToken {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + s.RefreshToken, "expires_in": 3600})
	default:
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, `{"error":"unsupported_grant_type"}`)
	}
}

var clause = regexp.MustCompile(`^(?:name = '((?:[^'\\]|\\.)*)'|mimeType = '([^']*)'|'((?:[^'\\]|\\.)*)' in parents|trashed = false)$`)

func unquote(s string) string { return strings.NewReplacer(`\'`, `'`, `\\`, `\`).Replace(s) }

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	var name, mime, parent *string
	for _, c := range strings.Split(r.URL.Query().Get("q"), " and ") {
		m := clause.FindStringSubmatch(c)
		if m == nil {
			fail(w, http.StatusBadRequest, "invalidQuery: "+c)
			return
		}
		switch {
		case strings.HasPrefix(c, "name"):
			v := unquote(m[1])
			name = &v
		case strings.HasPrefix(c, "mimeType"):
			mime = &m[2]
		case strings.HasSuffix(c, "in parents"):
			v := unquote(m[3])
			parent = &v
		}
	}
	var out []map[string]any
	for _, f := range s.files {
		if (name != nil && f.Name != *name) || (mime != nil && f.MimeType != *mime) || (parent != nil && f.Parent != *parent) {
			continue
		}
		out = append(out, s.item(f))
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	// Two per page, so paging is tested.
	start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	end := min(start+2, len(out))
	answer := map[string]any{"files": out[start:end]}
	if end < len(out) {
		answer["nextPageToken"] = strconv.Itoa(end)
	}
	json.NewEncoder(w).Encode(answer)
}

func (s *Server) item(f *File) map[string]any {
	item := map[string]any{"id": f.ID, "name": f.Name, "modifiedTime": f.Modified.Format(time.RFC3339Nano), "parents": []string{f.Parent}}
	if f.MimeType != "" {
		item["mimeType"] = f.MimeType
	} else {
		item["mimeType"] = "application/octet-stream"
		item["size"] = strconv.Itoa(len(f.Data))
	}
	return item
}

// Add puts a file in a folder ("root" for the top) and returns its id. An
// empty mime makes a plain file; FolderMime makes a folder.
func (s *Server) Add(parent, name, mime string, data []byte) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := &File{ID: s.id(), Name: name, MimeType: mime, Parent: parent, Data: data, Modified: s.tick()}
	s.files[f.ID] = f
	return f.ID
}

// FolderMime is the mime type of a Drive folder.
const FolderMime = "application/vnd.google-apps.folder"

var contentRange = regexp.MustCompile(`^bytes (?:(\d+)-(\d+)|\*)/(\d+|\*)$`)

func (s *Server) chunk(w http.ResponseWriter, r *http.Request, id string) {
	sess := s.sessions[id]
	if sess == nil {
		fail(w, http.StatusNotFound, "notFound")
		return
	}
	if r.Method == http.MethodDelete {
		delete(s.sessions, id)
		w.WriteHeader(499)
		return
	}
	m := contentRange.FindStringSubmatch(r.Header.Get("Content-Range"))
	if m == nil {
		fail(w, http.StatusBadRequest, "badContentRange")
		return
	}
	data, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, http.StatusBadRequest, "badBody")
		return
	}
	if m[1] != "" {
		if from, _ := strconv.Atoi(m[1]); from != len(sess.data) {
			fail(w, http.StatusBadRequest, "wrongOffset")
			return
		}
	}
	sess.data = append(sess.data, data...)
	if m[3] == "*" {
		w.Header().Set("Range", fmt.Sprintf("bytes=0-%d", len(sess.data)-1))
		w.WriteHeader(http.StatusPermanentRedirect)
		return
	}
	if total, _ := strconv.Atoi(m[3]); total != len(sess.data) {
		fail(w, http.StatusBadRequest, "wrongTotal")
		return
	}
	delete(s.sessions, id)
	f := &File{ID: s.id(), Name: sess.name, Parent: sess.parent, Data: sess.data, Modified: s.tick()}
	s.files[f.ID] = f
	json.NewEncoder(w).Encode(map[string]string{"id": f.ID})
}

// AuthURL parses an authorization address and returns its query.
func AuthURL(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
