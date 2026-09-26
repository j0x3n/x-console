package notes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// The notes and projects modules are registered in app/modules.go, so
// testutil.New already builds them; extra constructors here only install fakes.

// fakeModule lets a test install providers into the registry.
type fakeModule struct{}

func (fakeModule) Name() string     { return "fake" }
func (fakeModule) Mount(chi.Router) {}

// provide builds a constructor that registers v under key. A nil v hides any
// real provider, so the feature looks unavailable.
func provide[T any](key string, v T) func(*module.Deps) (module.Module, error) {
	return func(d *module.Deps) (module.Module, error) {
		module.Provide[T](d.Registry, key, v)
		return fakeModule{}, nil
	}
}

type fakeReminders struct {
	mu    sync.Mutex
	calls []contracts.CreateReminder
}

func (f *fakeReminders) Create(_ context.Context, in contracts.CreateReminder) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, in)
	return int64(len(f.calls)), nil
}

func (f *fakeReminders) Upcoming(context.Context, time.Time) ([]contracts.ReminderRef, error) {
	return nil, nil
}

func createNote(t *testing.T, env *testutil.Env, body api.CreateNote) api.Note {
	t.Helper()
	var n api.Note
	env.MustDo(http.MethodPost, "/notes", body, &n)
	return n
}

func str(s string) *string { return &s }

type page struct {
	Items      []api.NoteSummary
	NextCursor string
}

func search(t *testing.T, env *testutil.Env, query string) page {
	t.Helper()
	var p page
	env.MustDo(http.MethodGet, "/notes?"+query, nil, &p)
	return p
}

func titles(p page) string {
	var out []string
	for _, n := range p.Items {
		out = append(out, n.Title)
	}
	return strings.Join(out, ",")
}

func TestNotesCRUD(t *testing.T) {
	env := testutil.New(t)
	ch, cancel := env.App.Deps.Bus.Subscribe("note.", 16)
	defer cancel()
	n := createNote(t, env, api.CreateNote{Title: str(" Shopping "), Body: str("- milk\n- eggs"), Tags: &[]string{"#home", "home", " errands "}})
	if n.Title != "Shopping" || strings.Join(n.Tags, ",") != "errands,home" || n.Pinned {
		t.Fatalf("created: %+v", n)
	}
	if ev := <-ch; ev.Topic != "note.created" {
		t.Fatalf("event: %s", ev.Topic)
	}
	var got api.Note
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &got)
	if got.Body != "- milk\n- eggs" {
		t.Fatalf("get: %+v", got)
	}
	pinned := true
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), api.UpdateNote{Body: str("- milk\n- bread"), Pinned: &pinned, Tags: &[]string{"home"}}, &got)
	if !got.Pinned || got.Body != "- milk\n- bread" || strings.Join(got.Tags, ",") != "home" || !got.UpdatedAt.After(n.UpdatedAt) {
		t.Fatalf("patch: %+v", got)
	}
	if status, _ := env.Do(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), api.UpdateNote{Tags: &[]string{strings.Repeat("长", 41)}}, nil); status != 400 {
		t.Fatalf("long tag: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/notes/999", nil, nil); status != 404 {
		t.Fatalf("missing: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/notes", map[string]any{"unknown": 1}, nil); status != 400 {
		t.Fatalf("unknown field: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notes/%d", n.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notes/%d", n.Id), nil, nil); status != 404 {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestListTagsPinnedArchived(t *testing.T) {
	env := testutil.New(t)
	a := createNote(t, env, api.CreateNote{Title: str("a"), Tags: &[]string{"work"}})
	createNote(t, env, api.CreateNote{Title: str("b"), Tags: &[]string{"work", "idea"}})
	createNote(t, env, api.CreateNote{Title: str("c"), Pinned: ptr(true)})
	d := createNote(t, env, api.CreateNote{Title: str("d"), Tags: &[]string{"old"}})
	archived := true
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", d.Id), api.UpdateNote{Archived: &archived}, nil)

	// Pinned first, then newest first; archived notes are hidden.
	if got := titles(search(t, env, "")); got != "c,b,a" {
		t.Fatalf("list: %s", got)
	}
	if got := titles(search(t, env, "tag=work")); got != "b,a" {
		t.Fatalf("tag: %s", got)
	}
	if got := titles(search(t, env, "pinned=true")); got != "c" {
		t.Fatalf("pinned: %s", got)
	}
	if got := titles(search(t, env, "archived=true")); got != "d" {
		t.Fatalf("archived: %s", got)
	}
	var tags []api.TagCount
	env.MustDo(http.MethodGet, "/notes/tags", nil, &tags)
	if len(tags) != 2 || tags[0].Tag != "work" || tags[0].Count != 2 || tags[1].Tag != "idea" {
		t.Fatalf("tags: %+v", tags)
	}
	p := search(t, env, "limit=2")
	if titles(p) != "c,b" || p.NextCursor == "" {
		t.Fatalf("page 1: %s %q", titles(p), p.NextCursor)
	}
	p = search(t, env, "limit=2&cursor="+p.NextCursor)
	if titles(p) != "a" || p.NextCursor != "" {
		t.Fatalf("page 2: %s %q", titles(p), p.NextCursor)
	}
	_ = a
}

func ptr[T any](v T) *T { return &v }

func TestSearch(t *testing.T) {
	env := testutil.New(t)
	db := createNote(t, env, api.CreateNote{Title: str("学习笔记"), Body: str("今天学习了数据库索引的原理，还看了 PostgreSQL 的文档。")})
	createNote(t, env, api.CreateNote{Title: str("旅行"), Body: str("下个月去杭州，看看西湖。")})
	createNote(t, env, api.CreateNote{Title: str("100% done"), Body: str("under_score and percent")})

	cases := map[string]string{
		"数据库":            "学习笔记", // three Chinese characters: FTS trigram
		"索引":             "学习笔记", // two characters: LIKE
		"西":              "旅行",   // one character: LIKE
		"postgresql":     "学习笔记", // case-insensitive
		"数据库 PostgreSQL": "学习笔记", // several terms must all match
		"杭州 数据库":         "",     // no note has both
		"100%":           "100% done",
		"_":              "100% done", // LIKE wildcards are escaped
		"笔记":             "学习笔记",      // title match
	}
	for q, want := range cases {
		if got := titles(search(t, env, "q="+url.QueryEscape(q))); got != want {
			t.Errorf("q=%q: got %q want %q", q, got, want)
		}
	}
	// Snippets mark the matched text.
	p := search(t, env, "q="+url.QueryEscape("数据库"))
	if p.Items[0].Snippet == nil || !strings.Contains(*p.Items[0].Snippet, "数据库") {
		t.Fatalf("fts snippet: %+v", p.Items[0].Snippet)
	}
	p = search(t, env, "q="+url.QueryEscape("索引"))
	if p.Items[0].Snippet == nil || !strings.Contains(*p.Items[0].Snippet, "索引") {
		t.Fatalf("like snippet: %+v", p.Items[0].Snippet)
	}
	// The index follows edits and deletes.
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", db.Id), api.UpdateNote{Body: str("改成讲缓存策略了")}, nil)
	if got := titles(search(t, env, "q="+url.QueryEscape("数据库"))); got != "" {
		t.Fatalf("old text still found: %s", got)
	}
	if got := titles(search(t, env, "q="+url.QueryEscape("缓存策略"))); got != "学习笔记" {
		t.Fatalf("new text: %s", got)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notes/%d", db.Id), nil, nil)
	if got := titles(search(t, env, "q="+url.QueryEscape("缓存策略"))); got != "" {
		t.Fatalf("deleted note found: %s", got)
	}
	// Quotes in the query cannot break the FTS syntax.
	search(t, env, "q="+url.QueryEscape(`"abc" OR`))
}

func TestToIssue(t *testing.T) {
	env := testutil.New(t)
	var project struct{ Id int64 }
	env.MustDo(http.MethodPost, "/projects", map[string]string{"key": "XC", "name": "X Console"}, &project)
	n := createNote(t, env, api.CreateNote{Title: str("Fix search"), Body: str("Short queries are slow.")})
	var out struct {
		IssueKey string
		IssueUrl string
		Note     api.Note
	}
	env.MustDo(http.MethodPost, fmt.Sprintf("/notes/%d/to-issue", n.Id), map[string]int64{"projectId": project.Id}, &out)
	if out.IssueKey != "XC-1" || out.IssueUrl != "/projects/XC/1" || !strings.Contains(out.Note.Body, "[XC-1 Fix search](/projects/XC/1)") {
		t.Fatalf("to-issue: %+v", out)
	}
	var links []struct{ Kind, Url string }
	env.MustDo(http.MethodGet, "/issues/XC-1/links", nil, &links)
	if len(links) != 1 || links[0].Kind != "note" || links[0].Url != fmt.Sprintf("/notes/%d", n.Id) {
		t.Fatalf("issue links: %+v", links)
	}
	if status, _ := env.Do(http.MethodPost, "/notes/999/to-issue", map[string]int64{"projectId": project.Id}, nil); status != 404 {
		t.Fatalf("missing note: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/notes/%d/to-issue", n.Id), map[string]int64{"projectId": 42}, nil); status != 404 {
		t.Fatalf("missing project: %d", status)
	}

	// Without the projects module the endpoint answers 501.
	env = testutil.New(t, provide[contracts.Issues](contracts.IssuesKey, nil))
	n = createNote(t, env, api.CreateNote{Body: str("x")})
	status, raw := env.Do(http.MethodPost, fmt.Sprintf("/notes/%d/to-issue", n.Id), map[string]int64{"projectId": 1}, nil)
	if status != 501 || !strings.Contains(string(raw), "feature_unavailable") {
		t.Fatalf("no issues: %d %s", status, raw)
	}
}

func TestToReminder(t *testing.T) {
	fake := &fakeReminders{}
	env := testutil.New(t, provide[contracts.Reminders](contracts.RemindersKey, fake))
	n := createNote(t, env, api.CreateNote{Body: str("# Call the bank\nAsk about the card.")})
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	var out struct{ ReminderId int64 }
	env.MustDo(http.MethodPost, fmt.Sprintf("/notes/%d/to-reminder", n.Id), map[string]any{"at": at, "rrule": "FREQ=DAILY"}, &out)
	if out.ReminderId != 1 || len(fake.calls) != 1 {
		t.Fatalf("to-reminder: %+v %+v", out, fake.calls)
	}
	c := fake.calls[0]
	if c.Title != "Call the bank" || !c.At.Equal(at) || c.RRule != "FREQ=DAILY" || c.Link != fmt.Sprintf("/notes/%d", n.Id) {
		t.Fatalf("reminder input: %+v", c)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/notes/%d/to-reminder", n.Id), map[string]any{"rrule": "x"}, nil); status != 400 {
		t.Fatalf("missing at: %d", status)
	}

	env = testutil.New(t, provide[contracts.Reminders](contracts.RemindersKey, nil))
	n = createNote(t, env, api.CreateNote{Body: str("x")})
	status, raw := env.Do(http.MethodPost, fmt.Sprintf("/notes/%d/to-reminder", n.Id), map[string]any{"at": at}, nil)
	if status != 501 || !strings.Contains(string(raw), "feature_unavailable") {
		t.Fatalf("no reminders: %d %s", status, raw)
	}
}

func TestContractAndActions(t *testing.T) {
	env := testutil.New(t)
	ctx := context.Background()
	notes, ok := module.Lookup[contracts.Notes](env.App.Deps.Registry, contracts.NotesKey)
	if !ok {
		t.Fatal("contracts.Notes not provided")
	}
	id, err := notes.Create(ctx, "Brief", "morning summary", []string{"auto"})
	if err != nil || id == 0 {
		t.Fatalf("contract create: %d %v", id, err)
	}
	run := func(name, input string) any {
		t.Helper()
		out, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}
	created := run("notes.create", `{"body":"想法：做一个番茄钟","tags":["idea"]}`).(api.Note)
	appended := run("notes.append", fmt.Sprintf(`{"id":%d,"text":"再加一条"}`, created.Id)).(api.Note)
	if appended.Body != "想法：做一个番茄钟\n\n再加一条" {
		t.Fatalf("append: %q", appended.Body)
	}
	found := run("notes.search", `{"q":"番茄钟"}`).([]api.NoteSummary)
	if len(found) != 1 || found[0].Id != created.Id || found[0].Snippet == nil || !strings.Contains(*found[0].Snippet, "[番茄钟]") {
		t.Fatalf("search: %+v", found)
	}
	if list := run("notes.search", `{"tag":"auto"}`).([]api.NoteSummary); len(list) != 1 || list[0].Id != id {
		t.Fatalf("search by tag: %+v", list)
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "notes.append", json.RawMessage(`{"id":999,"text":"x"}`)); err == nil {
		t.Fatal("append to missing note should fail")
	}
	if _, err := env.App.Deps.Actions.Run(ctx, "notes.create", json.RawMessage(`{"body":"  "}`)); err == nil {
		t.Fatal("empty note should fail")
	}
}
