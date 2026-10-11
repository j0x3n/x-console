package reminders_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/app"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B127: notification digest.

func tgTexts(tg *fakeService) []string {
	var out []string
	for _, c := range tg.find("/sendMessage") {
		var b struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal([]byte(c.Body), &b)
		out = append(out, b.Text)
	}
	return out
}

func putDigest(t *testing.T, env *testutil.Env, enabled bool, times []string, modes map[string]api.DigestMode) api.DigestStatus {
	t.Helper()
	if times == nil {
		times = []string{"11:00", "15:00", "19:00"}
	}
	if modes == nil {
		modes = map[string]api.DigestMode{}
	}
	var out api.DigestStatus
	env.MustDo(http.MethodPut, "/notify/digest", api.DigestSettings{Enabled: enabled, Times: times, Modes: modes}, &out)
	return out
}

func getDigest(t *testing.T, env *testutil.Env) api.DigestStatus {
	t.Helper()
	var out api.DigestStatus
	env.MustDo(http.MethodGet, "/notify/digest", nil, &out)
	return out
}

func sendDigest(t *testing.T, env *testutil.Env, n notify.Notification) {
	t.Helper()
	if _, err := env.App.Deps.Notify.Send(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func mail(title string) notify.Notification {
	return notify.Notification{Kind: "mail.new", Title: title, Link: "/mail", Source: "mail"}
}

func digestEnv(t *testing.T) (*testutil.Env, *reminders.Module, *fakeService) {
	env, m := setup(t)
	tg := fakeTelegram(t)
	configureTelegram(t, env, tg)
	return env, m, tg
}

// settle gives background deliveries time to arrive before a test asserts
// that something was not sent.
func settle() { time.Sleep(150 * time.Millisecond) }

func TestDigestOffSendsNow(t *testing.T) {
	env, _, tg := digestEnv(t)
	st := getDigest(t, env)
	if st.Enabled || st.Pending != 0 || len(st.Times) != 3 || st.Times[0] != "11:00" || len(st.Categories) != 10 {
		t.Fatalf("defaults: %+v", st)
	}
	sendDigest(t, env, mail("新邮件"))
	waitFor(t, "telegram", func() bool { return len(tgTexts(tg)) == 1 })
	if st := getDigest(t, env); st.Pending != 0 {
		t.Fatalf("pending: %+v", st)
	}
}

func TestDigestHoldsOnlyWhatShouldWait(t *testing.T) {
	env, _, tg := digestEnv(t)
	putDigest(t, env, true, nil, nil)

	sendDigest(t, env, mail("可以等的邮件"))
	sendDigest(t, env, notify.Notification{Kind: "mail.new", Title: "紧急邮件", Priority: notify.PriorityUrgent})
	sendDigest(t, env, notify.Notification{Kind: "mail.new", Title: "标了立即", Immediate: true})
	sendDigest(t, env, notify.Notification{Kind: "mail.new", Title: "带按钮", Actions: []notify.Action{{ID: "reminder.done:1", Label: "完成"}}})
	sendDigest(t, env, notify.Notification{Kind: "host.alert", Title: "服务器告警", Priority: notify.PriorityHigh})
	sendDigest(t, env, notify.Notification{Kind: "unknown.thing", Title: "未知的高优先级", Priority: notify.PriorityHigh})
	sendDigest(t, env, notify.Notification{Kind: "unknown.thing", Title: "未知的普通通知"})

	waitFor(t, "five immediate messages", func() bool { return len(tgTexts(tg)) == 5 })
	settle()
	texts := strings.Join(tgTexts(tg), "\n")
	if len(tgTexts(tg)) != 5 || strings.Contains(texts, "可以等的邮件") || strings.Contains(texts, "未知的普通通知") {
		t.Fatalf("telegram: %q", texts)
	}
	if st := getDigest(t, env); st.Pending != 2 || st.OldestAt == nil || st.NextAt == nil {
		t.Fatalf("status: %+v", st)
	}
	// The notification center has everything at once.
	var list struct{ Items []struct{ Title string } }
	env.MustDo(http.MethodGet, "/notifications", nil, &list)
	if len(list.Items) != 7 {
		t.Fatalf("in-app: %+v", list.Items)
	}
}

func TestDigestCategoryModes(t *testing.T) {
	env, _, tg := digestEnv(t)
	st := putDigest(t, env, true, nil, map[string]api.DigestMode{"alert": api.Digest, "mail": api.Now})
	modes := map[string]api.DigestMode{}
	for _, c := range st.Categories {
		modes[c.Id] = c.Mode
		if c.Id == "alert" && c.DefaultMode != api.Now {
			t.Fatalf("alert default: %+v", c)
		}
	}
	if modes["alert"] != api.Digest || modes["mail"] != api.Now || modes["github"] != api.Digest || modes["reminder"] != api.Now {
		t.Fatalf("modes: %v", modes)
	}
	sendDigest(t, env, notify.Notification{Kind: "host.alert", Title: "告警被攒下", Priority: notify.PriorityHigh})
	sendDigest(t, env, mail("邮件立即发"))
	waitFor(t, "mail", func() bool { return len(tgTexts(tg)) == 1 })
	settle()
	if got := tgTexts(tg); len(got) != 1 || !strings.Contains(got[0], "邮件立即发") {
		t.Fatalf("telegram: %v", got)
	}
	if st := getDigest(t, env); st.Pending != 1 {
		t.Fatalf("pending: %+v", st)
	}
}

func TestDigestDecision(t *testing.T) {
	stored := func(kind, prio string, actions ...notify.Action) notify.Stored {
		return notify.Stored{Notification: notify.Notification{Kind: kind, Priority: prio, Actions: actions}}
	}
	cases := []struct {
		kind, prio string
		wantCat    string
		wantHold   bool
	}{
		{"host.alert", "high", "alert", false},
		{"monitor.down", "high", "alert", false},
		{"router.wan_down", "high", "alert", false},
		{"ai_agent.decision", "normal", "agent_decision", false},
		{"ai_agent.done", "normal", "agent_progress", true},
		{"coding_task.review", "normal", "agent_progress", true},
		{"mail.new", "normal", "mail", true},
		{"github.ci_failed", "normal", "github", true},
		{"issue.due", "normal", "project", true},
		{"reminder.due", "normal", "reminder", false},
		{"habit.reminder", "normal", "reminder", false},
		{"weather.rain", "normal", "brief", false},
		{"automation.failed", "normal", "other", true},
		{"automation.failed", "high", "other", false},
		{"mail.new", "urgent", "mail", false},
	}
	for _, c := range cases {
		cat, hold := reminders.DigestDecision(true, nil, stored(c.kind, c.prio))
		if cat != c.wantCat || hold != c.wantHold {
			t.Errorf("%s/%s: got %s %v, want %s %v", c.kind, c.prio, cat, hold, c.wantCat, c.wantHold)
		}
		if reminders.DigestCategoryOf(c.kind) != c.wantCat {
			t.Errorf("category of %s", c.kind)
		}
	}
	if _, hold := reminders.DigestDecision(false, nil, stored("mail.new", "normal")); hold {
		t.Error("held while turned off")
	}
	if _, hold := reminders.DigestDecision(true, nil, stored("mail.new", "normal", notify.Action{ID: "x:1"})); hold {
		t.Error("notification with buttons was held")
	}
}

func TestDigestSlots(t *testing.T) {
	times := []string{"19:00", "11:00", "15:00"}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 12, h, m, 0, 0, shanghai) }
	latest, ok := reminders.LatestSlot(times, at(12, 0), shanghai)
	if !ok || !latest.Equal(at(11, 0)) {
		t.Fatalf("latest at 12:00: %v", latest)
	}
	latest, _ = reminders.LatestSlot(times, at(11, 0), shanghai)
	if !latest.Equal(at(11, 0)) {
		t.Fatalf("latest at 11:00 sharp: %v", latest)
	}
	latest, _ = reminders.LatestSlot(times, at(2, 0), shanghai)
	if !latest.Equal(at(19, 0).AddDate(0, 0, -1)) {
		t.Fatalf("latest at 02:00: %v", latest)
	}
	next, _ := reminders.NextSlot(times, at(12, 0), shanghai)
	if !next.Equal(at(15, 0)) {
		t.Fatalf("next at 12:00: %v", next)
	}
	next, _ = reminders.NextSlot(times, at(20, 0), shanghai)
	if !next.Equal(at(11, 0).AddDate(0, 0, 1)) {
		t.Fatalf("next at 20:00: %v", next)
	}
	if _, ok := reminders.LatestSlot(nil, at(12, 0), shanghai); ok {
		t.Fatal("slot without times")
	}
}

func TestDigestSendsAtTimePoints(t *testing.T) {
	env, m, tg := digestEnv(t)
	ctx := context.Background()
	putDigest(t, env, true, nil, nil)
	day := time.Now().In(shanghai)
	at := func(h, min int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), h, min, 0, 0, shanghai)
	}
	tick := func(now time.Time) {
		t.Helper()
		if err := reminders.DigestTick(m, ctx, now); err != nil {
			t.Fatal(err)
		}
	}

	tick(at(10, 30)) // first run only remembers where we are
	sendDigest(t, env, mail("邮件甲"))
	sendDigest(t, env, mail("邮件乙"))
	tick(at(10, 45))
	if len(tgTexts(tg)) != 0 {
		t.Fatalf("sent before the time point: %v", tgTexts(tg))
	}
	tick(at(11, 0))
	got := tgTexts(tg)
	if len(got) != 1 || !strings.Contains(got[0], "通知汇总：2 条") || !strings.Contains(got[0], "邮件甲") || !strings.Contains(got[0], "邮件乙") {
		t.Fatalf("11:00 digest: %v", got)
	}
	if st := getDigest(t, env); st.Pending != 0 || st.LastSentAt == nil {
		t.Fatalf("status after send: %+v", st)
	}
	tick(at(11, 5))
	if len(tgTexts(tg)) != 1 {
		t.Fatalf("sent twice for one time point: %v", tgTexts(tg))
	}

	sendDigest(t, env, mail("邮件丙"))
	tick(at(11, 30))
	if len(tgTexts(tg)) != 1 {
		t.Fatalf("sent between time points: %v", tgTexts(tg))
	}
	tick(at(15, 0))
	got = tgTexts(tg)
	if len(got) != 2 || !strings.Contains(got[1], "通知汇总：1 条") || !strings.Contains(got[1], "邮件丙") {
		t.Fatalf("15:00 digest: %v", got)
	}

	tick(at(19, 0)) // empty queue: nothing to send
	if len(tgTexts(tg)) != 2 {
		t.Fatalf("empty digest was sent: %v", tgTexts(tg))
	}
}

func TestDigestWaitsOutQuietHours(t *testing.T) {
	env, m, tg := digestEnv(t)
	ctx := context.Background()
	// The tests fake "now" 12 hours away from the real clock, because the
	// channel router checks quiet hours against the real clock.
	base := time.Now().In(shanghai).Truncate(time.Minute)
	fake := base.Add(12 * time.Hour)
	slot := fake.Add(-30 * time.Minute).Format("15:04")
	putDigest(t, env, true, []string{slot}, nil)
	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{
		Enabled: true, Start: fake.Add(-time.Hour).Format("15:04"), End: fake.Add(time.Hour).Format("15:04"),
	}, nil)

	if err := reminders.DigestTick(m, ctx, fake.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	sendDigest(t, env, mail("夜里的邮件"))
	if err := reminders.DigestTick(m, ctx, fake); err != nil { // time point passed, inside quiet hours
		t.Fatal(err)
	}
	if len(tgTexts(tg)) != 0 || getDigest(t, env).Pending != 1 {
		t.Fatalf("sent during quiet hours: %v", tgTexts(tg))
	}
	if err := reminders.DigestTick(m, ctx, fake.Add(2*time.Hour)); err != nil { // quiet hours over
		t.Fatal(err)
	}
	if got := tgTexts(tg); len(got) != 1 || !strings.Contains(got[0], "夜里的邮件") {
		t.Fatalf("after quiet hours: %v", got)
	}
}

func TestDigestSurvivesRestart(t *testing.T) {
	env, _, tg := digestEnv(t)
	putDigest(t, env, true, nil, nil)
	sendDigest(t, env, mail("重启前的邮件"))
	if getDigest(t, env).Pending != 1 {
		t.Fatal("not queued")
	}

	// A second app on the same database is what a restarted server sees.
	second, err := app.New(env.App.Deps.Config, env.App.Deps.DB)
	if err != nil {
		t.Fatal(err)
	}
	svc, ok := module.Lookup[contracts.Reminders](second.Deps.Registry, contracts.RemindersKey)
	if !ok {
		t.Fatal("no reminders module")
	}
	m2 := svc.(*reminders.Module)
	res, err := reminders.FlushDigest(m2, context.Background(), time.Now(), true)
	if err != nil || res.Count != 1 || res.Delivered != 1 {
		t.Fatalf("flush after restart: %+v %v", res, err)
	}
	if got := tgTexts(tg); len(got) != 1 || !strings.Contains(got[0], "重启前的邮件") {
		t.Fatalf("telegram: %v", got)
	}
	if getDigest(t, env).Pending != 0 {
		t.Fatal("queue not emptied")
	}
}

func TestBuildDigestContent(t *testing.T) {
	now := time.Date(2026, 10, 12, 11, 0, 0, 0, shanghai)
	var items []db.NotifyDigestQueue
	add := func(cat, title, link string, age time.Duration, prio string) {
		items = append(items, db.NotifyDigestQueue{ID: int64(len(items) + 1), CreatedAt: now.Add(-age).UTC(), Kind: cat + ".x", Title: title, Link: link, Priority: prio, Category: cat})
	}
	for i := 1; i <= 8; i++ {
		add("mail", fmt.Sprintf("邮件%d", i), "/mail", time.Duration(9-i)*time.Minute, "normal")
	}
	add("github", "流水线失败", "/github", time.Minute, "high")
	add("other", "很早以前的通知", "", 72*time.Hour, "normal")

	n := reminders.BuildDigest(items, now, shanghai, func(l string) string {
		if l == "" {
			return ""
		}
		return "https://x.test" + l
	})
	if n.Kind != "notify.digest" || n.Title != "通知汇总：10 条" || n.Priority != notify.PriorityHigh || n.Immediate {
		t.Fatalf("header: %+v", n)
	}
	for _, want := range []string{
		"【邮件】8 条", "邮件8", "邮件4", "另有 3 条没有列出", "https://x.test/mail",
		"【GitHub】1 条", "流水线失败", "【其他】1 条", "很早以前的通知（10-09 11:00）",
		"其中 1 条已经积压超过 36 小时",
	} {
		if !strings.Contains(n.Body, want) {
			t.Errorf("body misses %q:\n%s", want, n.Body)
		}
	}
	if strings.Contains(n.Body, "邮件3") || strings.Contains(n.Body, "邮件1\n") {
		t.Errorf("older mails should be cut:\n%s", n.Body)
	}

	// Many categories: at most 25 titles in total, the rest only counted.
	items = nil
	for _, cat := range []string{"alert", "reminder", "brief", "agent_progress", "mail", "github", "project"} {
		for i := 1; i <= 5; i++ {
			add(cat, fmt.Sprintf("%s-%d", cat, i), "", time.Minute, "normal")
		}
	}
	n = reminders.BuildDigest(items, now, shanghai, func(string) string { return "" })
	if got := strings.Count(n.Body, "· "); got != 25 {
		t.Errorf("listed %d titles, want 25:\n%s", got, n.Body)
	}
	if !strings.Contains(n.Body, "另有 5 条没有列出") || !strings.Contains(n.Body, "【项目】5 条") {
		t.Errorf("truncation notice:\n%s", n.Body)
	}
}

func TestDigestManualFlush(t *testing.T) {
	env, _, tg := digestEnv(t)
	putDigest(t, env, true, nil, nil)

	var res api.DigestFlushResult
	env.MustDo(http.MethodPost, "/notify/digest/flush", nil, &res)
	if res.Count != 0 || len(tgTexts(tg)) != 0 {
		t.Fatalf("flush of an empty queue: %+v %v", res, tgTexts(tg))
	}

	sendDigest(t, env, mail("甲"))
	sendDigest(t, env, mail("乙"))
	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{
		Enabled: true, Start: time.Now().In(shanghai).Add(-time.Hour).Format("15:04"), End: time.Now().In(shanghai).Add(time.Hour).Format("15:04"),
	}, nil)
	status, raw := env.Do(http.MethodPost, "/notify/digest/flush", nil, nil)
	if status != http.StatusConflict || !strings.Contains(string(raw), "quiet_hours") {
		t.Fatalf("flush in quiet hours: %d %s", status, raw)
	}
	if getDigest(t, env).Pending != 2 {
		t.Fatal("queue changed by a refused flush")
	}

	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{Enabled: false, Start: "23:00", End: "07:30"}, nil)
	env.MustDo(http.MethodPost, "/notify/digest/flush", nil, &res)
	if res.Count != 2 || res.Delivered != 1 {
		t.Fatalf("flush: %+v", res)
	}
	if got := tgTexts(tg); len(got) != 1 || !strings.Contains(got[0], "甲") || !strings.Contains(got[0], "乙") {
		t.Fatalf("telegram: %v", got)
	}
	if getDigest(t, env).Pending != 0 {
		t.Fatal("queue not emptied")
	}
}

func TestDigestValidation(t *testing.T) {
	env, _ := setup(t)
	bad := []api.DigestSettings{
		{Enabled: true, Times: []string{"25:00"}, Modes: map[string]api.DigestMode{}},
		{Enabled: true, Times: []string{"11:00", "11:00"}, Modes: map[string]api.DigestMode{}},
		{Enabled: true, Times: []string{}, Modes: map[string]api.DigestMode{}},
		{Enabled: true, Times: []string{"1:00", "2:00", "3:00", "4:00", "5:00", "6:00", "7:00"}, Modes: map[string]api.DigestMode{}},
		{Enabled: true, Times: []string{"11:00"}, Modes: map[string]api.DigestMode{"nope": api.Now}},
		{Enabled: true, Times: []string{"11:00"}, Modes: map[string]api.DigestMode{"mail": "later"}},
	}
	for i, in := range bad {
		if status, raw := env.Do(http.MethodPut, "/notify/digest", in, nil); status != http.StatusBadRequest {
			t.Errorf("case %d: %d %s", i, status, raw)
		}
	}
	st := putDigest(t, env, true, []string{"19:00", "09:05"}, nil)
	if len(st.Times) != 2 || st.Times[0] != "09:05" || st.Times[1] != "19:00" {
		t.Fatalf("times are not sorted: %v", st.Times)
	}
}

func TestDigestKeepsQueueWhenEveryChannelFails(t *testing.T) {
	env, m := setup(t)
	var healthy atomic.Bool
	tg := newFake(t, func(string) (int, string) {
		if healthy.Load() {
			return 200, `{"ok":true,"result":{}}`
		}
		return 500, `{"ok":false,"description":"boom"}`
	})
	configureTelegram(t, env, tg)
	putDigest(t, env, true, nil, nil)
	sendDigest(t, env, mail("要重试的邮件"))

	ctx := context.Background()
	now := time.Now()
	if _, err := reminders.FlushDigest(m, ctx, now, true); err == nil {
		t.Fatal("flush succeeded although telegram failed")
	}
	if getDigest(t, env).Pending != 1 {
		t.Fatal("queue lost after a failed delivery")
	}
	// The scheduler backs off before it tries again.
	calls := len(tg.find("/sendMessage"))
	if _, err := reminders.FlushDigest(m, ctx, now.Add(time.Minute), false); err == nil {
		t.Fatal("no back-off after a failure")
	}
	if len(tg.find("/sendMessage")) != calls {
		t.Fatal("tried again too early")
	}

	healthy.Store(true)
	res, err := reminders.FlushDigest(m, ctx, now, true)
	if err != nil || res.Count != 1 || res.Delivered != 1 {
		t.Fatalf("retry: %+v %v", res, err)
	}
}

func TestDigestWithoutChannelsClearsQueue(t *testing.T) {
	env, m := setup(t)
	putDigest(t, env, true, nil, nil)
	sendDigest(t, env, mail("没有渠道"))
	res, err := reminders.FlushDigest(m, context.Background(), time.Now(), true)
	if err != nil || res.Count != 1 || res.Delivered != 0 {
		t.Fatalf("flush: %+v %v", res, err)
	}
	if getDigest(t, env).Pending != 0 {
		t.Fatal("queue not emptied")
	}
}

func TestDigestTurnedOffSendsWhatIsLeft(t *testing.T) {
	env, m, tg := digestEnv(t)
	putDigest(t, env, true, nil, nil)
	sendDigest(t, env, mail("关开关前的邮件"))
	putDigest(t, env, false, nil, nil)
	if err := reminders.DigestTick(m, context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := tgTexts(tg); len(got) != 1 || !strings.Contains(got[0], "关开关前的邮件") {
		t.Fatalf("telegram: %v", got)
	}
}

func TestDigestQueueIsBounded(t *testing.T) {
	env, _ := setup(t)
	putDigest(t, env, true, nil, nil)
	for i := 0; i < 1005; i++ {
		sendDigest(t, env, mail(fmt.Sprintf("邮件 %d", i)))
	}
	if st := getDigest(t, env); st.Pending != 1000 {
		t.Fatalf("pending: %d", st.Pending)
	}
}

// B113 and B127 together: a muted notification must not come back as a digest line.
func TestDigestSkipsMutedNotifications(t *testing.T) {
	env, _, tg := digestEnv(t)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "target": "telegram"}, nil)
	putDigest(t, env, true, nil, nil)
	sendDigest(t, env, mail("被静音的邮件"))
	sendDigest(t, env, notify.Notification{Kind: "unknown.thing", Title: "没静音的"})
	settle()
	if st := getDigest(t, env); st.Pending != 1 {
		t.Fatalf("pending: %+v", st)
	}
	if got := tgTexts(tg); len(got) != 0 {
		t.Fatalf("telegram: %q", got)
	}
}
