package files

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// GoogleEndpoints are the Google addresses GDrive talks to. Tests point them
// at a fake; the zero value means the real ones.
type GoogleEndpoints struct {
	Auth   string // https://accounts.google.com/o/oauth2/v2/auth
	Token  string // https://oauth2.googleapis.com/token
	Revoke string // https://oauth2.googleapis.com/revoke
	API    string // https://www.googleapis.com
	Client *http.Client
}

func (e GoogleEndpoints) withDefaults() GoogleEndpoints {
	if e.Auth == "" {
		e.Auth = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if e.Token == "" {
		e.Token = "https://oauth2.googleapis.com/token"
	}
	if e.Revoke == "" {
		e.Revoke = "https://oauth2.googleapis.com/revoke"
	}
	if e.API == "" {
		e.API = "https://www.googleapis.com"
	}
	e.API = strings.TrimRight(e.API, "/")
	if e.Client == nil {
		dial := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		e.Client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dial,
			TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 60 * time.Second, MaxIdleConnsPerHost: 4}}
	}
	// Google answers an unfinished upload chunk with 308 and no Location.
	c := *e.Client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	e.Client = &c
	return e
}

// GoogleDriveScope only lets the app see the files it made itself.
const GoogleDriveScope = "https://www.googleapis.com/auth/drive.file"

// GoogleBrowseScope lets the drive page list and download every file (B68).
const GoogleBrowseScope = "https://www.googleapis.com/auth/drive.readonly"

// ErrGDriveAuth means the refresh token no longer works: it expired or was
// revoked, and the user has to authorize again.
var ErrGDriveAuth = errors.New("Google Drive 授权过期，请重新授权")

// GoogleAuthURL is the Google page that asks the user to let the app use
// Drive. Google sends the browser back to redirect with code and state.
func GoogleAuthURL(e GoogleEndpoints, clientID, redirect, state string) string {
	e = e.withDefaults()
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {GoogleDriveScope + " " + GoogleBrowseScope}, "state": {state}, "access_type": {"offline"}, "prompt": {"consent"}}
	return e.Auth + "?" + q.Encode()
}

type tokenAnswer struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func postToken(ctx context.Context, e GoogleEndpoints, form url.Values) (tokenAnswer, error) {
	var out tokenAnswer
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.Client.Do(req)
	if err != nil {
		return out, explainNet(err)
	}
	defer drain(resp)
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil && resp.StatusCode/100 == 2 {
		return out, fmt.Errorf("Google 返回的令牌读不懂：%w", err)
	}
	switch {
	case out.Error == "invalid_grant":
		return out, ErrGDriveAuth
	case out.Error == "invalid_client" || out.Error == "unauthorized_client":
		return out, errors.New("Google 客户端 ID 或密钥不对")
	case out.Error != "":
		if out.Description != "" {
			return out, fmt.Errorf("Google 授权失败：%s（%s）", out.Error, out.Description)
		}
		return out, fmt.Errorf("Google 授权失败：%s", out.Error)
	case resp.StatusCode/100 != 2 || out.AccessToken == "":
		return out, fmt.Errorf("Google 授权失败：%d", resp.StatusCode)
	}
	return out, nil
}

// GoogleGrant is what the user allowed.
type GoogleGrant struct {
	RefreshToken string
	Scopes       []string
}

// CanBrowse tells whether the grant lets the app read every file.
func (g GoogleGrant) CanBrowse() bool {
	return slices.Contains(g.Scopes, GoogleBrowseScope) || slices.Contains(g.Scopes, "https://www.googleapis.com/auth/drive")
}

// GoogleExchange trades the code from the callback for a refresh token.
func GoogleExchange(ctx context.Context, e GoogleEndpoints, clientID, secret, code, redirect string) (GoogleGrant, error) {
	e = e.withDefaults()
	out, err := postToken(ctx, e, url.Values{"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {clientID}, "client_secret": {secret}, "redirect_uri": {redirect}})
	if errors.Is(err, ErrGDriveAuth) {
		return GoogleGrant{}, errors.New("授权码无效或已过期，请重新授权")
	}
	if err != nil {
		return GoogleGrant{}, err
	}
	if out.RefreshToken == "" {
		return GoogleGrant{}, errors.New("Google 没有给出长期令牌，请在 Google 账号里移除这个应用的授权后重试")
	}
	return GoogleGrant{RefreshToken: out.RefreshToken, Scopes: strings.Fields(out.Scope)}, nil
}

// GoogleRevoke tells Google to forget a token. A token Google does not know
// any more is not an error.
func GoogleRevoke(ctx context.Context, e GoogleEndpoints, token string) error {
	e = e.withDefaults()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Revoke, strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := e.Client.Do(req)
	if err != nil {
		return explainNet(err)
	}
	drain(resp)
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("Google 撤销授权失败：%d", resp.StatusCode)
	}
	return nil
}

// GDriveConfig is what it takes to reach the backup folder in Google Drive.
type GDriveConfig struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	FolderID     string // the folder, or "" to find or make it by FolderName
	FolderName   string // a folder in the root of the drive
	Endpoints    GoogleEndpoints
	ChunkSize    int // upload chunk, a multiple of 256 KiB; 0 means 8 MiB
}

// GDrive keeps files in one Google Drive folder. The folder is flat: a key is
// the name of a file in it, "/" and all. Drive allows several files with the
// same name; the newest one counts, and Put removes the older ones after the
// new file is complete.
type GDrive struct {
	c GDriveConfig
	e GoogleEndpoints

	mu       sync.Mutex
	access   string
	expiry   time.Time
	folderID string
}

var _ Store = (*GDrive)(nil)

const folderMime = "application/vnd.google-apps.folder"

// NewGDrive builds a client. It does not connect yet; call Check.
func NewGDrive(c GDriveConfig) (*GDrive, error) {
	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, errors.New("Google Drive 的客户端 ID 和密钥没有填")
	}
	if c.RefreshToken == "" {
		return nil, errors.New("Google Drive 还没授权")
	}
	if c.FolderID == "" && strings.TrimSpace(c.FolderName) == "" {
		return nil, errors.New("Google Drive 的文件夹名没有填")
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = 8 << 20
	}
	return &GDrive{c: c, e: c.Endpoints.withDefaults(), folderID: c.FolderID}, nil
}

func (g *GDrive) token(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.access != "" && time.Now().Before(g.expiry) {
		return g.access, nil
	}
	out, err := postToken(ctx, g.e, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {g.c.RefreshToken},
		"client_id": {g.c.ClientID}, "client_secret": {g.c.ClientSecret}})
	if err != nil {
		return "", err
	}
	g.access = out.AccessToken
	g.expiry = time.Now().Add(time.Duration(max(out.ExpiresIn-60, 30)) * time.Second)
	return g.access, nil
}

// driveError is an error answer of the Drive API.
type driveError struct {
	Status  int
	Reason  string
	Message string
}

func (e *driveError) Error() string {
	switch {
	case e.Reason == "storageQuotaExceeded":
		return "Google Drive 空间不够"
	case e.Reason == "accessNotConfigured" || e.Reason == "SERVICE_DISABLED":
		return "Google Cloud 项目里还没启用 Google Drive API"
	case e.Status == http.StatusUnauthorized:
		return ErrGDriveAuth.Error()
	case e.Message != "":
		return fmt.Sprintf("Google Drive：%s", e.Message)
	}
	return fmt.Sprintf("Google Drive 请求失败：%d", e.Status)
}

func (e *driveError) Is(target error) bool {
	return target == ErrGDriveAuth && e.Status == http.StatusUnauthorized
}

func readDriveError(resp *http.Response) error {
	defer drain(resp)
	var body struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
	e := &driveError{Status: resp.StatusCode, Message: body.Error.Message}
	if len(body.Error.Errors) > 0 {
		e.Reason = body.Error.Errors[0].Reason
	} else if len(body.Error.Details) > 0 {
		e.Reason = body.Error.Details[0].Reason
	}
	return e
}

// call sends one request to Google with the access token.
func (g *GDrive) call(ctx context.Context, method, target string, body io.Reader, size int64, header map[string]string) (*http.Response, error) {
	tok, err := g.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if body != nil && size >= 0 {
		req.ContentLength = size
		if size == 0 {
			req.Body = http.NoBody
		}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := g.e.Client.Do(req)
	if err != nil {
		return nil, explainNet(err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		g.mu.Lock()
		g.access = ""
		g.mu.Unlock()
	}
	return resp, nil
}

// callJSON sends a request with an optional JSON body and decodes the answer
// into out.
func (g *GDrive) callJSON(ctx context.Context, method, target string, in, out any) error {
	var body io.Reader
	size := int64(-1)
	header := map[string]string{}
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, size = bytes.NewReader(raw), int64(len(raw))
		header["Content-Type"] = "application/json; charset=UTF-8"
	}
	resp, err := g.call(ctx, method, target, body, size, header)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return readDriveError(resp)
	}
	defer drain(resp)
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
}

// driveFile is the part of a Drive file this package reads.
type driveFile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	MimeType     string    `json:"mimeType"`
	Size         int64     `json:"size,string"`
	ModifiedTime time.Time `json:"modifiedTime"`
}

func (f driveFile) info() Info { return Info{Key: f.Name, Size: f.Size, ModTime: f.ModifiedTime} }

// quote writes s as a string in a Drive search query.
func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// search returns the files matching q, newest first.
func (g *GDrive) search(ctx context.Context, q string) ([]driveFile, error) {
	var all []driveFile
	page := ""
	for {
		v := url.Values{"q": {q}, "spaces": {"drive"}, "pageSize": {"1000"},
			"fields": {"nextPageToken,files(id,name,mimeType,size,modifiedTime)"}}
		if page != "" {
			v.Set("pageToken", page)
		}
		var out struct {
			NextPageToken string      `json:"nextPageToken"`
			Files         []driveFile `json:"files"`
		}
		if err := g.callJSON(ctx, http.MethodGet, g.e.API+"/drive/v3/files?"+v.Encode(), nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Files...)
		if out.NextPageToken == "" {
			break
		}
		page = out.NextPageToken
	}
	slices.SortStableFunc(all, func(a, b driveFile) int { return b.ModifiedTime.Compare(a.ModifiedTime) })
	return all, nil
}

// Folder returns the id of the backup folder, making the folder when it is
// not there yet.
func (g *GDrive) Folder(ctx context.Context) (string, error) {
	g.mu.Lock()
	id := g.folderID
	g.mu.Unlock()
	if id != "" {
		return id, nil
	}
	name := strings.TrimSpace(g.c.FolderName)
	found, err := g.search(ctx, "name = "+quote(name)+" and mimeType = "+quote(folderMime)+" and 'root' in parents and trashed = false")
	if err != nil {
		return "", err
	}
	if len(found) > 0 {
		id = found[len(found)-1].ID // the oldest, so it stays the same one
	} else {
		var made driveFile
		err := g.callJSON(ctx, http.MethodPost, g.e.API+"/drive/v3/files?fields=id",
			map[string]any{"name": name, "mimeType": folderMime, "parents": []string{"root"}}, &made)
		if err != nil {
			return "", err
		}
		id = made.ID
	}
	g.mu.Lock()
	g.folderID = id
	g.mu.Unlock()
	return id, nil
}

// byName returns the files called key in the folder, newest first.
func (g *GDrive) byName(ctx context.Context, key string) ([]driveFile, error) {
	if err := CheckKey(key); err != nil {
		return nil, err
	}
	folder, err := g.Folder(ctx)
	if err != nil {
		return nil, err
	}
	return g.search(ctx, "name = "+quote(key)+" and "+quote(folder)+" in parents and trashed = false")
}

func (g *GDrive) newest(ctx context.Context, key string) (driveFile, error) {
	found, err := g.byName(ctx, key)
	if err != nil {
		return driveFile{}, err
	}
	for _, f := range found {
		if f.MimeType != folderMime {
			return f, nil
		}
	}
	return driveFile{}, ErrNotFound
}

// removeOthers deletes the files called key except keep.
func (g *GDrive) removeOthers(ctx context.Context, key, keep string) error {
	found, err := g.byName(ctx, key)
	if err != nil {
		return err
	}
	for _, f := range found {
		if f.ID != keep && f.MimeType != folderMime {
			if err := g.deleteID(ctx, f.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *GDrive) deleteID(ctx context.Context, id string) error {
	resp, err := g.call(ctx, http.MethodDelete, g.e.API+"/drive/v3/files/"+url.PathEscape(id), nil, -1, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusNotFound {
		drain(resp)
		return nil
	}
	return readDriveError(resp)
}

// Put uploads with a resumable upload session, one chunk at a time. Drive
// only makes the file when the last chunk arrives, so a failed upload leaves
// nothing behind.
func (g *GDrive) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	folder, err := g.Folder(ctx)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"name": key, "parents": []string{folder}})
	header := map[string]string{"Content-Type": "application/json; charset=UTF-8", "X-Upload-Content-Type": "application/octet-stream"}
	if size >= 0 {
		header["X-Upload-Content-Length"] = fmt.Sprint(size)
	}
	resp, err := g.call(ctx, http.MethodPost, g.e.API+"/upload/drive/v3/files?uploadType=resumable&fields=id", bytes.NewReader(raw), int64(len(raw)), header)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return readDriveError(resp)
	}
	drain(resp)
	session := resp.Header.Get("Location")
	if session == "" {
		return errors.New("Google Drive 没有返回上传地址")
	}
	id, err := g.upload(ctx, session, r, size)
	if err != nil {
		// Drop the session so Google does not keep the parts.
		if resp, derr := g.call(context.WithoutCancel(ctx), http.MethodDelete, session, nil, -1, nil); derr == nil {
			drain(resp)
		}
		return err
	}
	return g.removeOthers(ctx, key, id)
}

func (g *GDrive) upload(ctx context.Context, session string, r io.Reader, size int64) (string, error) {
	br := bufio.NewReader(&counter{r: r, size: size})
	buf := make([]byte, g.c.ChunkSize)
	var off int64
	for {
		n, err := io.ReadFull(br, buf)
		final := false
		switch {
		case err == io.EOF || err == io.ErrUnexpectedEOF:
			final = true
		case err != nil:
			return "", err
		default:
			if _, err := br.Peek(1); err == io.EOF {
				final = true
			} else if err != nil {
				return "", err
			}
		}
		var rng string
		switch {
		case !final:
			rng = fmt.Sprintf("bytes %d-%d/*", off, off+int64(n)-1)
		case n > 0:
			rng = fmt.Sprintf("bytes %d-%d/%d", off, off+int64(n)-1, off+int64(n))
		default:
			rng = fmt.Sprintf("bytes */%d", off)
		}
		resp, err := g.call(ctx, http.MethodPut, session, bytes.NewReader(buf[:n]), int64(n), map[string]string{"Content-Range": rng})
		if err != nil {
			return "", err
		}
		off += int64(n)
		if !final {
			if resp.StatusCode != http.StatusPermanentRedirect {
				if resp.StatusCode/100 == 2 {
					drain(resp)
					return "", errors.New("Google Drive 提前结束了上传")
				}
				return "", readDriveError(resp)
			}
			drain(resp)
			continue
		}
		if resp.StatusCode/100 != 2 {
			return "", readDriveError(resp)
		}
		var made driveFile
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&made)
		drain(resp)
		if err != nil || made.ID == "" {
			return "", errors.New("Google Drive 上传完成，但没有返回文件")
		}
		return made.ID, nil
	}
}

func (g *GDrive) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	return g.GetRange(ctx, key, 0, -1)
}

func (g *GDrive) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	f, err := g.newest(ctx, key)
	if err != nil {
		return nil, Info{}, err
	}
	info := f.info()
	if offset < 0 {
		offset = 0
	}
	if offset >= info.Size || length == 0 {
		return io.NopCloser(strings.NewReader("")), info, nil
	}
	end := info.Size
	if length > 0 {
		end = min(offset+length, info.Size)
	}
	header := map[string]string{}
	if offset > 0 || end < info.Size {
		header["Range"] = fmt.Sprintf("bytes=%d-%d", offset, end-1)
	}
	resp, err := g.call(ctx, http.MethodGet, g.e.API+"/drive/v3/files/"+url.PathEscape(f.ID)+"?alt=media", nil, -1, header)
	if err != nil {
		return nil, Info{}, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		return resp.Body, info, nil
	case http.StatusOK:
		if offset == 0 && end == info.Size {
			return resp.Body, info, nil
		}
		if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
			resp.Body.Close()
			return nil, Info{}, err
		}
		return readCloser{io.LimitReader(resp.Body, end-offset), resp.Body}, info, nil
	case http.StatusNotFound:
		drain(resp)
		return nil, Info{}, ErrNotFound
	}
	return nil, Info{}, readDriveError(resp)
}

func (g *GDrive) Stat(ctx context.Context, key string) (Info, error) {
	f, err := g.newest(ctx, key)
	if err != nil {
		return Info{}, err
	}
	return f.info(), nil
}

func (g *GDrive) Delete(ctx context.Context, key string) error {
	return g.removeOthers(ctx, key, "")
}

func (g *GDrive) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return failed(err)
	}
	return func(yield func(Info, error) bool) {
		folder, err := g.Folder(ctx)
		if err != nil {
			yield(Info{}, err)
			return
		}
		all, err := g.search(ctx, quote(folder)+" in parents and trashed = false")
		if err != nil {
			yield(Info{}, err)
			return
		}
		seen := map[string]bool{}
		var out []Info
		for _, f := range all { // newest first, so the first of a name wins
			if f.MimeType == folderMime || seen[f.Name] || CheckKey(f.Name) != nil {
				continue
			}
			if prefix != "" && !strings.HasPrefix(f.Name, prefix+"/") {
				continue
			}
			seen[f.Name] = true
			out = append(out, f.info())
		}
		slices.SortFunc(out, func(a, b Info) int {
			return slices.Compare(strings.Split(a.Key, "/"), strings.Split(b.Key, "/"))
		})
		for _, info := range out {
			if !yield(info, nil) {
				return
			}
		}
	}
}

func (g *GDrive) Copy(ctx context.Context, from, to string) error {
	if err := CheckKey(to); err != nil {
		return err
	}
	src, err := g.newest(ctx, from)
	if err != nil {
		return err
	}
	folder, err := g.Folder(ctx)
	if err != nil {
		return err
	}
	var made driveFile
	err = g.callJSON(ctx, http.MethodPost, g.e.API+"/drive/v3/files/"+url.PathEscape(src.ID)+"/copy?fields=id",
		map[string]any{"name": to, "parents": []string{folder}}, &made)
	if err != nil {
		return err
	}
	return g.removeOthers(ctx, to, made.ID)
}

// Account returns the email address of the Google account.
func (g *GDrive) Account(ctx context.Context) (string, error) {
	var out struct {
		User struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	if err := g.callJSON(ctx, http.MethodGet, g.e.API+"/drive/v3/about?fields=user(emailAddress)", nil, &out); err != nil {
		return "", err
	}
	return out.User.EmailAddress, nil
}

// Check finds or makes the folder, writes, reads and deletes a small file, and
// returns a short Chinese message that says what is wrong.
func (g *GDrive) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := g.Folder(ctx); err != nil {
		return explainNet(err)
	}
	return probe(ctx, g, explainNet)
}

// DirEntry is one item of a folder listing on the drive page (B68).
type DirEntry struct {
	Ref      string // WebDAV: the path; Google Drive: the file id
	Name     string
	Dir      bool
	Size     int64
	ModTime  time.Time
	MimeType string
}

// Downloadable is false for Google Docs and other online-only files.
func (e DirEntry) Downloadable() bool {
	return !e.Dir && !strings.HasPrefix(e.MimeType, "application/vnd.google-apps.")
}

func sortEntries(list []DirEntry) {
	slices.SortStableFunc(list, func(a, b DirEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

const fileFields = "id,name,mimeType,size,modifiedTime,parents"

type driveMeta struct {
	driveFile
	Parents []string `json:"parents"`
}

func (f driveMeta) entry() DirEntry {
	return DirEntry{Ref: f.ID, Name: f.Name, Dir: f.MimeType == folderMime, Size: f.Size, ModTime: f.ModifiedTime, MimeType: f.MimeType}
}

func (g *GDrive) meta(ctx context.Context, id string) (driveMeta, error) {
	var out driveMeta
	err := g.callJSON(ctx, http.MethodGet, g.e.API+"/drive/v3/files/"+url.PathEscape(id)+"?fields="+fileFields, nil, &out)
	var derr *driveError
	if errors.As(err, &derr) && derr.Status == http.StatusNotFound {
		return out, ErrNotFound
	}
	return out, err
}

// ReadDir lists a folder of the whole drive. "" is the root.
func (g *GDrive) ReadDir(ctx context.Context, folder string) ([]DirEntry, error) {
	if folder == "" {
		folder = "root"
	}
	if strings.ContainsAny(folder, `'\`) {
		return nil, ErrBadKey
	}
	var all []DirEntry
	page := ""
	for {
		v := url.Values{"q": {quote(folder) + " in parents and trashed = false"}, "spaces": {"drive"}, "pageSize": {"1000"},
			"fields": {"nextPageToken,files(" + fileFields + ")"}}
		if page != "" {
			v.Set("pageToken", page)
		}
		var out struct {
			NextPageToken string      `json:"nextPageToken"`
			Files         []driveMeta `json:"files"`
		}
		if err := g.callJSON(ctx, http.MethodGet, g.e.API+"/drive/v3/files?"+v.Encode(), nil, &out); err != nil {
			return nil, err
		}
		for _, f := range out.Files {
			all = append(all, f.entry())
		}
		if out.NextPageToken == "" {
			break
		}
		page = out.NextPageToken
	}
	sortEntries(all)
	return all, nil
}

// Trail returns the folders from below the root down to folder, for the
// path above a listing.
func (g *GDrive) Trail(ctx context.Context, folder string) ([]DirEntry, error) {
	if folder == "" || folder == "root" {
		return nil, nil
	}
	root, err := g.meta(ctx, "root")
	if err != nil {
		return nil, err
	}
	var out []DirEntry
	for id := folder; id != "" && id != root.ID && len(out) < 32; {
		f, err := g.meta(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append([]DirEntry{f.entry()}, out...)
		id = ""
		if len(f.Parents) > 0 {
			id = f.Parents[0]
		}
	}
	return out, nil
}

// OpenFile downloads a file of the whole drive by id.
func (g *GDrive) OpenFile(ctx context.Context, id string) (io.ReadCloser, DirEntry, error) {
	f, err := g.meta(ctx, id)
	if err != nil {
		return nil, DirEntry{}, err
	}
	e := f.entry()
	if !e.Downloadable() {
		return nil, e, errors.New("Google 文档这类在线文件不能直接下载，请在 Google Drive 里打开")
	}
	resp, err := g.call(ctx, http.MethodGet, g.e.API+"/drive/v3/files/"+url.PathEscape(id)+"?alt=media", nil, -1, nil)
	if err != nil {
		return nil, e, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, e, readDriveError(resp)
	}
	return resp.Body, e, nil
}
