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
	ms int
}

type fakeAgents struct{ calls chan detailCall }

func (f *fakeAgents) Call(_ context.Context, id, method string, params, _ any) error {
	if method == protocol.MethodMetricsDetail {
		p := params.(protocol.MetricsDetailParams)
		f.calls <- detailCall{id, p.On, p.IntervalMs}
	}
	return nil
}

// expectCall waits for an "on" call with the default interval, or an "off" call.
func expectCall(t *testing.T, calls <-chan detailCall, id string, on bool) {
	t.Helper()
	ms := 0
	if on {
		ms = 5000
	}
	expectInterval(t, calls, id, on, ms)
}

func expectInterval(t *testing.T, calls <-chan detailCall, id string, on bool, ms int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != (detailCall{id, on, ms}) {
			t.Fatalf("detail call: %+v, want %+v", got, detailCall{id, on, ms})
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("missing detail call %+v", detailCall{id, on, ms})
	}
}

func expectNoCall(t *testing.T, calls <-chan detailCall) {
	t.Helper()
	select {
	case got := <-calls:
		t.Fatalf("unexpected detail call: %+v", got)
	case <-time.After(150 * time.Millisecond):
	}
}

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(websocket.StatusNormalClosure, "") })
	return c
}

func send(t *testing.T, c *websocket.Conn, cmd command) {
	t.Helper()
	if err := wsjson.Write(context.Background(), c, cmd); err != nil {
		t.Fatal(err)
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
	c := dial(t, srv)

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

func TestIntervalIsTheShortestAsked(t *testing.T) {
	bus := events.NewBus()
	agents := &fakeAgents{calls: make(chan detailCall, 16)}
	h := New(bus, agents)
	h.grace = 20 * time.Millisecond
	srv := httptest.NewServer(h)
	defer srv.Close()
	events2, cancel := bus.Subscribe("host.metrics_interval", 16)
	defer cancel()

	slow := dial(t, srv)
	send(t, slow, command{Type: "subscribe", Topics: []string{"host.metrics:one"}})
	expectInterval(t, agents.calls, "one", true, 5000)
	send(t, slow, command{Type: "interval", HostID: "one", Ms: 30000})
	expectInterval(t, agents.calls, "one", true, 30000)

	fast := dial(t, srv)
	send(t, fast, command{Type: "subscribe", Topics: []string{"host.metrics:one"}})
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 1000})
	expectInterval(t, agents.calls, "one", true, 1000)

	// The same interval again does not call the agent again.
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 1000})
	expectNoCall(t, agents.calls)

	// A paused browser does not count, and counts again after resume.
	send(t, fast, command{Type: "pause"})
	expectInterval(t, agents.calls, "one", true, 30000)
	send(t, fast, command{Type: "resume"})
	expectInterval(t, agents.calls, "one", true, 1000)

	// Values that are not 1000, 5000 or 30000 mean "not asking".
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 2000})
	expectInterval(t, agents.calls, "one", true, 30000)
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 1000})
	expectInterval(t, agents.calls, "one", true, 1000)
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 0})
	expectInterval(t, agents.calls, "one", true, 30000)
	send(t, fast, command{Type: "interval", HostID: "one", Ms: 1000})
	expectInterval(t, agents.calls, "one", true, 1000)

	// When the fast browser leaves, the slow one decides again.
	fast.Close(websocket.StatusNormalClosure, "")
	expectInterval(t, agents.calls, "one", true, 30000)

	// The hosts module hears about every change of the effective interval.
	var seen []int
	for len(events2) > 0 {
		ev := <-events2
		seen = append(seen, ev.Data.(map[string]any)["ms"].(int))
	}
	if len(seen) == 0 || seen[len(seen)-1] != 30000 {
		t.Fatalf("interval events: %v", seen)
	}

	// The last viewer leaves: detail mode ends after the grace period and the
	// hosts module is told to go back to its normal cadence.
	slow.Close(websocket.StatusNormalClosure, "")
	expectCall(t, agents.calls, "one", false)
	select {
	case ev := <-events2:
		if ms := ev.Data.(map[string]any)["ms"].(int); ms != 0 {
			t.Fatalf("interval after leaving: %d", ms)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no interval event after the last viewer left")
	}
}

func TestAgentReconnectGetsTheIntervalAgain(t *testing.T) {
	bus := events.NewBus()
	agents := &fakeAgents{calls: make(chan detailCall, 16)}
	h := New(bus, agents)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := dial(t, srv)
	send(t, c, command{Type: "subscribe", Topics: []string{"host.metrics:one"}})
	expectInterval(t, agents.calls, "one", true, 5000)
	send(t, c, command{Type: "interval", HostID: "one", Ms: 1000})
	expectInterval(t, agents.calls, "one", true, 1000)

	bus.Publish("agent.online", map[string]any{"agentId": "one"})
	expectInterval(t, agents.calls, "one", true, 1000)
	// Another agent coming online is not our business.
	bus.Publish("agent.online", map[string]any{"agentId": "two"})
	expectNoCall(t, agents.calls)
}

func TestIntervalOnlyCountsForWatchedHosts(t *testing.T) {
	agents := &fakeAgents{calls: make(chan detailCall, 16)}
	h := New(events.NewBus(), agents)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := dial(t, srv)
	// Asking before watching is remembered and used once the host is watched.
	send(t, c, command{Type: "interval", HostID: "one", Ms: 1000})
	expectNoCall(t, agents.calls)
	send(t, c, command{Type: "subscribe", Topics: []string{"host.metrics:one"}})
	expectInterval(t, agents.calls, "one", true, 1000)
}
