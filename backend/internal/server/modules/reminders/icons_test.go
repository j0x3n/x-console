package reminders_test

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

func TestSignedPushIconIsPublicAndCached(t *testing.T) {
	env, m := setup(t)
	n := notify.Stored{Notification: notify.Notification{Kind: "habit.reminder", Data: map[string]any{"habitId": int64(12), "icon": "💧", "done": 3.0, "target": 8.0}}}
	icon, badge := reminders.PushIcons(m, context.Background(), n)
	if !strings.Contains(icon, "habit-12-progress-3-8-water.png?sig=") || badge != "/icons/notify/habit-badge.png" {
		t.Fatal(icon, badge)
	}
	get := func(path string) (int, []byte) {
		t.Helper()
		response, err := http.Get(env.Server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode == 200 && response.Header.Get("Cache-Control") != "public, max-age=86400" {
			t.Fatal(response.Header)
		}
		return response.StatusCode, raw
	}
	if s, _ := get(strings.Split(icon, "?sig=")[0] + "?sig=00"); s != 403 {
		t.Fatalf("bad signature: %d", s)
	}
	before := reminders.IconDraws(m)
	s, raw := get(icon)
	if s != 200 {
		t.Fatalf("no session: %d %s", s, raw)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil || img.Bounds().Dx() != 192 || img.Bounds().Dy() != 192 {
		t.Fatalf("PNG: %v %v", img, err)
	}
	_, _, _, a := img.At(0, 0).RGBA()
	if a != 0 {
		t.Fatal("corner is not transparent")
	}
	_, again := get(icon)
	if !bytes.Equal(raw, again) || reminders.IconDraws(m) != before+1 {
		t.Fatal("not cached")
	}
	n.Data["done"] = 0.0
	zero, _ := reminders.PushIcons(m, context.Background(), n)
	_, empty := get(zero)
	if bytes.Equal(raw, empty) {
		t.Fatal("progress ring does not change")
	}
	n.Data["target"] = 0.00001
	small, _ := reminders.PushIcons(m, context.Background(), n)
	if s, _ := get(small); s != 200 {
		t.Fatalf("fractional target: %d", s)
	}
	for _, emoji := range []string{"📖", "🌙", "💪", "🍎", "🚴", "🧘", "🦷", "🎸"} {
		n.Data = map[string]any{"icon": emoji}
		u, _ := reminders.PushIcons(m, context.Background(), n)
		if s, _ := get(u); s != 200 {
			t.Errorf("%s: %d", emoji, s)
		}
	}
}

func TestReminderIconSaveAndPushChoice(t *testing.T) {
	env, m := setup(t)
	var r api.Reminder
	env.MustDo("POST", "/reminders", map[string]any{"title": "喝水", "at": time.Now().Add(time.Hour), "icon": "💧"}, &r)
	if r.Icon == nil || *r.Icon != "💧" {
		t.Fatal(r)
	}
	n := notify.Stored{Notification: notify.Notification{Kind: "reminder.due", Data: map[string]any{"reminderId": r.Id}}}
	icon, _ := reminders.PushIcons(m, context.Background(), n)
	if !strings.Contains(icon, "emoji-1f4a7.png") {
		t.Fatal(icon)
	}
	env.MustDo("PATCH", fmt.Sprintf("/reminders/%d", r.Id), map[string]any{"icon": "未支持的图标"}, &r)
	icon, _ = reminders.PushIcons(m, context.Background(), n)
	if icon != "/icons/notify/reminder.png" {
		t.Fatal(icon)
	}
	n.Notification = notify.Notification{Kind: "habit.reminder", Data: map[string]any{"template": "water"}}
	icon, _ = reminders.PushIcons(m, context.Background(), n)
	if icon != "/icons/notify/water.png" {
		t.Fatal(icon)
	}
	n.Notification = notify.Notification{Kind: "repo.push"}
	icon, badge := reminders.PushIcons(m, context.Background(), n)
	if icon != "/icons/notify/repo.png" || badge != "/icons/notify/repo-badge.png" {
		t.Fatal(icon, badge)
	}
	for kind, want := range map[string]string{"coding_task.review": "agent", "forgejo.push": "repo", "subscription.due": "monitor", "backup.failed": "drive", "ha.alert": "home"} {
		n.Notification = notify.Notification{Kind: kind}
		icon, badge = reminders.PushIcons(m, context.Background(), n)
		if icon != "/icons/notify/"+want+".png" || badge != "/icons/notify/"+want+"-badge.png" {
			t.Errorf("%s: %s %s", kind, icon, badge)
		}
	}
}
