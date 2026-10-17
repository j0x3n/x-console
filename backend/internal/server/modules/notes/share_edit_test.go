package notes_test

import (
	"encoding/json"
	"fmt"
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

// editShare opens the share link of a note with editing on.
func editShare(t *testing.T, env *testutil.Env, id int64, password string, allow bool) api.NoteShare {
	t.Helper()
	env.Elevate()
	body := map[string]any{"expiresIn": "never", "allowEdit": allow}
	if password != "" {
		body["password"] = password
	}
	var share api.NoteShare
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", id), body, &share)
	return share
}

func publicRead(t *testing.T, env *testutil.Env, token, access string) api.PublicNote {
	t.Helper()
	path := "/public/notes/" + token
	if access != "" {
		path += "?t=" + url.QueryEscape(access)
	}
	resp, raw := publicNoteRequest(t, env, http.MethodGet, path, nil, nil)
	var out api.PublicNote
	if err := json.Unmarshal(raw, &out); err != nil || resp.StatusCode != 200 {
		t.Fatalf("public read: %d %s", resp.StatusCode, raw)
	}
	return out
}

func publicEdit(t *testing.T, env *testutil.Env, token, access string, in api.PublicNoteEdit) (int, []byte) {
	t.Helper()
	path := "/public/notes/" + token
	if access != "" {
		path += "?t=" + url.QueryEscape(access)
	}
	resp, raw := publicNoteRequest(t, env, http.MethodPut, path, in, nil)
	return resp.StatusCode, raw
}

func errorCode(raw []byte) string {
	var out struct{ Code string }
	_ = json.Unmarshal(raw, &out)
	return out.Code
}

func sharedEditNotifications(t *testing.T, env *testutil.Env) []struct{ Kind, Title, Body, Link string } {
	t.Helper()
	var out struct {
		Items []struct{ Kind, Title, Body, Link string }
	}
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var items []struct{ Kind, Title, Body, Link string }
	for _, item := range out.Items {
		if item.Kind == "note.shared_edit" {
			items = append(items, item)
		}
	}
	return items
}

func TestNoteShareEditSettingNeedsElevation(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("标题")})
	path := fmt.Sprintf("/notes/%d/share", n.Id)
	if code, _ := env.Do(http.MethodPut, path, map[string]any{"expiresIn": "never", "allowEdit": true}, nil); code != 403 {
		t.Fatalf("no elevation: %d", code)
	}
	share := editShare(t, env, n.Id, "", true)
	if share.AllowEdit == nil || !*share.AllowEdit {
		t.Fatalf("allowEdit not set: %+v", share)
	}
	// Not sending allowEdit keeps it.
	var again api.NoteShare
	env.MustDo(http.MethodPut, path, map[string]any{"expiresIn": "7d"}, &again)
	if again.AllowEdit == nil || !*again.AllowEdit || again.Token != share.Token {
		t.Fatalf("kept: %+v", again)
	}
	var got api.NoteShare
	env.MustDo(http.MethodGet, path, nil, &got)
	if got.AllowEdit == nil || !*got.AllowEdit {
		t.Fatalf("get: %+v", got)
	}
	if public := publicRead(t, env, share.Token, ""); public.CanEdit == nil || !*public.CanEdit {
		t.Fatalf("canEdit: %+v", public)
	}
	off := editShare(t, env, n.Id, "", false)
	if off.AllowEdit == nil || *off.AllowEdit {
		t.Fatalf("off: %+v", off)
	}
	if public := publicRead(t, env, share.Token, ""); public.CanEdit == nil || *public.CanEdit {
		t.Fatalf("canEdit off: %+v", public)
	}
	fresh := createNote(t, env, api.CreateNote{})
	env.Elevate()
	var plain api.NoteShare
	env.MustDo(http.MethodPut, fmt.Sprintf("/notes/%d/share", fresh.Id), map[string]any{"expiresIn": "never"}, &plain)
	if plain.AllowEdit == nil || *plain.AllowEdit {
		t.Fatalf("default: %+v", plain)
	}
}

func TestNoteShareEditDisabledPasswordAndConflict(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("旧标题"), Body: str("旧正文")})
	share := editShare(t, env, n.Id, "", false)
	base := "/public/notes/" + share.Token
	opened := publicRead(t, env, share.Token, "")
	edit := api.PublicNoteEdit{Title: "新标题", Body: "新正文 独特词汇蓝鲸", BaseUpdatedAt: opened.UpdatedAt}
	if code, raw := publicEdit(t, env, share.Token, "", edit); code != 403 || errorCode(raw) != "note_edit_disabled" {
		t.Fatalf("disabled: %d %s", code, raw)
	}

	share = editShare(t, env, n.Id, "pass1234", true)
	if code, raw := publicEdit(t, env, share.Token, "", edit); code != 401 {
		t.Fatalf("no access: %d %s", code, raw)
	}
	_, raw := publicNoteRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"password": "pass1234"}, nil)
	var unlocked struct{ Access string }
	if err := json.Unmarshal(raw, &unlocked); err != nil || unlocked.Access == "" {
		t.Fatalf("unlock: %s %v", raw, err)
	}
	opened = publicRead(t, env, share.Token, unlocked.Access)
	edit.BaseUpdatedAt = opened.UpdatedAt

	// A stale baseUpdatedAt is refused and the note stays as it is.
	stale := edit
	stale.BaseUpdatedAt = opened.UpdatedAt.Add(-time.Second)
	if code, raw := publicEdit(t, env, share.Token, unlocked.Access, stale); code != 409 || errorCode(raw) != "note_changed" {
		t.Fatalf("stale: %d %s", code, raw)
	}
	var current api.Note
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &current)
	if current.Title != "旧标题" || current.Body != "旧正文" || !current.UpdatedAt.Equal(opened.UpdatedAt) {
		t.Fatalf("changed after 409: %+v", current)
	}

	// The name is limited to 20 characters.
	long := edit
	long.EditorName = str(strings.Repeat("字", 21))
	if code, _ := publicEdit(t, env, share.Token, unlocked.Access, long); code != 400 {
		t.Fatalf("long name: %d", code)
	}

	// Nanoseconds do not matter: the API works in milliseconds.
	rough := edit
	rough.BaseUpdatedAt = opened.UpdatedAt.Truncate(time.Millisecond)
	code, raw := publicEdit(t, env, share.Token, unlocked.Access, rough)
	var saved api.PublicNote
	if err := json.Unmarshal(raw, &saved); err != nil || code != 200 || saved.Title != "新标题" || saved.Body != "新正文 独特词汇蓝鲸" || saved.CanEdit == nil || !*saved.CanEdit || !saved.UpdatedAt.After(opened.UpdatedAt) {
		t.Fatalf("saved: %d %s %v", code, raw, err)
	}

	// The old base is stale now.
	if code, _ := publicEdit(t, env, share.Token, unlocked.Access, rough); code != 409 {
		t.Fatalf("second save with old base: %d", code)
	}
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &current)
	if current.Title != "新标题" || current.Body != "新正文 独特词汇蓝鲸" {
		t.Fatalf("own read: %+v", current)
	}
	if got := titles(search(t, env, "q="+url.QueryEscape("独特词汇蓝鲸"))); got != "新标题" {
		t.Fatalf("search: %q", got)
	}
	if got := titles(search(t, env, "q="+url.QueryEscape("旧正文"))); got != "" {
		t.Fatalf("old text still found: %q", got)
	}
	// Saving is not a visit: only the two reads above count.
	var share2 api.NoteShare
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d/share", n.Id), nil, &share2)
	if share2.Visits != 2 {
		t.Fatalf("visits: %d", share2.Visits)
	}
}

func TestNoteShareEditClosedAndHidden(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("标题"), Body: str("正文")})
	share := editShare(t, env, n.Id, "", true)
	opened := publicRead(t, env, share.Token, "")
	// Switched off after the editor was opened.
	editShare(t, env, n.Id, "", false)
	if code, raw := publicEdit(t, env, share.Token, "", api.PublicNoteEdit{Title: "改", Body: "改", BaseUpdatedAt: opened.UpdatedAt}); code != 403 {
		t.Fatalf("closed: %d %s", code, raw)
	}
	if code, _ := publicEdit(t, env, "nosuchtoken0000000000", "", api.PublicNoteEdit{Title: "改", Body: "改", BaseUpdatedAt: opened.UpdatedAt}); code != 404 {
		t.Fatalf("unknown token: %d", code)
	}
}

func TestNoteShareEditKeepsAttachmentAddresses(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Title: str("图")})
	_, file := uploadFile(t, env, n.Id, "a.txt", []byte("abc"))
	body := "![图](" + file.Url + "?thumb=1)\n[文件](" + file.Url + ")\n结尾"
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), api.UpdateNote{Body: &body}, nil)
	share := editShare(t, env, n.Id, "", true)
	opened := publicRead(t, env, share.Token, "")
	if strings.Contains(opened.Body, "/api/v1/notes/attachments/") || !strings.Contains(opened.Body, "/api/v1/public/notes/"+share.Token+"/files/") {
		t.Fatalf("public body: %q", opened.Body)
	}
	// Send the public text back, with a word added.
	edit := api.PublicNoteEdit{Title: "图", Body: opened.Body + "\n访客加的一行", BaseUpdatedAt: opened.UpdatedAt, EditorName: str("  小王 ")}
	code, raw := publicEdit(t, env, share.Token, "", edit)
	var saved api.PublicNote
	if err := json.Unmarshal(raw, &saved); err != nil || code != 200 {
		t.Fatalf("save: %d %s", code, raw)
	}
	if saved.Body != edit.Body {
		t.Fatalf("response body: %q", saved.Body)
	}
	var current api.Note
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &current)
	if current.Body != body+"\n访客加的一行" {
		t.Fatalf("own body: %q", current.Body)
	}
	var audits int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM audit_log WHERE action='note.share.edit' AND target=? AND result='ok' AND detail LIKE '%小王%' AND detail LIKE '%"before"%' AND detail LIKE '%"ip"%'`, fmt.Sprint(n.Id)).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit: %d %v", audits, err)
	}
}

func TestNoteShareEditNotifiesOncePerTenMinutes(t *testing.T) {
	env := testutil.New(t)
	mod, ok := module.Lookup[*notes.Module](env.App.Deps.Registry, "notes.module")
	if !ok {
		t.Fatal("notes module missing")
	}
	now := time.Now().UTC()
	mod.SetShareClockForTest(func() time.Time { return now })
	n := createNote(t, env, api.CreateNote{Title: str("会议记录"), Body: str("a")})
	share := editShare(t, env, n.Id, "", true)
	save := func(body, name string) {
		t.Helper()
		now = now.Add(time.Minute) // keeps the read limit away
		opened := publicRead(t, env, share.Token, "")
		in := api.PublicNoteEdit{Title: "会议记录", Body: body, BaseUpdatedAt: opened.UpdatedAt}
		if name != "" {
			in.EditorName = &name
		}
		if code, raw := publicEdit(t, env, share.Token, "", in); code != 200 {
			t.Fatalf("save: %d %s", code, raw)
		}
	}
	events, cancel := env.App.Deps.Bus.Subscribe("note.updated", 8)
	defer cancel()
	save("b", "")
	save("c", "小王")
	list := sharedEditNotifications(t, env)
	if len(list) != 1 || list[0].Body != "访客改了笔记《会议记录》" || list[0].Title != "笔记被访客修改" || list[0].Link != fmt.Sprintf("/notes/%d", n.Id) {
		t.Fatalf("after two saves: %+v", list)
	}
	select {
	case event := <-events:
		raw, _ := json.Marshal(event.Data)
		if !strings.Contains(string(raw), `"source":"share"`) {
			t.Fatalf("event: %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("no note.updated event")
	}
	now = now.Add(10 * time.Minute)
	save("d", "小王")
	list = sharedEditNotifications(t, env)
	if len(list) != 2 {
		t.Fatalf("after 10 minutes: %+v", list)
	}
	found := false
	for _, item := range list {
		found = found || item.Body == "小王改了笔记《会议记录》"
	}
	if !found {
		t.Fatalf("named notification missing: %+v", list)
	}
}

func TestRestoreNoteAttachmentsRoundTrip(t *testing.T) {
	const token = "tok_en-ABC123"
	cases := []string{
		"没有附件",
		"![a](/api/v1/notes/attachments/3)",
		"![a](/api/v1/notes/attachments/3?thumb=1) 和 [f](/api/v1/notes/attachments/12) 以及 <img src=\"/api/v1/notes/attachments/7?thumb=1\">",
		"![a](/api/v1/notes/attachments/3?thumb=1)\n![b](/api/v1/notes/attachments/3)\n",
		"[x](/api/v1/notes/attachments/9?foo=bar)",
	}
	for _, original := range cases {
		for _, access := range []string{"", "1893456000.abc-_DEF"} {
			public := notes.RewriteNoteAttachments(original, token, access)
			if got := notes.RestoreNoteAttachments(public, token, original); got != original {
				t.Errorf("access %q:\n original %q\n public   %q\n restored %q", access, original, public, got)
			}
		}
	}
	// Without the original text the canonical form is used.
	public := notes.RewriteNoteAttachments("![a](/api/v1/notes/attachments/3?thumb=1) [f](/api/v1/notes/attachments/4)", token, "1.x")
	if got := notes.RestoreNoteAttachments(public, token, ""); got != "![a](/api/v1/notes/attachments/3?thumb=1) [f](/api/v1/notes/attachments/4)" {
		t.Errorf("canonical: %q", got)
	}
	// Other tokens are left alone; a new file the visitor pasted is restored.
	other := "![x](/api/v1/public/notes/othertoken/files/5?t=abc)"
	mixed := other + " ![y](/api/v1/public/notes/" + token + "/files/6?thumb=1&t=zzz)"
	want := other + " ![y](/api/v1/notes/attachments/6?thumb=1)"
	if got := notes.RestoreNoteAttachments(mixed, token, ""); got != want {
		t.Errorf("mixed: %q", got)
	}
	// A token that only starts like ours is not ours.
	prefix := "/api/v1/public/notes/" + token + "x/files/5"
	if got := notes.RestoreNoteAttachments(prefix, token, ""); got != prefix {
		t.Errorf("prefix token: %q", got)
	}
}
