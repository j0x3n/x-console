package notes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func publicNoteRequest(t *testing.T, env *testutil.Env, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, env.URL(path), input)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, raw
}

func noteShare(t *testing.T, env *testutil.Env, id int64, password string) api.NoteShare {
	t.Helper()
	env.Elevate()
	body := map[string]any{"expiresIn": "never"}
	if password != "" {
		body["password"] = password
	}
	var share api.NoteShare
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", id), body, &share)
	return share
}

func TestNoteShareLifecycleAndVisits(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("公开标题"), Body: str("公开正文")})
	path := fmt.Sprintf("/notes/%d/share", n.Id)
	if code, _ := env.Do(http.MethodPut, path, map[string]string{"expiresIn": "never"}, nil); code != 403 {
		t.Fatalf("elevation: %d", code)
	}
	share := noteShare(t, env, n.Id, "")
	if len(share.Token) != 22 || share.HasPassword || share.ExpiresAt != nil || !strings.HasSuffix(share.Url, "/n/"+share.Token) {
		t.Fatalf("share: %+v", share)
	}
	base := "/public/notes/" + share.Token
	for range 2 {
		resp, raw := publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
		var out api.PublicNote
		if err := json.Unmarshal(raw, &out); err != nil || resp.StatusCode != 200 || out.Title != n.Title || out.Body != n.Body || resp.Header.Get("Cache-Control") != "private, no-store" {
			t.Fatalf("read: %d %s %v", resp.StatusCode, raw, err)
		}
	}
	publicNoteRequest(t, env, http.MethodGet, base, nil, map[string]string{"User-Agent": "another browser"})
	var got api.NoteShare
	env.MustDo(http.MethodGet, path, nil, &got)
	if got.Visits != 2 || got.LastVisitAt == nil {
		t.Fatalf("visits: %+v", got)
	}
	var detail api.Note
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &detail)
	p := search(t, env, "q="+url.QueryEscape("公开"))
	if detail.Shared == nil || !*detail.Shared || len(p.Items) != 1 || p.Items[0].Shared == nil || !*p.Items[0].Shared {
		t.Fatalf("shared: %+v %+v", detail, p.Items)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]bool{"archived": true}, nil)
	resp, _ := publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("archived: %d", resp.StatusCode)
	}
	if _, err := env.App.Deps.DB.Exec("UPDATE note_shares SET expires_at=? WHERE note_id=?", time.Now().Add(-time.Minute), n.Id); err != nil {
		t.Fatal(err)
	}
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("expired: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodPut, path, map[string]string{"expiresIn": "7d"}, nil)
	env.MustDo(http.MethodDelete, path, nil, nil)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("revoked: %d", resp.StatusCode)
	}
	if code, _ := env.Do(http.MethodGet, path, nil, nil); code != 404 {
		t.Fatalf("missing share: %d", code)
	}
	share = noteShare(t, env, n.Id, "")
	if share.Token == got.Token {
		t.Fatal("recreated token reused")
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notes/%d", n.Id), nil, nil)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, "/public/notes/"+share.Token, nil, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("deleted: %d", resp.StatusCode)
	}
}

func TestNoteSharePasswordAndFileScope(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("秘密标题")})
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 1000, 800))); err != nil {
		t.Fatal(err)
	}
	_, file := uploadFile(t, env, n.Id, "image.png", pngBytes.Bytes())
	_, unused := uploadFile(t, env, n.Id, "unused.txt", []byte("unused"))
	other := createNote(t, env, api.CreateNote{})
	_, outside := uploadFile(t, env, other.Id, "outside.txt", []byte("outside"))
	body := "![图片](" + file.Url + "?thumb=1)\n[文件](" + file.Url + ")\n[别的笔记](" + outside.Url + ")"
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), api.UpdateNote{Body: &body}, nil)
	share := noteShare(t, env, n.Id, "pass1234")
	base := "/public/notes/" + share.Token
	resp, raw := publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 401 || !strings.Contains(string(raw), "note_password_required") || strings.Contains(string(raw), n.Title) {
		t.Fatalf("protected: %d %s", resp.StatusCode, raw)
	}
	resp, raw = publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "pass1234"}, nil)
	var unlocked struct {
		Access    string
		ExpiresAt time.Time
	}
	if err := json.Unmarshal(raw, &unlocked); err != nil || resp.StatusCode != 200 || time.Until(unlocked.ExpiresAt) < 11*time.Hour {
		t.Fatalf("unlock: %d %s %v", resp.StatusCode, raw, err)
	}
	query := "?t=" + url.QueryEscape(unlocked.Access)
	resp, raw = publicNoteRequest(t, env, http.MethodGet, base+query, nil, nil)
	if resp.StatusCode != 200 || strings.Contains(string(raw), "/api/v1/notes/attachments/") || !strings.Contains(string(raw), "/public/notes/"+share.Token+"/files/") || !strings.Contains(string(raw), "t=") {
		t.Fatalf("rewrite: %d %s", resp.StatusCode, raw)
	}
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base+query+"x", nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("tampered: %d", resp.StatusCode)
	}
	resp, _ = publicNoteRequest(t, env, http.MethodGet, fmt.Sprintf("%s/files/%d", base, file.Id), nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("protected file: %d", resp.StatusCode)
	}
	resp, raw = publicNoteRequest(t, env, http.MethodGet, fmt.Sprintf("%s/files/%d%s", base, file.Id, query), nil, nil)
	if resp.StatusCode != 200 || !bytes.Equal(raw, pngBytes.Bytes()) || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("file: %d %+v", resp.StatusCode, resp.Header)
	}
	resp, raw = publicNoteRequest(t, env, http.MethodGet, fmt.Sprintf("%s/files/%d%s&thumb=1", base, file.Id, query), nil, nil)
	thumb, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil || resp.StatusCode != 200 || thumb.Bounds().Dx() > 480 || thumb.Bounds().Dy() > 480 {
		t.Fatalf("thumb: %d %v", resp.StatusCode, err)
	}
	for _, id := range []int64{unused.Id, outside.Id, file.Id*10 + 1} {
		resp, _ = publicNoteRequest(t, env, http.MethodGet, fmt.Sprintf("%s/files/%d%s", base, id, query), nil, nil)
		if resp.StatusCode != 404 {
			t.Fatalf("scope %d: %d", id, resp.StatusCode)
		}
	}
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", n.Id), map[string]string{"expiresIn": "never", "password": "newpass"}, nil)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base+query, nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("old password access: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", n.Id), map[string]any{"expiresIn": "never", "clearPassword": true}, nil)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("cleared password: %d", resp.StatusCode)
	}
}

func TestNoteShareValidationHiddenAndRateLimit(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{})
	share := noteShare(t, env, n.Id, "pass1234")
	path := fmt.Sprintf("/notes/%d/share", n.Id)
	for _, body := range []map[string]any{{"expiresIn": "bad"}, {"expiresIn": "never", "password": "abc"}, {"expiresIn": "never", "password": strings.Repeat("长", 33)}, {"expiresIn": "never", "password": "valid", "clearPassword": true}} {
		if code, _ := env.Do(http.MethodPut, path, body, nil); code != 400 {
			t.Fatalf("validation: %d %+v", code, body)
		}
	}
	base := "/public/notes/" + share.Token
	for range 5 {
		resp, raw := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "wrong"}, nil)
		if resp.StatusCode != 403 || !strings.Contains(string(raw), "note_password_wrong") {
			t.Fatalf("wrong: %d %s", resp.StatusCode, raw)
		}
	}
	resp, _ := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "pass1234"}, nil)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("minute limit: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]bool{"hidden": true}, nil)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 404 {
		t.Fatalf("hidden: %d", resp.StatusCode)
	}
	if code, _ := env.Do(http.MethodPut, path, map[string]string{"expiresIn": "never"}, nil); code != 400 {
		t.Fatalf("share hidden: %d", code)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]bool{"hidden": false}, nil)
	if code, _ := env.Do(http.MethodGet, path, nil, nil); code != 404 {
		t.Fatalf("hide removes share: %d", code)
	}
}

func TestNoteShareLockoutAccessExpiryAndReadRate(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Body: str("private")})
	share := noteShare(t, env, n.Id, "pass1234")
	mod, ok := module.Lookup[*notes.Module](env.App.Deps.Registry, "notes.module")
	if !ok {
		t.Fatal("notes module missing")
	}
	now := time.Now().UTC()
	mod.SetShareClockForTest(func() time.Time { return now })
	base := "/public/notes/" + share.Token
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt == 6 {
			now = now.Add(time.Minute + time.Second)
		}
		resp, _ := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "wrong"}, nil)
		want := 403
		if attempt == 10 {
			want = 429
		}
		if resp.StatusCode != want {
			t.Fatalf("attempt %d: %d", attempt, resp.StatusCode)
		}
	}
	now = now.Add(14 * time.Minute)
	resp, _ := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "pass1234"}, nil)
	if resp.StatusCode != 429 {
		t.Fatalf("locked 15 minutes: %d", resp.StatusCode)
	}
	now = now.Add(time.Minute + time.Second)
	resp, raw := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "pass1234"}, nil)
	var access struct{ Access string }
	if err := json.Unmarshal(raw, &access); err != nil || resp.StatusCode != 200 {
		t.Fatalf("after lock: %d %s", resp.StatusCode, raw)
	}
	now = now.Add(12 * time.Hour)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base+"?t="+access.Access, nil, nil)
	if resp.StatusCode != 401 {
		t.Fatalf("expired access: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", n.Id), map[string]any{"expiresIn": "never", "clearPassword": true}, nil)
	now = now.Add(time.Minute + time.Second)
	for range 60 {
		resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("read: %d", resp.StatusCode)
		}
	}
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 429 {
		t.Fatalf("read limit: %d", resp.StatusCode)
	}
	now = now.Add(31 * time.Minute)
	resp, _ = publicNoteRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("read after window: %d", resp.StatusCode)
	}
	var got api.NoteShare
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d/share", n.Id), nil, &got)
	if got.Visits != 2 {
		t.Fatalf("visit window: %d", got.Visits)
	}
}
