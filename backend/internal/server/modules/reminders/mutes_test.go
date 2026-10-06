package reminders_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// twoDevices subscribes two Web Push devices ("phone" and "desktop") against a
// fake push service and returns their subscription ids.
func twoDevices(t *testing.T, env *testutil.Env) (push *fakeService, phone, desktop int64) {
	t.Helper()
	push = newFake(t, func(string) (int, string) { return http.StatusCreated, "" })
	subscribe := func(path, ua string) {
		priv, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		auth := make([]byte, 16)
		_, _ = rand.Read(auth)
		env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", map[string]any{
			"endpoint": push.URL + path, "userAgent": ua,
			"keys": map[string]string{"p256dh": b64(priv.PublicKey().Bytes()), "auth": b64(auth)},
		}, nil)
	}
	subscribe("/push/phone", "iPhone Safari")
	subscribe("/push/desktop", "Windows Chrome")
	var subs []api.WebPushSubscriptionInfo
	env.MustDo(http.MethodGet, "/notify/webpush/subscriptions", nil, &subs)
	for _, s := range subs {
		switch {
		case strings.HasSuffix(s.Endpoint, "/push/phone"):
			phone = s.Id
		case strings.HasSuffix(s.Endpoint, "/push/desktop"):
			desktop = s.Id
		}
	}
	if phone == 0 || desktop == 0 {
		t.Fatalf("subscriptions: %+v", subs)
	}
	return push, phone, desktop
}

func send(t *testing.T, env *testutil.Env, kind, scope string) {
	t.Helper()
	_, err := env.App.Deps.Notify.Send(context.Background(), notify.Notification{Kind: kind, Scope: scope, Title: "测试 " + kind, Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
}

func device(id int64) string { return fmt.Sprintf("webpush:%d", id) }

func bellCount(t *testing.T, env *testutil.Env, kind string) int {
	t.Helper()
	var out struct{ Items []struct{ Kind string } }
	env.MustDo(http.MethodGet, "/notifications?limit=100", nil, &out)
	n := 0
	for _, x := range out.Items {
		if x.Kind == kind {
			n++
		}
	}
	return n
}

func TestMuteKeepsOneKindOfOneScopeFromOneDevice(t *testing.T) {
	env, _ := setup(t)
	push, phone, _ := twoDevices(t, env)

	var m api.NotifyMute
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:7", "target": device(phone)}, &m)
	if m.Id == 0 || m.Scope != "mail:7" || m.Target != device(phone) {
		t.Fatalf("created: %+v", m)
	}

	// mailbox 7 reaches the desktop, not the phone; the bell has it
	send(t, env, "mail.new", "mail:7")
	waitFor(t, "desktop push", func() bool { return len(push.find("/push/desktop")) == 1 })
	time.Sleep(150 * time.Millisecond)
	if n := len(push.find("/push/phone")); n != 0 {
		t.Fatalf("the phone must not get mailbox 7: %d", n)
	}
	if bellCount(t, env, "mail.new") != 1 {
		t.Fatal("the bell is never muted")
	}

	// another mailbox, and another kind, still reach the phone
	send(t, env, "mail.new", "mail:8")
	send(t, env, "host.alert", "")
	waitFor(t, "phone push", func() bool { return len(push.find("/push/phone")) == 2 })
	waitFor(t, "desktop push 3", func() bool { return len(push.find("/push/desktop")) == 3 })
}

func TestMuteWholeChannelAndKindWildcard(t *testing.T) {
	env, _ := setup(t)
	push, _, _ := twoDevices(t, env)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "github.*", "target": "webpush"}, nil)
	send(t, env, "github.ci_failed", "")
	send(t, env, "mail.new", "mail:1")
	waitFor(t, "mail pushes", func() bool { return len(push.find("/push/phone")) == 1 && len(push.find("/push/desktop")) == 1 })
	time.Sleep(150 * time.Millisecond)
	if n := len(push.find("/push/phone")) + len(push.find("/push/desktop")); n != 2 {
		t.Fatalf("github.* must not be pushed: %d calls", n)
	}
	if bellCount(t, env, "github.ci_failed") != 1 {
		t.Fatal("the bell is never muted")
	}
}

func TestReplaceScopeMutesAndList(t *testing.T) {
	env, _ := setup(t)
	_, phone, desktop := twoDevices(t, env)
	body := func(targets ...string) map[string]any {
		return map[string]any{"kindPattern": "mail.new", "scope": "mail:3", "targets": targets}
	}
	var out struct{ Items []api.NotifyMute }
	env.MustDo(http.MethodPut, "/notify/mutes/scope", body(device(phone), "bark"), &out)
	if len(out.Items) != 2 {
		t.Fatalf("two targets: %+v", out.Items)
	}
	// another scope is untouched
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:4", "target": "bark"}, nil)
	env.MustDo(http.MethodPut, "/notify/mutes/scope", body(device(desktop), device(desktop)), &out)
	if len(out.Items) != 1 || out.Items[0].Target != device(desktop) {
		t.Fatalf("replaced: %+v", out.Items)
	}
	env.MustDo(http.MethodPut, "/notify/mutes/scope", body(), &out)
	if len(out.Items) != 0 {
		t.Fatalf("emptied: %+v", out.Items)
	}
	var all, one struct{ Items []api.NotifyMute }
	env.MustDo(http.MethodGet, "/notify/mutes", nil, &all)
	env.MustDo(http.MethodGet, "/notify/mutes?scope=mail:4", nil, &one)
	if len(all.Items) != 1 || len(one.Items) != 1 || one.Items[0].Scope != "mail:4" {
		t.Fatalf("all %+v one %+v", all.Items, one.Items)
	}
	if status, _ := env.Do(http.MethodPut, "/notify/mutes/scope", map[string]any{"kindPattern": "mail.new", "scope": "", "targets": []string{}}, nil); status != http.StatusBadRequest {
		t.Fatalf("empty scope: %d", status)
	}
}

func TestMuteValidationDuplicatesAndDelete(t *testing.T) {
	env, _ := setup(t)
	_, phone, _ := twoDevices(t, env)
	bad := []map[string]any{
		{"kindPattern": "", "target": "bark"},
		{"kindPattern": "Mail New", "target": "bark"},
		{"kindPattern": "mail.new", "scope": "nonsense", "target": "bark"},
		{"kindPattern": "mail.new", "target": "pigeon"},
		{"kindPattern": "mail.new", "target": "webpush:9999"},
		{"kindPattern": "mail.new", "target": "webpush:x"},
		{"kindPattern": "[a-", "target": "bark"},
	}
	for _, b := range bad {
		if status, raw := env.Do(http.MethodPost, "/notify/mutes", b, nil); status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
			t.Errorf("%v: %d %s", b, status, raw)
		}
	}
	ok := map[string]any{"kindPattern": "mail.new", "scope": "mail:1", "target": device(phone)}
	var m api.NotifyMute
	env.MustDo(http.MethodPost, "/notify/mutes", ok, &m)
	if status, _ := env.Do(http.MethodPost, "/notify/mutes", ok, nil); status != http.StatusConflict {
		t.Fatalf("duplicate: %d", status)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notify/mutes/%d", m.Id), nil, nil)
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notify/mutes/%d", m.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("delete twice: %d", status)
	}
}

func TestMutesGoWithTheirDeviceAndScope(t *testing.T) {
	env, _ := setup(t)
	_, phone, desktop := twoDevices(t, env)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:5", "target": device(phone)}, nil)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:5", "target": device(desktop)}, nil)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:6", "target": "bark"}, nil)

	// deleting a device takes its rules away
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notify/webpush/subscriptions/%d", phone), nil, nil)
	var got struct{ Items []api.NotifyMute }
	env.MustDo(http.MethodGet, "/notify/mutes?scope=mail:5", nil, &got)
	if len(got.Items) != 1 || got.Items[0].Target != device(desktop) {
		t.Fatalf("after device delete: %+v", got.Items)
	}

	// a deleted mailbox announces its scope and its rules go
	env.App.Deps.Bus.Publish("notify.scope_removed", map[string]any{"scope": "mail:5"})
	waitFor(t, "scope rules removed", func() bool {
		env.MustDo(http.MethodGet, "/notify/mutes", nil, &got)
		return len(got.Items) == 1 && got.Items[0].Scope == "mail:6"
	})
}

func TestNewDeviceReceivesByDefault(t *testing.T) {
	env, _ := setup(t)
	push, phone, _ := twoDevices(t, env)
	env.MustDo(http.MethodPost, "/notify/mutes", map[string]any{"kindPattern": "mail.new", "scope": "mail:7", "target": device(phone)}, nil)
	priv, _ := ecdh.P256().GenerateKey(rand.Reader)
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	env.MustDo(http.MethodPost, "/notify/webpush/subscriptions", map[string]any{
		"endpoint": push.URL + "/push/tablet", "userAgent": "iPad",
		"keys": map[string]string{"p256dh": b64(priv.PublicKey().Bytes()), "auth": b64(auth)},
	}, nil)
	send(t, env, "mail.new", "mail:7")
	waitFor(t, "tablet push", func() bool { return len(push.find("/push/tablet")) == 1 })
}
