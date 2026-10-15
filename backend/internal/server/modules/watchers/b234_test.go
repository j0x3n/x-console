package watchers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// ---- helpers that write the tables the way a run would ----

type seedStep struct {
	Text   string `json:"text"`
	ShotID *int64 `json:"shotId,omitempty"`
}

func (f *fixture) exec(t *testing.T, q string, args ...any) sql.Result {
	t.Helper()
	res, err := f.env.App.Deps.DB.Exec(q, args...)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return res
}

func (f *fixture) seedShot(t *testing.T, wid int64, size int, at time.Time) int64 {
	t.Helper()
	res := f.exec(t, `INSERT INTO watcher_shots (watcher_id, data, created_at) VALUES (?, ?, ?)`, wid, make([]byte, size), at)
	id, _ := res.LastInsertId()
	return id
}

func (f *fixture) seedRun(t *testing.T, wid int64, outcome string, started time.Time, shots ...int64) int64 {
	t.Helper()
	var steps []seedStep
	for i, s := range shots {
		s := s
		steps = append(steps, seedStep{Text: fmt.Sprintf("第 %d 步", i+1), ShotID: &s})
	}
	if steps == nil {
		steps = []seedStep{}
	}
	raw, _ := json.Marshal(steps)
	var finished *time.Time
	if outcome != "running" {
		fin := started.Add(time.Minute)
		finished = &fin
	}
	res := f.exec(t, `INSERT INTO watcher_runs (watcher_id, kind, outcome, steps, started_at, finished_at) VALUES (?, 'check', ?, ?, ?, ?)`,
		wid, outcome, string(raw), started, finished)
	id, _ := res.LastInsertId()
	return id
}

func (f *fixture) seedChange(t *testing.T, wid int64, summary string, shot *int64, at time.Time) {
	t.Helper()
	f.exec(t, `INSERT INTO watcher_changes (watcher_id, old_value, new_value, summary, important, shot_id, created_at) VALUES (?, 'a', 'b', ?, 1, ?, ?)`,
		wid, summary, shot, at)
}

// quiet makes a watcher that has finished its first run, so the tables hold
// only what a test puts in.
func (f *fixture) quiet(t *testing.T, name string) api.Watcher {
	t.Helper()
	w := f.create(t, map[string]any{"name": name, "url": "https://www.example.gov.cn/" + name, "goal": ""})
	watchers.WaitRuns(f.m)
	return w
}

func (f *fixture) shotStatus(t *testing.T, id int64) int {
	t.Helper()
	status, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/watchers/shots/%d", id), nil, nil)
	return status
}

func shotIDs(r api.WatcherRun) []int64 {
	var out []int64
	for _, s := range r.Steps {
		if s.ShotId != nil {
			out = append(out, *s.ShotId)
		}
	}
	return out
}

func runByID(t *testing.T, d api.WatcherDetail, id int64) api.WatcherRun {
	t.Helper()
	for _, r := range d.Runs {
		if r.Id == id {
			return r
		}
	}
	t.Fatalf("run %d not in the detail", id)
	return api.WatcherRun{}
}

// seeded is a watcher with runs and shots of every kind.
type seeded struct {
	w                      api.Watcher
	a, b, c, d, e, f       int64 // shots: 100, 200, 300, 400, 500, 600 bytes
	run1, run2, run3, run4 int64
}

func (f *fixture) seed(t *testing.T) seeded {
	t.Helper()
	w := f.quiet(t, "seed")
	now := time.Now()
	s := seeded{w: w}
	s.a = f.seedShot(t, w.Id, 100, now.Add(-30*time.Minute))
	s.b = f.seedShot(t, w.Id, 200, now.Add(-29*time.Minute))
	s.c = f.seedShot(t, w.Id, 300, now.Add(-20*time.Minute))
	s.d = f.seedShot(t, w.Id, 400, now.Add(-10*time.Minute))
	s.e = f.seedShot(t, w.Id, 500, now.Add(-9*time.Minute))
	s.run1 = f.seedRun(t, w.Id, "changed", now.Add(-31*time.Minute), s.a, s.b)
	s.run2 = f.seedRun(t, w.Id, "failed", now.Add(-21*time.Minute), s.c)
	s.run3 = f.seedRun(t, w.Id, "same", now.Add(-11*time.Minute), s.d, s.e)
	// A run in progress: its newest shot is not in the steps yet.
	s.run4 = f.seedRun(t, w.Id, "running", now.Add(-2*time.Minute))
	s.f = f.seedShot(t, w.Id, 600, now.Add(-time.Minute))
	f.seedChange(t, w.Id, "状态变了", &s.a, now.Add(-30*time.Minute))
	return s
}

func TestRunsArePaged(t *testing.T) {
	f := setup(t, false)
	w := f.quiet(t, "page")
	now := time.Now()
	for i := 0; i < 24; i++ {
		f.seedRun(t, w.Id, "same", now.Add(time.Duration(i-30)*time.Minute))
	}
	// 25 runs in all: the one the creation made and 24 more.
	var p1, p2, p3 api.WatcherRunPage
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/watchers/%d/runs", w.Id), nil, &p1)
	if len(p1.Items) != 10 || !p1.HasMore {
		t.Fatalf("first page: %d items, hasMore %v", len(p1.Items), p1.HasMore)
	}
	for i := 1; i < len(p1.Items); i++ {
		if p1.Items[i].Id >= p1.Items[i-1].Id {
			t.Fatalf("not newest first: %d after %d", p1.Items[i].Id, p1.Items[i-1].Id)
		}
	}
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/watchers/%d/runs?before=%d", w.Id, p1.Items[9].Id), nil, &p2)
	if len(p2.Items) != 10 || !p2.HasMore || p2.Items[0].Id >= p1.Items[9].Id {
		t.Fatalf("second page: %d items, hasMore %v", len(p2.Items), p2.HasMore)
	}
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/watchers/%d/runs?before=%d&limit=50", w.Id, p2.Items[9].Id), nil, &p3)
	if len(p3.Items) != 5 || p3.HasMore {
		t.Fatalf("last page: %d items, hasMore %v", len(p3.Items), p3.HasMore)
	}
	// Exactly one page: nothing more.
	var exact api.WatcherRunPage
	f.env.MustDo(http.MethodGet, fmt.Sprintf("/watchers/%d/runs?limit=25", w.Id), nil, &exact)
	if len(exact.Items) != 25 || exact.HasMore {
		t.Fatalf("exact page: %d items, hasMore %v", len(exact.Items), exact.HasMore)
	}
	for _, bad := range []string{"limit=0", "limit=51"} {
		if status, _ := f.env.Do(http.MethodGet, fmt.Sprintf("/watchers/%d/runs?%s", w.Id, bad), nil, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, status)
		}
	}
	if status, _ := f.env.Do(http.MethodGet, "/watchers/999/runs", nil, nil); status != http.StatusNotFound {
		t.Errorf("missing watcher: %d", status)
	}
	// The detail still has the latest 20.
	if d := f.get(t, w.Id); len(d.Runs) != 20 {
		t.Errorf("detail has %d runs", len(d.Runs))
	}
}

func TestCleanupKeepsTheLatestSuccess(t *testing.T) {
	f := setup(t, false)
	s := f.seed(t)
	other := f.quiet(t, "other")
	otherShot := f.seedShot(t, other.Id, 700, time.Now().Add(-time.Hour))

	d := f.get(t, s.w.Id)
	if d.Shots.Count != 6 || d.Shots.Bytes != 2100 {
		t.Fatalf("usage %+v", d.Shots)
	}

	var res api.WatcherShotsDeleted
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/shots/cleanup", s.w.Id), map[string]string{"keep": "latest"}, &res)
	// Kept: the change's shot (a), the newest success (d, e) and the run in progress (f).
	if res.Deleted != 2 || res.Bytes != 500 {
		t.Fatalf("latest: %+v", res)
	}
	for id, want := range map[int64]int{s.a: 200, s.b: 404, s.c: 404, s.d: 200, s.e: 200, s.f: 200, otherShot: 200} {
		if got := f.shotStatus(t, id); got != want {
			t.Errorf("shot %d: %d, want %d", id, got, want)
		}
	}
	d = f.get(t, s.w.Id)
	if got := shotIDs(runByID(t, d, s.run1)); len(got) != 1 || got[0] != s.a {
		t.Errorf("run1 steps point at %v", got)
	}
	if got := shotIDs(runByID(t, d, s.run2)); len(got) != 0 {
		t.Errorf("run2 steps point at %v", got)
	}
	if got := shotIDs(runByID(t, d, s.run3)); len(got) != 2 {
		t.Errorf("run3 steps point at %v", got)
	}
	if d.Changes[0].ShotId == nil || *d.Changes[0].ShotId != s.a {
		t.Errorf("change lost its shot: %+v", d.Changes[0])
	}
	if d.Shots.Count != 4 || d.Shots.Bytes != 1600 {
		t.Errorf("usage after latest %+v", d.Shots)
	}

	// Nothing left to delete: a second cleanup does nothing.
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/shots/cleanup", s.w.Id), map[string]string{"keep": "latest"}, &res)
	if res.Deleted != 0 || res.Bytes != 0 {
		t.Errorf("second latest: %+v", res)
	}

	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/shots/cleanup", s.w.Id), map[string]string{"keep": "none"}, &res)
	// Everything but the shot of the run in progress.
	if res.Deleted != 3 || res.Bytes != 1000 {
		t.Fatalf("none: %+v", res)
	}
	d = f.get(t, s.w.Id)
	if d.Changes[0].ShotId != nil {
		t.Errorf("change still points at shot %d", *d.Changes[0].ShotId)
	}
	for _, id := range []int64{s.run1, s.run2, s.run3} {
		if got := shotIDs(runByID(t, d, id)); len(got) != 0 {
			t.Errorf("run %d steps point at %v", id, got)
		}
	}
	if d.Shots.Count != 1 || d.Shots.Bytes != 600 || f.shotStatus(t, s.f) != 200 || f.shotStatus(t, otherShot) != 200 {
		t.Errorf("after none: %+v", d.Shots)
	}
	// The steps are still there, only without pictures.
	if len(runByID(t, d, s.run1).Steps) != 2 {
		t.Error("steps were removed")
	}

	if status, _ := f.env.Do(http.MethodPost, fmt.Sprintf("/watchers/%d/shots/cleanup", s.w.Id), map[string]string{"keep": "all"}, nil); status != http.StatusBadRequest {
		t.Errorf("bad keep: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPost, "/watchers/999/shots/cleanup", map[string]string{"keep": "none"}, nil); status != http.StatusNotFound {
		t.Errorf("missing watcher: %d", status)
	}
}

func TestDeleteTheShotsOfOneRun(t *testing.T) {
	f := setup(t, false)
	s := f.seed(t)
	other := f.quiet(t, "other")

	var res api.WatcherShotsDeleted
	// Run 1 owns a (the change's shot) and b.
	f.env.MustDo(http.MethodDelete, fmt.Sprintf("/watchers/%d/runs/%d/shots", s.w.Id, s.run1), nil, &res)
	if res.Deleted != 2 || res.Bytes != 300 {
		t.Fatalf("delete run1: %+v", res)
	}
	d := f.get(t, s.w.Id)
	if got := shotIDs(runByID(t, d, s.run1)); len(got) != 0 {
		t.Errorf("run1 steps point at %v", got)
	}
	if d.Changes[0].ShotId != nil {
		t.Error("the change still has its shot")
	}
	if got := shotIDs(runByID(t, d, s.run3)); len(got) != 2 {
		t.Errorf("run3 lost shots: %v", got)
	}
	if f.shotStatus(t, s.a) != 404 || f.shotStatus(t, s.b) != 404 || f.shotStatus(t, s.d) != 200 {
		t.Error("wrong shots deleted")
	}
	// Again: nothing to delete.
	f.env.MustDo(http.MethodDelete, fmt.Sprintf("/watchers/%d/runs/%d/shots", s.w.Id, s.run1), nil, &res)
	if res.Deleted != 0 || res.Bytes != 0 {
		t.Errorf("again: %+v", res)
	}

	// A run of another watcher, a missing run and a run in progress.
	for path, want := range map[string]int{
		fmt.Sprintf("/watchers/%d/runs/%d/shots", other.Id, s.run3): http.StatusNotFound,
		fmt.Sprintf("/watchers/%d/runs/99999/shots", s.w.Id):        http.StatusNotFound,
		fmt.Sprintf("/watchers/%d/runs/%d/shots", s.w.Id, s.run4):   http.StatusConflict,
	} {
		if status, _ := f.env.Do(http.MethodDelete, path, nil, nil); status != want {
			t.Errorf("%s: %d, want %d", path, status, want)
		}
	}
	if f.shotStatus(t, s.f) != 200 || f.shotStatus(t, s.d) != 200 {
		t.Error("a refused delete removed shots")
	}
}

func TestFailedRunKeepsTheLastShotOnly(t *testing.T) {
	f := setup(t, true)
	// Three steps with a picture each, then the AI gives up.
	f.chat.script(append(loginMoves(), do("give_up", func(string) map[string]any { return map[string]any{"reason": "没找到"} }))...)
	w := f.create(t, map[string]any{"name": "失败", "url": f.site.URL + "/status", "goal": "盯办理状态",
		"username": "alice", "password": "s3cret"})
	got := f.until(t, w.Id, api.WatcherStatusError)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	run := got.Runs[0]
	if run.Outcome != api.WatcherRunOutcomeFailed {
		t.Fatalf("outcome %s", run.Outcome)
	}
	ids := shotIDs(run)
	if len(ids) != 1 {
		t.Fatalf("a failed run keeps %d shots: %+v", len(ids), run.Steps)
	}
	if got.Shots.Count != 1 || f.shotStatus(t, ids[0]) != 200 {
		t.Fatalf("stored shots %+v", got.Shots)
	}
	// The kept picture is the last one of the run.
	lastWithShot := -1
	for i, s := range run.Steps {
		if s.ShotId != nil {
			lastWithShot = i
		}
	}
	if lastWithShot < 1 {
		t.Fatalf("the picture is on step %d", lastWithShot)
	}

	// A run that works keeps every picture.
	f.chat.script(append(loginMoves(), finish("审核中"))...)
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "explore"}, nil)
	f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	ok := got.Runs[0]
	if ok.Outcome != api.WatcherRunOutcomeOk && ok.Outcome != api.WatcherRunOutcomeSame {
		t.Fatalf("outcome %s", ok.Outcome)
	}
	if n := len(shotIDs(ok)); n < 2 || got.Shots.Count != n+1 {
		t.Fatalf("success keeps %d shots, %d stored", n, got.Shots.Count)
	}
}

func TestNoMoreAsksAfterTheLimit(t *testing.T) {
	f := setup(t, true)
	watchers.SetWait(f.m, 200*time.Millisecond) // nobody answers
	askMove := do("ask_user", func(string) map[string]any {
		return map[string]any{"kind": "captcha", "message": "请输入图形验证码"}
	})
	f.chat.script(askMove)
	w := f.create(t, map[string]any{"name": "验证码", "url": f.site.URL + "/status", "goal": ""})
	f.until(t, w.Id, api.WatcherStatusLogin)
	watchers.WaitRuns(f.m)
	for i := 0; i < 2; i++ {
		f.chat.script(askMove)
		f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "explore"}, nil)
		watchers.WaitRuns(f.m)
	}
	if n := f.notified(t, "watcher.ask"); n != 3 {
		t.Fatalf("%d reminders before the limit", n)
	}

	// The fourth run may not ask.
	f.chat.script(askMove)
	before := time.Now()
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "explore"}, nil)
	watchers.WaitRuns(f.m)
	got := f.get(t, w.Id)
	if n := f.notified(t, "watcher.ask"); n != 3 {
		t.Errorf("%d reminders after the limit", n)
	}
	if got.StatusDetail != "今天已经提醒过你 3 次，明天再检查" {
		t.Errorf("status detail %q", got.StatusDetail)
	}
	run := got.Runs[0]
	if run.Outcome != api.WatcherRunOutcomeFailed {
		t.Fatalf("outcome %s", run.Outcome)
	}
	for _, s := range run.Steps {
		if strings.HasPrefix(s.Text, "等你处理") && s.ShotId != nil {
			t.Errorf("a picture was made before the reminder was allowed: %+v", s)
		}
	}
	// The next check is tomorrow at 0:00 in the time zone of the settings.
	loc, _ := time.LoadLocation("Asia/Shanghai")
	day := before.In(loc)
	want := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	if got.NextCheckAt == nil || !got.NextCheckAt.Equal(want) {
		t.Errorf("next check %v, want %v", got.NextCheckAt, want)
	}
	// A scheduled tick does not start it today.
	calls := f.chat.count()
	if err := watchers.Tick(f.m, t.Context()); err != nil {
		t.Fatal(err)
	}
	watchers.WaitRuns(f.m)
	if f.chat.count() != calls {
		t.Error("the tick ran the check")
	}
}

func TestListCarriesTheLastChange(t *testing.T) {
	f := setup(t, false)
	w1 := f.quiet(t, "one")
	w2 := f.quiet(t, "two")
	w3 := f.quiet(t, "three")
	now := time.Now().Truncate(time.Second)
	f.seedChange(t, w1.Id, "第一次", nil, now.Add(-2*time.Hour))
	f.seedChange(t, w1.Id, "第二次", nil, now.Add(-time.Hour))
	f.seedChange(t, w2.Id, "另一个", nil, now.Add(-3*time.Hour))
	f.exec(t, `UPDATE watcher_changes SET important = 0 WHERE summary = '另一个'`)

	var list struct{ Items []api.Watcher }
	f.env.MustDo(http.MethodGet, "/watchers", nil, &list)
	byID := map[int64]api.Watcher{}
	for _, it := range list.Items {
		byID[it.Id] = it
	}
	if lc := byID[w1.Id].LastChange; lc == nil || lc.Summary != "第二次" || !lc.Important || !lc.CreatedAt.Equal(now.Add(-time.Hour)) {
		t.Errorf("w1 %+v", lc)
	}
	if lc := byID[w2.Id].LastChange; lc == nil || lc.Summary != "另一个" || lc.Important {
		t.Errorf("w2 %+v", lc)
	}
	if byID[w3.Id].LastChange != nil {
		t.Errorf("w3 %+v", byID[w3.Id].LastChange)
	}
	if d := f.get(t, w1.Id); d.LastChange == nil || d.LastChange.Summary != "第二次" {
		t.Errorf("detail %+v", d.LastChange)
	}
	if d := f.get(t, w3.Id); d.LastChange != nil {
		t.Errorf("detail of w3 %+v", d.LastChange)
	}
}
