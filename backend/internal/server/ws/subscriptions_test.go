package ws

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type detailCall struct {
	id string
	on bool
}

type fakeAgents struct{ calls chan detailCall }

func (f *fakeAgents) Call(_ context.Context, id, method string, params, _ any) error {
	if method == protocol.MethodMetricsDetail {
		f.calls <- detailCall{id, params.(protocol.MetricsDetailParams).On}
	}
	return nil
}

func expectCall(t *testing.T, calls <-chan detailCall, id string, on bool) {
	t.Helper()
	select {
	case got := <-calls:
		if got != (detailCall{id, on}) {
			t.Fatalf("detail call: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("missing detail call")
	}
}

func readEvent(t *testing.T, c *websocket.Conn) events.Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var ev events.Event
	if err := wsjson.Read(ctx, c, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestSubscriptionsAndPause(t *testing.T) {
	bus := events.NewBus()
	agents := &fakeAgents{calls: make(chan detailCall, 8)}
	h := New(bus, agents)
	h.grace = 20 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close(websocket.StatusNormalClosure, "")

	if err := wsjson.Write(context.Background(), c, command{Type: "subscribe", Topics: []string{"host.metrics:one"}}); err != nil {
		t.Fatal(err)
	}
	expectCall(t, agents.calls, "one", true)
	bus.Publish("host.metrics", map[string]any{"hostId": "two"})
	bus.Publish("reminder.due", map[string]any{"id": 1})
	if ev := readEvent(t, c); ev.Topic != "reminder.due" {
		t.Fatalf("unexpected event: %s", ev.Topic)
	}
	bus.Publish("host.metrics", map[string]any{"hostId": "one"})
	if ev := readEvent(t, c); ev.Topic != "host.metrics" {
		t.Fatalf("unexpected event: %s", ev.Topic)
	}

	if err := wsjson.Write(context.Background(), c, command{Type: "pause"}); err != nil {
		t.Fatal(err)
	}
	expectCall(t, agents.calls, "one", false)
	bus.Publish("host.metrics", map[string]any{"hostId": "one"})
	bus.Publish("host.alert.fired", map[string]any{"hostId": "one"})
	if ev := readEvent(t, c); ev.Topic != "host.alert.fired" {
		t.Fatalf("pause dropped alert: %s", ev.Topic)
	}
	if err := wsjson.Write(context.Background(), c, command{Type: "resume"}); err != nil {
		t.Fatal(err)
	}
	expectCall(t, agents.calls, "one", true)
}

func TestWantedHighVolumeTopics(t *testing.T) {
	topics := validTopics([]string{"ha.", "coding_task.output:42", "coding_task.output:", "unknown"})
	if len(topics) != 2 {
		t.Fatalf("topics: %+v", topics)
	}
	for _, tc := range []struct {
		topic string
		data  any
		want  bool
	}{
		{"ha.state_changed", nil, true},
		{"coding_task.output", map[string]any{"taskId": 42}, true},
		{"coding_task.output", map[string]any{"taskId": 43}, false},
		{"host.metrics", map[string]any{"hostId": "one"}, false},
		{"notification.created", nil, true},
	} {
		if got := wanted(events.Event{Topic: tc.topic, Data: tc.data}, topics, false); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.topic, got, tc.want)
		}
	}
}

func TestDetailDemandWaitsForLastViewer(t *testing.T) {
	agents := &fakeAgents{calls: make(chan detailCall, 8)}
	h := New(events.NewBus(), agents)
	h.grace = 25 * time.Millisecond
	h.changeDetail("one", 1)
	expectCall(t, agents.calls, "one", true)
	h.changeDetail("one", 1)
	h.changeDetail("one", -1)
	select {
	case call := <-agents.calls:
		t.Fatalf("stopped while another viewer remained: %+v", call)
	case <-time.After(2 * h.grace):
	}
	h.changeDetail("one", -1)
	h.changeDetail("one", 1)
	select {
	case call := <-agents.calls:
		t.Fatalf("stopped during grace period: %+v", call)
	case <-time.After(2 * h.grace):
	}
	h.changeDetail("one", -1)
	expectCall(t, agents.calls, "one", false)
}
