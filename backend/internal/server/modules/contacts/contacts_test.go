package contacts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// clock is the fixed time of the tests: 2026-10-10 09:00 in Asia/Shanghai.
var clock = time.Date(2026, 10, 10, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

type rig struct {
	env *testutil.Env
	m   *contacts.Module
	now time.Time
}

func setup(t *testing.T) *rig {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*contacts.Module](env.App.Deps.Registry, contacts.ServiceKey)
	if !ok {
		t.Fatal("contacts module not registered")
	}
	r := &rig{env: env, m: m, now: clock}
	contacts.SetNow(m, func() time.Time { return r.now })
	return r
}

func (r *rig) day(n int) string { return r.now.AddDate(0, 0, n).Format("2006-01-02") }

// md is a date without a year, n days from the fixed day.
func (r *rig) md(n int) string { return r.now.AddDate(0, 0, n).Format("01-02") }

type listOut struct {
	Items   []api.Contact      `json:"items"`
	Summary api.ContactSummary `json:"summary"`
}

func (r *rig) create(t *testing.T, body map[string]any) api.Contact {
	t.Helper()
	var c api.Contact
	r.env.MustDo(http.MethodPost, "/contacts", body, &c)
	return c
}

func (r *rig) list(t *testing.T, query string) listOut {
	t.Helper()
	var out listOut
	r.env.MustDo(http.MethodGet, "/contacts"+query, nil, &out)
	return out
}

func (r *rig) remind(t *testing.T) {
	t.Helper()
	if err := contacts.RemindAll(r.m, context.Background()); err != nil {
		t.Fatal(err)
	}
}

type notice struct{ Kind, Title, Body, Link string }

func (r *rig) notices(t *testing.T, kind string) []notice {
	t.Helper()
	var out struct{ Items []notice }
	r.env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == kind {
			got = append(got, n)
		}
	}
	return got
}

func event(kind, date string) map[string]any { return map[string]any{"kind": kind, "date": date} }

func id(c api.Contact) string { return strconv.FormatInt(c.Id, 10) }

func TestCreateListSortAndStatus(t *testing.T) {
	r := setup(t)
	r.create(t, map[string]any{"name": "没有日子的人"})
	r.create(t, map[string]any{"name": "老王", "group": "friend", "events": []map[string]any{event("birthday", "1990-"+r.md(3)[0:5])}})
	// the year above is built from the month and day only
	r.create(t, map[string]any{"name": "小李", "group": "colleague", "events": []map[string]any{event("anniversary", r.md(100))}})
	r.create(t, map[string]any{"name": "好久没联系", "lastContactOn": r.day(-100), "contactEveryDays": 30})
	r.create(t, map[string]any{"name": "刚联系过", "lastContactOn": r.day(-2), "contactEveryDays": 30})
	r.create(t, map[string]any{"name": "妈妈", "group": "family", "events": []map[string]any{event("birthday", r.md(1))}})

	out := r.list(t, "")
	var names []string
	for _, c := range out.Items {
		names = append(names, c.Name)
	}
	// near dates first, then overdue, then the rest, then nothing to track
	if got := strings.Join(names, ","); got != "妈妈,老王,好久没联系,小李,刚联系过,没有日子的人" {
		t.Fatalf("order: %s", got)
	}
	if out.Summary.Total != 6 || out.Summary.Soon != 2 || out.Summary.Overdue != 1 {
		t.Fatalf("summary: %+v", out.Summary)
	}
	mom := out.Items[0]
	if mom.Status != api.Soon || mom.NextEventIn == nil || *mom.NextEventIn != 1 || mom.Events[0].Label != "生日" || mom.Events[0].Years != nil {
		t.Fatalf("mom: %+v", mom)
	}
	wang := out.Items[1]
	if wang.Events[0].Years == nil || *wang.Events[0].Years != 36 {
		t.Fatalf("years: %+v", wang.Events[0])
	}
	lost := out.Items[2]
	if lost.Status != api.Overdue || lost.SinceContact == nil || *lost.SinceContact != 100 || lost.ContactDueIn == nil || *lost.ContactDueIn != -70 {
		t.Fatalf("lost: %+v", lost)
	}
	var none, fine api.Contact
	for _, c := range out.Items {
		switch c.Name {
		case "没有日子的人":
			none = c
		case "刚联系过":
			fine = c
		}
	}
	if none.Status != api.None || fine.Status != api.Ok || fine.ContactDueIn == nil || *fine.ContactDueIn != 28 {
		t.Fatalf("none/fine: %+v %+v", none, fine)
	}

	// filters
	if got := r.list(t, "?group=family"); len(got.Items) != 1 || got.Items[0].Name != "妈妈" || got.Summary.Total != 6 {
		t.Fatalf("group filter: %+v", got)
	}
	if got := r.list(t, "?q=%E8%80%81"); len(got.Items) != 1 || got.Items[0].Name != "老王" {
		t.Fatalf("q filter: %+v", got.Items)
	}
}

func TestLeapDayAndYearlyRollover(t *testing.T) {
	r := setup(t)
	// 2027 is not a leap year: 02-29 falls on 02-28
	c := r.create(t, map[string]any{"name": "闰日出生", "events": []map[string]any{event("birthday", "2000-02-29")}})
	if c.Events[0].NextOn != "2027-02-28" || c.Events[0].Years == nil || *c.Events[0].Years != 27 {
		t.Fatalf("leap: %+v", c.Events[0])
	}
	// a date that was yesterday this year comes next year; today counts as today
	today := r.create(t, map[string]any{"name": "今天", "events": []map[string]any{event("other", r.md(0))}})
	if today.Events[0].NextIn != 0 || today.Events[0].NextOn != "2026-10-10" {
		t.Fatalf("today: %+v", today.Events[0])
	}
	past := r.create(t, map[string]any{"name": "昨天", "events": []map[string]any{event("other", r.md(-1))}})
	if past.Events[0].NextOn != "2027-10-09" || past.Events[0].NextIn != 364 {
		t.Fatalf("past: %+v", past.Events[0])
	}
}

func TestValidation(t *testing.T) {
	r := setup(t)
	bad := map[string]map[string]any{
		"empty name":      {"name": " "},
		"long name":       {"name": strings.Repeat("长", 101)},
		"bad group":       {"name": "x", "group": "enemy"},
		"bad date":        {"name": "x", "events": []map[string]any{event("birthday", "13-40")}},
		"feb 30":          {"name": "x", "events": []map[string]any{event("birthday", "02-30")}},
		"year no zeros":   {"name": "x", "events": []map[string]any{event("birthday", "90-08-15")}},
		"two birthdays":   {"name": "x", "events": []map[string]any{event("birthday", "08-15"), event("birthday", "09-15")}},
		"bad kind":        {"name": "x", "events": []map[string]any{event("holiday", "08-15")}},
		"future contact":  {"name": "x", "lastContactOn": r.day(1)},
		"bad contact":     {"name": "x", "lastContactOn": "yesterday"},
		"bad every":       {"name": "x", "contactEveryDays": 4000},
		"bad remind":      {"name": "x", "remindDays": []int{0}},
		"remind too big":  {"name": "x", "remindDays": []int{400}},
		"too many remind": {"name": "x", "remindDays": []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
	}
	many := []map[string]any{}
	for i := 0; i < 21; i++ {
		many = append(many, event("other", "08-15"))
	}
	bad["too many events"] = map[string]any{"name": "x", "events": many}
	for name, body := range bad {
		if status, raw := r.env.Do(http.MethodPost, "/contacts", body, nil); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d: %s", name, status, raw)
		}
	}
	if got := r.list(t, ""); got.Summary.Total != 0 {
		t.Fatalf("a rejected request created something: %+v", got)
	}
}

func TestEventReminderOnceAndOnTheDay(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "老王", "events": []map[string]any{event("birthday", "1990-"+r.md(5))}})
	got := r.notices(t, "contacts.event")
	if len(got) != 1 || got[0].Title != "老王的生日还有 5 天（36 岁）" || got[0].Link != "/contacts" {
		t.Fatalf("first: %+v", got)
	}
	r.remind(t)
	r.remind(t)
	if got := r.notices(t, "contacts.event"); len(got) != 1 {
		t.Fatalf("repeated: %+v", got)
	}
	// two days before: day 1 has not come yet
	r.now = clock.AddDate(0, 0, 3)
	r.remind(t)
	if got := r.notices(t, "contacts.event"); len(got) != 1 {
		t.Fatalf("day 2: %+v", got)
	}
	r.now = clock.AddDate(0, 0, 4)
	r.remind(t)
	if got := r.notices(t, "contacts.event"); len(got) != 2 || got[0].Title != "老王的生日还有 1 天（36 岁）" {
		t.Fatalf("day 1: %+v", got)
	}
	r.now = clock.AddDate(0, 0, 5)
	r.remind(t)
	got = r.notices(t, "contacts.event")
	if len(got) != 3 || got[0].Title != "老王的生日是今天（36 岁）" {
		t.Fatalf("on the day: %+v", got)
	}
	// the day after, nothing until next year
	r.now = clock.AddDate(0, 0, 6)
	r.remind(t)
	if len(r.notices(t, "contacts.event")) != 3 {
		t.Fatal("reminded after the day")
	}
	_ = c
}

func TestChangingADateRemindsAgainAndRemovedDatesAreForgotten(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "小李", "events": []map[string]any{{"kind": "anniversary", "label": "结婚纪念日", "date": "2016-" + r.md(3)}}})
	if got := r.notices(t, "contacts.event"); len(got) != 1 || got[0].Title != "小李的结婚纪念日还有 3 天（10 周年）" {
		t.Fatalf("first: %+v", got)
	}
	var again api.Contact
	r.env.MustDo(http.MethodPatch, "/contacts/"+id(c), map[string]any{"events": []map[string]any{
		{"id": c.Events[0].Id, "kind": "anniversary", "label": "结婚纪念日", "date": "2016-" + r.md(2)},
	}}, &again)
	if got := r.notices(t, "contacts.event"); len(got) != 2 || got[0].Title != "小李的结婚纪念日还有 2 天（10 周年）" {
		t.Fatalf("after changing: %+v", got)
	}
	if again.Events[0].Id != c.Events[0].Id {
		t.Fatal("the id of a date must stay")
	}
	// saving without a change does not remind again
	r.env.MustDo(http.MethodPatch, "/contacts/"+id(c), map[string]any{"notes": "x"}, nil)
	if len(r.notices(t, "contacts.event")) != 2 {
		t.Fatal("reminded without a change")
	}
}

func TestLostContactReminderOnceThenTouch(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "老王", "lastContactOn": r.day(-40), "contactEveryDays": 30})
	got := r.notices(t, "contacts.lost")
	if len(got) != 1 || got[0].Title != "老王已经 40 天没联系了" || !strings.Contains(got[0].Body, r.day(-40)) {
		t.Fatalf("first: %+v", got)
	}
	r.now = clock.AddDate(0, 0, 60)
	r.remind(t)
	if len(r.notices(t, "contacts.lost")) != 1 {
		t.Fatal("must remind only once")
	}
	// touch: status back to ok, no reminder
	r.now = clock
	var v api.Contact
	r.env.MustDo(http.MethodPost, "/contacts/"+id(c)+"/touch", nil, &v)
	if v.Status != api.Ok || v.LastContactOn != r.day(0) || v.SinceContact == nil || *v.SinceContact != 0 {
		t.Fatalf("after touch: %+v", v)
	}
	r.remind(t)
	if len(r.notices(t, "contacts.lost")) != 1 {
		t.Fatal("reminded right after a touch")
	}
	// a new round of silence reminds again
	r.now = clock.AddDate(0, 0, 31)
	r.remind(t)
	if got := r.notices(t, "contacts.lost"); len(got) != 2 || got[0].Title != "老王已经 31 天没联系了" {
		t.Fatalf("second round: %+v", got)
	}
}

func TestNeverRecordedCountsFromCreation(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "新朋友", "contactEveryDays": 10})
	if c.Status != api.Ok || c.ContactDueIn == nil || *c.ContactDueIn != 10 || c.SinceContact != nil {
		t.Fatalf("new: %+v", c)
	}
	r.now = clock.AddDate(0, 0, 10)
	r.remind(t)
	got := r.notices(t, "contacts.lost")
	if len(got) != 1 || !strings.Contains(got[0].Body, "还没记过联系") {
		t.Fatalf("lost: %+v", got)
	}
}

func TestTouchRules(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "老王"})
	if status, _ := r.env.Do(http.MethodPost, "/contacts/"+id(c)+"/touch", map[string]any{"date": r.day(1)}, nil); status != http.StatusBadRequest {
		t.Fatalf("future touch: %d", status)
	}
	var v api.Contact
	r.env.MustDo(http.MethodPost, "/contacts/"+id(c)+"/touch", map[string]any{"date": r.day(-5)}, &v)
	if v.LastContactOn != r.day(-5) {
		t.Fatalf("touch: %+v", v)
	}
	if status, _ := r.env.Do(http.MethodPost, "/contacts/999/touch", nil, nil); status != http.StatusNotFound {
		t.Fatalf("missing: %d", status)
	}
}

func TestArchiveStopsRemindersAndDelete(t *testing.T) {
	r := setup(t)
	c := r.create(t, map[string]any{"name": "老王", "lastContactOn": r.day(-40), "contactEveryDays": 30, "events": []map[string]any{event("birthday", r.md(30))}})
	r.env.MustDo(http.MethodPatch, "/contacts/"+id(c), map[string]any{"archived": true}, nil)
	if got := r.list(t, ""); len(got.Items) != 0 || got.Summary.Total != 0 {
		t.Fatalf("archived listed: %+v", got)
	}
	if got := r.list(t, "?archived=true"); len(got.Items) != 1 || !got.Items[0].Archived {
		t.Fatalf("archived: %+v", got)
	}
	n := len(r.notices(t, "contacts.lost"))
	r.now = clock.AddDate(0, 0, 100)
	r.remind(t)
	if len(r.notices(t, "contacts.lost")) != n {
		t.Fatal("archived reminded")
	}
	if status, _ := r.env.Do(http.MethodDelete, "/contacts/"+id(c), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := r.env.Do(http.MethodGet, "/contacts/"+id(c), nil, nil); status != http.StatusNotFound {
		t.Fatalf("after delete: %d", status)
	}
	if status, _ := r.env.Do(http.MethodDelete, "/contacts/"+id(c), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestHiddenModuleGate(t *testing.T) {
	r := setup(t)
	r.create(t, map[string]any{"name": "老王", "lastContactOn": r.day(-40), "contactEveryDays": 30})
	r.env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	r.env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"contacts"}}, nil)
	if status, _ := r.env.Do(http.MethodGet, "/contacts", nil, nil); status != 200 {
		t.Fatalf("unlocked: %d", status)
	}
	r.env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := r.env.Do(http.MethodGet, "/contacts", nil, nil); status != 404 {
		t.Fatalf("locked list: %d", status)
	}
	if got := r.notices(t, "contacts.lost"); len(got) != 0 {
		t.Fatalf("locked notifications must be hidden: %+v", got)
	}
	var avail struct{ Modules []string }
	r.env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, m := range avail.Modules {
		if m == "contacts" {
			t.Fatalf("contacts still available: %v", avail.Modules)
		}
	}
	for _, name := range []string{"contacts.list", "contacts.touch"} {
		if _, err := r.env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(`{"name":"老王"}`)); err == nil {
			t.Fatalf("%s must be hidden while the module is locked", name)
		}
	}
}

func TestAIActions(t *testing.T) {
	r := setup(t)
	r.create(t, map[string]any{"name": "老王", "events": []map[string]any{event("birthday", r.md(10))}})
	r.create(t, map[string]any{"name": "小李", "events": []map[string]any{event("birthday", r.md(100))}})
	r.create(t, map[string]any{"name": "重名"})
	r.create(t, map[string]any{"name": "重名"})
	run := func(name, args string) (string, error) {
		out, err := r.env.App.Deps.Actions.Run(context.Background(), name, json.RawMessage(args))
		raw, _ := json.Marshal(out)
		return string(raw), err
	}
	out, err := run("contacts.list", `{"days":30}`)
	if err != nil || !strings.Contains(out, "老王") || strings.Contains(out, "小李") {
		t.Fatalf("within 30 days: %s %v", out, err)
	}
	if out, err = run("contacts.list", `{"q":"小"}`); err != nil || !strings.Contains(out, "小李") || strings.Contains(out, "老王") {
		t.Fatalf("q: %s %v", out, err)
	}
	if out, err = run("contacts.touch", `{"name":"老王","date":"`+r.day(-3)+`"}`); err != nil || !strings.Contains(out, `"lastContactOn":"`+r.day(-3)+`"`) {
		t.Fatalf("touch: %s %v", out, err)
	}
	if _, err = run("contacts.touch", `{"name":"重名"}`); err == nil {
		t.Fatal("a name shared by two contacts must be refused")
	}
	if _, err = run("contacts.touch", `{"name":"没有这个人"}`); err == nil {
		t.Fatal("an unknown name must be refused")
	}
	if _, err = run("contacts.touch", `{}`); err == nil {
		t.Fatal("neither name nor id must be refused")
	}
}
