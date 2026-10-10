package journal_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal"
	"github.com/j0x3n/x-console/backend/internal/server/modules/journal/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// fakeSource reports a fixed list, cut to the asked range, and counts calls.
type fakeSource struct {
	mu    sync.Mutex
	items []contracts.Activity
	err   error
	calls int
}

func (f *fakeSource) set(items []contracts.Activity, err error) {
	f.mu.Lock()
	f.items, f.err = items, err
	f.mu.Unlock()
}

func (f *fakeSource) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSource) Activity(_ context.Context, from, until time.Time) ([]contracts.Activity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	var out []contracts.Activity
	for _, it := range f.items {
		if !it.At.Before(from) && it.At.Before(until) {
			out = append(out, it)
		}
	}
	return out, nil
}

type rig struct {
	env *testutil.Env
	m   *journal.Module
	src *fakeSource
	loc *time.Location
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*journal.Module](env.App.Deps.Registry, journal.ServiceKey)
	if !ok {
		t.Fatal("journal module missing")
	}
	src := &fakeSource{}
	module.Provide[contracts.ActivitySource](env.App.Deps.Registry, contracts.ActivitySourcePrefix+"fake", src)
	return &rig{env: env, m: m, src: src, loc: env.App.Deps.Scheduler.Location()}
}

// day returns local midnight n days from today.
func (r *rig) day(n int) time.Time {
	now := time.Now().In(r.loc)
	return time.Date(now.Year(), now.Month(), now.Day()+n, 0, 0, 0, 0, r.loc)
}

func (r *rig) date(n int) string { return r.day(n).Format("2006-01-02") }

func (r *rig) at(n int, hour, minute int) time.Time {
	d := r.day(n)
	return time.Date(d.Year(), d.Month(), d.Day(), hour, minute, 0, 0, r.loc)
}

func (r *rig) getDay(t *testing.T, date string) api.JournalDay {
	t.Helper()
	var out api.JournalDay
	r.env.MustDo(http.MethodGet, "/journal/days/"+date, nil, &out)
	return out
}

func act(ref, module, kind, title string, at time.Time) contracts.Activity {
	return contracts.Activity{Ref: ref, Module: module, Kind: kind, Title: title, At: at}
}

func titles(items []api.JournalItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func TestDayListsItemsInTimeOrderWithCounts(t *testing.T) {
	r := setup(t)
	focus := act("f1", "calendar", "focus", "专注 25 分钟", r.at(0, 10, 0))
	focus.Minutes = 25
	focus2 := act("f2", "calendar", "focus", "专注 50 分钟", r.at(0, 14, 0))
	focus2.Minutes = 50
	card := act("c1", "projects", "card", "完成 XC-1 登录页", r.at(0, 9, 0))
	card.Detail, card.Link = "示例项目", "/projects/XC/1"
	r.src.set([]contracts.Activity{focus2, focus, card,
		act("k1", "coding", "task", "Claude 跑完：修复溢出", r.at(0, 11, 30)),
		act("y1", "projects", "card", "昨天的卡片", r.at(-1, 20, 0))}, nil)

	d := r.getDay(t, r.date(0))
	if got := strings.Join(titles(d.Items), "|"); got != "完成 XC-1 登录页|专注 25 分钟|Claude 跑完：修复溢出|专注 50 分钟" {
		t.Fatalf("order: %s", got)
	}
	if d.Items[0].Link != "/projects/XC/1" || d.Items[0].Detail != "示例项目" || d.Items[0].Module != "projects" {
		t.Fatalf("fields: %+v", d.Items[0])
	}
	want := []api.JournalCount{{Kind: api.Card, Count: 1}, {Kind: api.Task, Count: 1}, {Kind: api.Focus, Count: 2, Minutes: 75}}
	if fmt.Sprint(d.Counts) != fmt.Sprint(want) {
		t.Fatalf("counts: %+v", d.Counts)
	}
	if d.Day != r.date(0) || d.Diary.Body != "" || d.Diary.UpdatedAt != nil {
		t.Fatalf("day: %+v", d)
	}
	// yesterday is a different page
	if y := r.getDay(t, r.date(-1)); len(y.Items) != 1 || y.Items[0].Title != "昨天的卡片" {
		t.Fatalf("yesterday: %+v", y.Items)
	}
	// an empty day is still a page
	empty := r.getDay(t, r.date(-5))
	if len(empty.Items) != 0 || len(empty.Counts) != 0 || empty.Items == nil {
		t.Fatalf("empty day: %+v", empty)
	}
}

func TestCollectReplacesOldItemsAndKeepsThemOnError(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	r.src.set([]contracts.Activity{act("a", "projects", "card", "第一张", r.at(0, 8, 0)), act("b", "projects", "card", "第二张", r.at(0, 9, 0))}, nil)
	journal.CollectRecent(r.m, ctx)
	if d := r.getDay(t, r.date(0)); len(d.Items) != 2 {
		t.Fatalf("first: %v", titles(d.Items))
	}
	// a card turned back to open and another changed its title
	r.src.set([]contracts.Activity{act("b", "projects", "card", "第二张（改名）", r.at(0, 9, 0))}, nil)
	journal.Collect(r.m, ctx, r.day(0), r.day(1))
	if d := r.getDay(t, r.date(0)); strings.Join(titles(d.Items), "|") != "第二张（改名）" {
		t.Fatalf("replaced: %v", titles(d.Items))
	}
	// a failing source keeps what it said before
	r.src.set(nil, errors.New("broken"))
	journal.Collect(r.m, ctx, r.day(0), r.day(1))
	if d := r.getDay(t, r.date(0)); len(d.Items) != 1 {
		t.Fatalf("kept: %v", titles(d.Items))
	}
	// only the asked range is replaced
	r.src.set([]contracts.Activity{act("old", "projects", "card", "上周的卡片", r.at(-7, 9, 0))}, nil)
	journal.Collect(r.m, ctx, r.day(-7), r.day(-6))
	r.src.set(nil, nil)
	journal.Collect(r.m, ctx, r.day(0), r.day(1))
	if d := r.getDay(t, r.date(-7)); len(d.Items) != 1 {
		t.Fatalf("old day must stay: %v", titles(d.Items))
	}
	if d := r.getDay(t, r.date(0)); len(d.Items) != 0 {
		t.Fatalf("today must be emptied: %v", titles(d.Items))
	}
}

func TestBadItemsAreDroppedAndTextIsClipped(t *testing.T) {
	r := setup(t)
	long := act("long", "notes", "note", strings.Repeat("长", 300), r.at(0, 8, 0))
	long.Detail = strings.Repeat("说", 800)
	spaced := act("sp", "notes", "note", "  两行\n标题  ", r.at(0, 8, 5))
	r.src.set([]contracts.Activity{
		long, spaced,
		act("", "notes", "note", "没有 ref", r.at(0, 9, 0)),
		act("nokind", "notes", "", "没有类型", r.at(0, 9, 0)),
		act("nomod", "", "note", "没有模块", r.at(0, 9, 0)),
		act("notitle", "notes", "note", "   ", r.at(0, 9, 0)),
	}, nil)
	journal.CollectRecent(r.m, context.Background())
	d := r.getDay(t, r.date(0))
	if len(d.Items) != 2 {
		t.Fatalf("items: %v", titles(d.Items))
	}
	if n := len([]rune(d.Items[0].Title)); n != 200 {
		t.Fatalf("title length %d", n)
	}
	if n := len([]rune(d.Items[0].Detail)); n != 500 {
		t.Fatalf("detail length %d", n)
	}
	if d.Items[1].Title != "两行 标题" {
		t.Fatalf("whitespace: %q", d.Items[1].Title)
	}
}

func TestPageRefreshesRecentDaysOnlyAndNotTooOften(t *testing.T) {
	r := setup(t)
	now := time.Now()
	journal.SetNow(r.m, func() time.Time { return now })
	r.src.set([]contracts.Activity{act("a", "projects", "card", "刚完成", r.at(0, 0, 1))}, nil)

	d := r.getDay(t, r.date(0))
	if len(d.Items) != 1 {
		t.Fatalf("first visit must collect: %v", titles(d.Items))
	}
	calls := r.src.count()
	r.getDay(t, r.date(0))
	if r.src.count() != calls {
		t.Fatal("a second visit within 30 seconds must not collect again")
	}
	now = now.Add(31 * time.Second)
	r.src.set([]contracts.Activity{act("a", "projects", "card", "刚完成", r.at(0, 0, 1)), act("b", "projects", "card", "又一张", r.at(0, 0, 2))}, nil)
	if d := r.getDay(t, r.date(0)); len(d.Items) != 2 {
		t.Fatalf("after 30 seconds: %v", titles(d.Items))
	}
	// an old day is read from the store only
	calls = r.src.count()
	now = now.Add(time.Hour)
	r.getDay(t, r.date(-10))
	if r.src.count() != calls {
		t.Fatal("an old day must not trigger a collection")
	}
}

func TestBackfillReadsOlderDays(t *testing.T) {
	r := setup(t)
	r.src.set([]contracts.Activity{
		act("old", "projects", "card", "两个月前", r.at(-60, 9, 0)),
		act("older", "projects", "card", "三个月以前", r.at(-120, 9, 0)),
	}, nil)
	if err := journal.Backfill(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
	if d := r.getDay(t, r.date(-60)); len(d.Items) != 1 {
		t.Fatalf("60 days ago: %v", titles(d.Items))
	}
	if d := r.getDay(t, r.date(-120)); len(d.Items) != 0 {
		t.Fatalf("120 days ago is past the backfill: %v", titles(d.Items))
	}
}

func TestDiary(t *testing.T) {
	r := setup(t)
	day := r.date(-2)
	put := func(date string, body any) (int, []byte) {
		return r.env.Do(http.MethodPut, "/journal/days/"+date+"/diary", map[string]any{"body": body}, nil)
	}
	var diary api.JournalDiary
	r.env.MustDo(http.MethodPut, "/journal/days/"+day+"/diary", map[string]any{"body": "## 今天\n修好了 **溢出**。"}, &diary)
	if diary.Day != day || !strings.Contains(diary.Body, "修好了") || diary.UpdatedAt == nil {
		t.Fatalf("saved: %+v", diary)
	}
	if d := r.getDay(t, day); d.Diary.Body != diary.Body || d.Diary.UpdatedAt == nil {
		t.Fatalf("read back: %+v", d.Diary)
	}
	r.env.MustDo(http.MethodPut, "/journal/days/"+day+"/diary", map[string]any{"body": "改过了"}, &diary)
	if d := r.getDay(t, day); d.Diary.Body != "改过了" {
		t.Fatalf("overwrite: %+v", d.Diary)
	}
	// 清空 = 删除这一天的日记
	r.env.MustDo(http.MethodPut, "/journal/days/"+day+"/diary", map[string]any{"body": " \n\t "}, &diary)
	if d := r.getDay(t, day); d.Diary.Body != "" || d.Diary.UpdatedAt != nil {
		t.Fatalf("cleared: %+v", d.Diary)
	}
	var recent api.JournalRecent
	r.env.MustDo(http.MethodGet, "/journal/recent", nil, &recent)
	for _, x := range recent.Days {
		if x.Day == day {
			t.Fatalf("a day without diary or items must not be listed: %+v", recent.Days)
		}
	}
	// 校验
	if status, _ := put(day, strings.Repeat("字", 20001)); status != http.StatusBadRequest {
		t.Fatalf("too long: %d", status)
	}
	if status, _ := put(day, strings.Repeat("字", 20000)); status != http.StatusOK {
		t.Fatalf("max length: %d", status)
	}
	for _, bad := range []string{"abc", "2026-13-01", "2026-02-30", "20261010", "2026-1-1"} {
		if status, _ := put(bad, "x"); status != http.StatusBadRequest {
			t.Fatalf("PUT %q: %d", bad, status)
		}
		if status, _ := r.env.Do(http.MethodGet, "/journal/days/"+bad, nil, nil); status != http.StatusBadRequest {
			t.Fatalf("GET %q: %d", bad, status)
		}
	}
	if status, _ := r.env.Do(http.MethodPut, "/journal/days/"+day+"/diary", map[string]any{}, nil); status != http.StatusBadRequest {
		t.Fatalf("missing body: %d", status)
	}
}

func TestRecentListsDaysWithContent(t *testing.T) {
	r := setup(t)
	r.src.set([]contracts.Activity{
		act("a", "projects", "card", "A", r.at(-1, 9, 0)), act("b", "projects", "card", "B", r.at(-1, 10, 0)),
		act("c", "projects", "card", "C", r.at(-9, 9, 0)),
	}, nil)
	journal.Collect(r.m, context.Background(), r.day(-10), r.day(1))
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(-3)+"/diary", map[string]any{"body": "只有日记"}, nil)
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(-1)+"/diary", map[string]any{"body": "也有日记"}, nil)

	var out api.JournalRecent
	r.env.MustDo(http.MethodGet, "/journal/recent?days=14", nil, &out)
	want := []api.JournalRecentDay{
		{Day: r.date(0)}, {Day: r.date(-1), Items: 2, Diary: true}, {Day: r.date(-3), Diary: true}, {Day: r.date(-9), Items: 1},
	}
	if fmt.Sprint(out.Days) != fmt.Sprint(want) {
		t.Fatalf("recent: %+v", out.Days)
	}
	r.env.MustDo(http.MethodGet, "/journal/recent?days=5", nil, &out)
	if len(out.Days) != 3 || out.Days[2].Day != r.date(-3) {
		t.Fatalf("5 days: %+v", out.Days)
	}
	r.env.MustDo(http.MethodGet, "/journal/recent?days=1", nil, &out)
	if len(out.Days) != 1 || out.Days[0].Day != r.date(0) {
		t.Fatalf("1 day: %+v", out.Days)
	}
	for _, bad := range []string{"0", "367", "-1", "x"} {
		if status, _ := r.env.Do(http.MethodGet, "/journal/recent?days="+bad, nil, nil); status != http.StatusBadRequest {
			t.Fatalf("days=%s: %d", bad, status)
		}
	}
}

func TestSearchFindsDiaryAndItems(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	hit := act("a", "projects", "card", "完成 XC-7 修复服务器巡检", r.at(-4, 9, 0))
	hit.Detail = "和 Redis 有关"
	r.src.set([]contracts.Activity{hit, act("b", "coding", "task", "别的事情", r.at(-1, 9, 0)), act("p", "projects", "card", "进度 100% 完成", r.at(-2, 9, 0)), act("u", "projects", "card", "my_var 重命名", r.at(-2, 10, 0))}, nil)
	journal.Collect(r.m, ctx, r.day(-10), r.day(1))
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(-6)+"/diary", map[string]any{"body": "今天服务器突然变慢，查了一下午，最后是磁盘满了。"}, nil)
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(-2)+"/diary", map[string]any{"body": "没什么特别的"}, nil)

	search := func(q string) []api.JournalHit {
		var out api.JournalSearch
		r.env.MustDo(http.MethodGet, "/journal/search?q="+q, nil, &out)
		return out.Items
	}
	got := search("%E6%9C%8D%E5%8A%A1%E5%99%A8") // 服务器
	if len(got) != 2 || got[0].Day != r.date(-4) || got[0].Kind != "card" || got[1].Day != r.date(-6) || got[1].Kind != "diary" {
		t.Fatalf("server: %+v", got)
	}
	if !strings.Contains(got[1].Snippet, "服务器突然变慢") || got[0].Link != "/journal?date="+r.date(-4) {
		t.Fatalf("snippet or link: %+v", got)
	}
	if h := search("redis"); len(h) != 1 || !strings.Contains(h[0].Snippet, "Redis") {
		t.Fatalf("detail, case-insensitive: %+v", h)
	}
	// % 和 _ 是普通字符
	if h := search("100%25"); len(h) != 1 || h[0].Title != "进度 100% 完成" {
		t.Fatalf("percent: %+v", h)
	}
	if h := search("%25"); len(h) != 1 {
		t.Fatalf("a lone percent must not match everything: %+v", h)
	}
	if h := search("_"); len(h) != 1 || h[0].Title != "my_var 重命名" {
		t.Fatalf("underscore: %+v", h)
	}
	if h := search("没有这个词xyz"); len(h) != 0 {
		t.Fatalf("miss: %+v", h)
	}
	if status, _ := r.env.Do(http.MethodGet, "/journal/search?q=%20%20", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("blank query: %d", status)
	}
	if status, _ := r.env.Do(http.MethodGet, "/journal/search", nil, nil); status != http.StatusBadRequest {
		t.Fatalf("missing query: %d", status)
	}
	if status, _ := r.env.Do(http.MethodGet, "/journal/search?q="+strings.Repeat("a", 101), nil, nil); status != http.StatusBadRequest {
		t.Fatalf("long query: %d", status)
	}
}

func TestSearchStopsAt100Hits(t *testing.T) {
	r := setup(t)
	var items []contracts.Activity
	for i := 0; i < 130; i++ {
		items = append(items, act(fmt.Sprint("n", i), "notes", "note", fmt.Sprintf("同名笔记 %d", i), r.at(0, 0, 0).Add(time.Duration(i)*time.Minute)))
	}
	r.src.set(items, nil)
	journal.CollectRecent(r.m, context.Background())
	var out api.JournalSearch
	r.env.MustDo(http.MethodGet, "/journal/search?q=%E5%90%8C%E5%90%8D", nil, &out)
	if len(out.Items) != 100 {
		t.Fatalf("hits: %d", len(out.Items))
	}
	if out.Items[0].Title != "同名笔记 129" {
		t.Fatalf("newest first: %q", out.Items[0].Title)
	}
}

// ---- 隐藏内容 ----

func TestHiddenModulesAreLeftOutWhileLocked(t *testing.T) {
	r := setup(t)
	r.src.set([]contracts.Activity{
		act("a", "projects", "card", "项目里的卡片", r.at(0, 9, 0)),
		act("b", "screentime", "screen", "电脑用了 5 小时", r.at(0, 10, 0)),
		act("c", "coding", "task", "Agent 任务", r.at(0, 11, 0)),
	}, nil)
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(0)+"/diary", map[string]any{"body": "日记 秘密词"}, nil)
	journal.CollectRecent(r.m, context.Background())
	r.env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	r.env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"screentime", "projects"}}, nil)

	// unlocked: everything
	if d := r.getDay(t, r.date(0)); len(d.Items) != 3 {
		t.Fatalf("unlocked: %v", titles(d.Items))
	}
	r.env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	d := r.getDay(t, r.date(0))
	if strings.Join(titles(d.Items), "|") != "Agent 任务" || len(d.Counts) != 1 || d.Counts[0].Kind != api.Task {
		t.Fatalf("locked day: %+v", d)
	}
	var recent api.JournalRecent
	r.env.MustDo(http.MethodGet, "/journal/recent", nil, &recent)
	if recent.Days[0].Items != 1 {
		t.Fatalf("locked recent counts: %+v", recent.Days[0])
	}
	var found api.JournalSearch
	r.env.MustDo(http.MethodGet, "/journal/search?q=%E7%94%B5%E8%84%91", nil, &found) // 电脑
	if len(found.Items) != 0 {
		t.Fatalf("locked search: %+v", found.Items)
	}
	r.env.MustDo(http.MethodGet, "/journal/search?q=%E5%8D%A1%E7%89%87", nil, &found) // 卡片
	if len(found.Items) != 0 {
		t.Fatalf("locked search: %+v", found.Items)
	}
}

func TestJournalItselfCanBeHidden(t *testing.T) {
	r := setup(t)
	r.env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	r.env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"journal"}}, nil)
	if status, _ := r.env.Do(http.MethodGet, "/journal/days/"+r.date(0), nil, nil); status != http.StatusOK {
		t.Fatalf("unlocked: %d", status)
	}
	r.env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	for _, path := range []string{"/journal/days/" + r.date(0), "/journal/recent", "/journal/search?q=x"} {
		if status, _ := r.env.Do(http.MethodGet, path, nil, nil); status != http.StatusNotFound {
			t.Fatalf("locked GET %s: %d", path, status)
		}
	}
	if status, _ := r.env.Do(http.MethodPut, "/journal/days/"+r.date(0)+"/diary", map[string]any{"body": "x"}, nil); status != http.StatusNotFound {
		t.Fatalf("locked PUT: %d", status)
	}
	var avail struct{ Modules []string }
	r.env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, id := range avail.Modules {
		if id == "journal" {
			t.Fatalf("journal still available: %v", avail.Modules)
		}
	}
}

// ---- 日程和告警来源 ----

type fakeCalendar struct{ events []contracts.CalendarEvent }

func (f fakeCalendar) Events(_ context.Context, from, to time.Time) ([]contracts.CalendarEvent, error) {
	var out []contracts.CalendarEvent
	for _, e := range f.events {
		if !e.Start.Before(from) && e.Start.Before(to) {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeHosts struct {
	contracts.Hosts
	alerts []contracts.HostAlert
}

func (f fakeHosts) Alerts(_ context.Context, since time.Time) ([]contracts.HostAlert, error) {
	var out []contracts.HostAlert
	for _, a := range f.alerts {
		if !a.FiredAt.Before(since) {
			out = append(out, a)
		}
	}
	return out, nil
}

func TestCalendarAndHostAlertSources(t *testing.T) {
	r := setup(t)
	reg := r.env.App.Deps.Registry
	module.Provide[contracts.Calendar](reg, contracts.CalendarKey, fakeCalendar{events: []contracts.CalendarEvent{
		{Title: "周会", Start: r.at(0, 10, 0), End: r.at(0, 11, 0), Location: "会议室 A", Calendar: "工作"},
		{Title: "生日", Start: r.at(0, 0, 0), End: r.at(1, 0, 0), AllDay: true, Calendar: "家人"},
		{Title: "  ", Start: r.at(0, 12, 0)},
	}})
	resolved := r.at(0, 8, 40)
	module.Provide[contracts.Hosts](reg, contracts.HostsKey, fakeHosts{alerts: []contracts.HostAlert{
		{HostID: "h1", HostName: "web-1", Rule: "cpu", Message: "CPU 超过 90%", FiredAt: r.at(0, 8, 10), Resolved: &resolved},
		{HostID: "h2", HostName: "db-1", Rule: "disk", Message: "磁盘快满了", FiredAt: r.at(0, 9, 0)},
		{HostID: "h3", HostName: "old", Rule: "cpu", Message: "明天的告警", FiredAt: r.at(1, 9, 0)},
	}})
	// the fake source from setup stays quiet
	r.src.set(nil, nil)
	journal.CollectRecent(r.m, context.Background())
	d := r.getDay(t, r.date(0))
	got := map[string]api.JournalItem{}
	for _, it := range d.Items {
		got[it.Title] = it
	}
	if len(d.Items) != 4 {
		t.Fatalf("items: %v", titles(d.Items))
	}
	if e := got["周会"]; e.Kind != api.Event || e.Module != "calendar" || e.Detail != "会议室 A · 工作" {
		t.Fatalf("event: %+v", e)
	}
	if e := got["生日"]; !strings.HasPrefix(e.Detail, "全天") {
		t.Fatalf("all-day: %+v", e)
	}
	a1, a2 := got["web-1 CPU 超过 90%"], got["db-1 磁盘快满了"]
	if a1.Kind != api.Alert || a1.Module != "servers" || !strings.HasPrefix(a1.Detail, "已恢复") || a1.Link != "/servers/h1" {
		t.Fatalf("resolved alert: %+v", a1)
	}
	if a2.Detail != "还没有恢复" {
		t.Fatalf("open alert: %+v", a2)
	}
}

// ---- AI 动作 ----

func TestAIActions(t *testing.T) {
	r := setup(t)
	ctx := context.Background()
	card := act("a", "projects", "card", "完成 XC-3 搜索", r.at(0, 9, 0))
	card.Link = "/projects/XC/3"
	r.src.set([]contracts.Activity{card}, nil)
	r.env.MustDo(http.MethodPut, "/journal/days/"+r.date(0)+"/diary", map[string]any{"body": "今天写了搜索"}, nil)

	out, err := r.env.App.Deps.Actions.Run(ctx, "journal.day", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "完成 XC-3 搜索") || !strings.Contains(string(raw), "今天写了搜索") || strings.Contains(string(raw), "/projects/XC/3") {
		t.Fatalf("journal.day: %s", raw)
	}
	if _, err := r.env.App.Deps.Actions.Run(ctx, "journal.day", json.RawMessage(`{"date":"not-a-date"}`)); err == nil {
		t.Fatal("bad date must fail")
	}
	out, err = r.env.App.Deps.Actions.Run(ctx, "journal.search", json.RawMessage(`{"q":"搜索"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(out)
	if strings.Count(string(raw), `"day":"`+r.date(0)+`"`) != 2 {
		t.Fatalf("journal.search: %s", raw)
	}
	if _, err := r.env.App.Deps.Actions.Run(ctx, "journal.search", json.RawMessage(`{"q":""}`)); err == nil {
		t.Fatal("empty query must fail")
	}
}
