package reminders_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B164: notifications by state.

func setMe(t *testing.T, env *testutil.Env, state string) {
	t.Helper()
	env.MustDo(http.MethodPut, "/me/manual", map[string]any{"state": state, "duration": "forever"}, nil)
}

// mailWhenWorking makes the mail category wait for the computer.
func mailWhenWorking(t *testing.T, env *testutil.Env, enabled bool) {
	t.Helper()
	putDigest(t, env, enabled, nil, map[string]api.DigestMode{"mail": api.Working})
}

func TestHoldFor(t *testing.T) {
	modes := map[string]string{"mail": "working"}
	n := func(kind, prio string, actions ...notify.Action) notify.Stored {
		return notify.Stored{Notification: notify.Notification{Kind: kind, Priority: prio, Actions: actions}}
	}
	for _, c := range []struct {
		name      string
		enabled   bool
		outDirect bool
		n         notify.Stored
		state     string
		want      string
	}{
		{"away waits", false, true, n("mail.new", "normal"), "away", "working"},
		{"working sends", false, true, n("mail.new", "normal"), "working", ""},
		{"out sends", false, true, n("mail.new", "normal"), "out", ""},
		{"out waits when asked", false, false, n("mail.new", "normal"), "out", "working"},
		{"sleeping waits", true, true, n("mail.new", "normal"), "sleeping", "working"},
		{"urgent sends", false, true, n("mail.new", "urgent"), "away", ""},
		{"buttons send", false, true, n("mail.new", "normal", notify.Action{ID: "x:1", Label: "x"}), "away", ""},
		{"no me module sends", false, true, n("mail.new", "normal"), "", ""},
		{"digest category", true, true, n("github.ci", "normal"), "away", "digest"},
		{"digest off", false, true, n("github.ci", "normal"), "away", ""},
	} {
		if got := reminders.HoldFor(c.enabled, modes, c.outDirect, c.n, c.state); got != c.want {
			t.Errorf("%s: %q", c.name, got)
		}
	}
}

func TestWorkingWaitsForTheComputer(t *testing.T) {
	env, m, tg := digestEnv(t)
	mailWhenWorking(t, env, false) // the digest switch is off, "working" still applies
	setMe(t, env, "away")

	sendDigest(t, env, mail("不在时的信"))
	settle()
	if n := len(tg.find("/sendMessage")); n != 0 {
		t.Fatalf("sent while away: %d", n)
	}
	if st := getDigest(t, env); st.PendingWorking != 1 || st.Pending != 0 || !st.OutDirect {
		t.Fatalf("status: %+v", st)
	}
	if bellCount(t, env, "mail.new") != 1 {
		t.Fatal("the bell gets it at once")
	}
	// A digest time point does not send what waits for the computer.
	putDigest(t, env, true, []string{"11:00"}, map[string]api.DigestMode{"mail": api.Working})
	ctx := context.Background()
	day := time.Date(2026, 10, 1, 10, 0, 0, 0, shanghai)
	_ = reminders.DigestTick(m, ctx, day)
	_ = reminders.DigestTick(m, ctx, day.Add(90*time.Minute))
	if st := getDigest(t, env); st.PendingWorking != 1 {
		t.Fatalf("time point sent the working queue: %+v", st)
	}

	// Back at work (set by hand, so no computer is known): one merged message.
	setMe(t, env, "working")
	waitFor(t, "merged message", func() bool { return len(tg.find("/sendMessage")) == 1 })
	if texts := tgTexts(tg); !strings.Contains(texts[0], "通知汇总：1 条") || !strings.Contains(texts[0], "不在时的信") {
		t.Fatalf("message: %v", texts)
	}
	waitFor(t, "queue emptied", func() bool { return getDigest(t, env).PendingWorking == 0 })

	// While working it goes out at once.
	sendDigest(t, env, mail("在时的信"))
	waitFor(t, "sent at once", func() bool { return len(tg.find("/sendMessage")) == 2 })
}

func TestHeldNotificationsPopUpOnTheComputer(t *testing.T) {
	env, m, tg := digestEnv(t)
	mailWhenWorking(t, env, false)
	setMe(t, env, "away")
	shown := make(chan string, 8)
	host := env.Agent("desktop", []string{"notify.show"}, func(c *conn.Client) {
		c.Handle("notify.show", func(_ context.Context, raw json.RawMessage) (any, error) {
			var in struct{ Title, Body string }
			_ = json.Unmarshal(raw, &in)
			shown <- in.Title + "\n" + in.Body
			return map[string]bool{"shown": true}, nil
		})
	})
	ctx := context.Background()
	hold := func(n int) {
		for i := range n {
			sendDigest(t, env, mail("信 "+string(rune('A'+i))))
		}
		waitFor(t, "queued", func() bool { return getDigest(t, env).PendingWorking == n })
	}
	receive := func() string {
		t.Helper()
		select {
		case s := <-shown:
			return s
		case <-time.After(3 * time.Second):
			t.Fatal("no system notification")
		}
		return ""
	}

	// Two: one by one.
	hold(2)
	if err := reminders.DeliverHeld(m, ctx, host, time.Now()); err != nil {
		t.Fatal(err)
	}
	if a, b := receive(), receive(); !strings.HasPrefix(a, "信 A") || !strings.HasPrefix(b, "信 B") {
		t.Fatalf("shown: %q %q", a, b)
	}
	// Five: one summary with the first three titles.
	hold(5)
	if err := reminders.DeliverHeld(m, ctx, host, time.Now()); err != nil {
		t.Fatal(err)
	}
	if s := receive(); !strings.HasPrefix(s, "你不在时有 5 条通知") || !strings.Contains(s, "信 C") || strings.Contains(s, "信 D") {
		t.Fatalf("summary: %q", s)
	}
	settle()
	if len(shown) != 0 || len(tg.find("/sendMessage")) != 0 || getDigest(t, env).PendingWorking != 0 {
		t.Fatal("the computer showed them; nothing else goes out")
	}

	// A computer without system notifications: the channels get them.
	plain := env.Agent("desktop", nil, nil)
	hold(1)
	if err := reminders.DeliverHeld(m, ctx, plain, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "channel fallback", func() bool { return len(tg.find("/sendMessage")) == 1 })
}

func TestOutDirectAndDayLimit(t *testing.T) {
	env, m, tg := digestEnv(t)
	mailWhenWorking(t, env, false)
	setMe(t, env, "out")
	sendDigest(t, env, mail("外出"))
	waitFor(t, "out sends at once", func() bool { return len(tg.find("/sendMessage")) == 1 })

	off := false
	env.MustDo(http.MethodPut, "/notify/digest", api.DigestSettings{Times: []string{"11:00"}, Modes: map[string]api.DigestMode{"mail": api.Working}, OutDirect: &off}, nil)
	if getDigest(t, env).OutDirect {
		t.Fatal("outDirect not saved")
	}
	// Not sent: an update keeps it.
	mailWhenWorking(t, env, false)
	if getDigest(t, env).OutDirect {
		t.Fatal("outDirect lost on update")
	}
	sendDigest(t, env, mail("外出攒着"))
	waitFor(t, "queued", func() bool { return getDigest(t, env).PendingWorking == 1 })

	ctx := context.Background()
	if err := reminders.FlushStaleHeld(m, ctx, time.Now().Add(23*time.Hour)); err != nil || getDigest(t, env).PendingWorking != 1 {
		t.Fatalf("sent before a day: %v", err)
	}
	if err := reminders.FlushStaleHeld(m, ctx, time.Now().Add(25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "day limit", func() bool { return len(tg.find("/sendMessage")) == 2 })
	if getDigest(t, env).PendingWorking != 0 {
		t.Fatal("not cleared")
	}
}

func TestClaimHeldOnThePhone(t *testing.T) {
	env, _, tg := digestEnv(t)
	mailWhenWorking(t, env, false)
	setMe(t, env, "away")
	sendDigest(t, env, mail("手机上看"))
	sendDigest(t, env, mail("第二封"))
	waitFor(t, "queued", func() bool { return getDigest(t, env).PendingWorking == 2 })
	var out struct{ Items []api.HeldNotification }
	env.MustDo(http.MethodPost, "/notify/digest/claim", nil, &out)
	if len(out.Items) != 2 || out.Items[0].Title != "手机上看" || out.Items[0].Id == 0 || out.Items[0].Link != "/mail" {
		t.Fatalf("claimed: %+v", out.Items)
	}
	env.MustDo(http.MethodPost, "/notify/digest/claim", nil, &out)
	if len(out.Items) != 0 || getDigest(t, env).PendingWorking != 0 {
		t.Fatalf("claimed twice: %+v", out.Items)
	}
	setMe(t, env, "working")
	settle()
	if len(tg.find("/sendMessage")) != 0 {
		t.Fatal("claimed items must not go out again")
	}
}

func TestSleepingCountsAsQuiet(t *testing.T) {
	env, m, _ := digestEnv(t)
	var q api.QuietHours
	env.MustDo(http.MethodGet, "/notify/quiet-hours", nil, &q)
	if q.WhenAsleep == nil || !*q.WhenAsleep {
		t.Fatalf("default: %+v", q)
	}
	setMe(t, env, "sleeping")
	ctx := context.Background()
	r := reminders.Router(m)
	normal := notify.Stored{Notification: notify.Notification{Kind: "test.x", Title: "普通"}}
	urgent := notify.Stored{Notification: notify.Notification{Kind: "test.x", Title: "紧急", Priority: notify.PriorityUrgent}}
	if got := r.Route(ctx, normal); len(got) != 0 {
		t.Fatalf("normal while asleep: %v", got)
	}
	if got := r.Route(ctx, urgent); len(got) != 1 {
		t.Fatalf("urgent while asleep: %v", got)
	}
	// Turned off: sleeping no longer counts. Updates without it keep it.
	off := false
	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{Start: "23:00", End: "07:30", WhenAsleep: &off}, &q)
	env.MustDo(http.MethodPut, "/notify/quiet-hours", api.QuietHours{Start: "23:00", End: "07:30"}, &q)
	if q.WhenAsleep == nil || *q.WhenAsleep {
		t.Fatalf("kept: %+v", q)
	}
	if got := r.Route(ctx, normal); len(got) != 1 {
		t.Fatalf("normal with the switch off: %v", got)
	}
}
