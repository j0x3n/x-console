package ws

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type agentCaller interface {
	Call(context.Context, string, string, any, any) error
}

// defaultDetailMs is the interval of a host that is viewed by someone who
// did not ask for a particular one.
const defaultDetailMs = 5000

// Handler shares detail demand across browser connections.
type Handler struct {
	bus    *events.Bus
	agents agentCaller
	mu     sync.Mutex
	refs   map[string]int
	timers map[string]*time.Timer
	calls  map[string]*sync.Mutex
	grace  time.Duration

	nextConn  int64
	wants     map[int64]map[string]int // connection -> host -> interval asked for
	sent      map[string]int           // host -> interval the agent was last told, 0 when off
	announced map[string]int           // host -> interval last published on the bus

	Allow func(ctx context.Context, topic string, data any) bool
}

func New(bus *events.Bus, agents agentCaller) *Handler {
	return &Handler{bus: bus, agents: agents, refs: map[string]int{}, timers: map[string]*time.Timer{}, calls: map[string]*sync.Mutex{},
		grace: time.Minute, wants: map[int64]map[string]int{}, sent: map[string]int{}, announced: map[string]int{}}
}

type command struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics"`
	HostID string   `json:"hostId"`
	Ms     int      `json:"ms"`
}

// validInterval reports whether ms is an interval a browser may ask for.
func validInterval(ms int) bool { return ms == 1000 || ms == 5000 || ms == 30000 }

func validTopics(in []string) map[string]bool {
	out := map[string]bool{}
	for _, topic := range in {
		if len(out) >= 64 {
			break
		}
		if topic == "host.metrics" || topic == "ha." ||
			(strings.HasPrefix(topic, "host.metrics:") && len(topic) > len("host.metrics:")) ||
			(strings.HasPrefix(topic, "coding_task.output:") && len(topic) > len("coding_task.output:")) {
			out[topic] = true
		}
	}
	return out
}

func wanted(ev events.Event, topics map[string]bool, paused bool) bool {
	switch ev.Topic {
	case "host.metrics":
		if paused {
			return false
		}
		var data struct {
			HostID string `json:"hostId"`
		}
		decodeData(ev.Data, &data)
		return topics["host.metrics"] || (data.HostID != "" && topics["host.metrics:"+data.HostID])
	case "host.metrics_interval":
		return false // for the hosts module only
	case "ha.state_changed":
		return !paused && topics["ha."]
	case "coding_task.output":
		if paused {
			return false
		}
		var data struct {
			TaskID int64 `json:"taskId"`
		}
		decodeData(ev.Data, &data)
		return data.TaskID > 0 && topics["coding_task.output:"+strconv.FormatInt(data.TaskID, 10)]
	default:
		return true
	}
}

func decodeData(data any, out any) {
	b, err := json.Marshal(data)
	if err == nil {
		_ = json.Unmarshal(b, out)
	}
}

func agentID(data any) string {
	var x struct {
		AgentID string `json:"agentId"`
	}
	decodeData(data, &x)
	return x.AgentID
}

func subscribedAgent(topics map[string]bool, data any) bool {
	id := agentID(data)
	return id != "" && topics["host.metrics:"+id]
}

func (h *Handler) updateDetail(old, next map[string]bool) {
	for topic := range old {
		if strings.HasPrefix(topic, "host.metrics:") && !next[topic] {
			h.changeDetail(strings.TrimPrefix(topic, "host.metrics:"), -1)
		}
	}
	for topic := range next {
		if strings.HasPrefix(topic, "host.metrics:") && !old[topic] {
			h.changeDetail(strings.TrimPrefix(topic, "host.metrics:"), 1)
		}
	}
}

func (h *Handler) changeDetail(id string, delta int) {
	h.mu.Lock()
	prev := h.refs[id]
	h.refs[id] += delta
	if h.refs[id] < 0 {
		h.refs[id] = 0
	}
	timer := h.timers[id]
	if timer != nil {
		timer.Stop()
		delete(h.timers, id)
	}
	on := delta > 0 && prev == 0 && timer == nil
	if h.refs[id] == 0 {
		delete(h.refs, id)
		h.timers[id] = time.AfterFunc(h.grace, func() {
			h.mu.Lock()
			if h.refs[id] != 0 {
				h.mu.Unlock()
				return
			}
			delete(h.timers, id)
			h.mu.Unlock()
			h.setDetail(id, false)
		})
	}
	h.mu.Unlock()
	if on {
		h.setDetail(id, true)
	}
}

// newConn hands out an id for a browser connection.
func (h *Handler) newConn() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextConn++
	return h.nextConn
}

// effective is the interval a viewed host runs at: the shortest one any
// connection asked for, or the default. Caller holds the lock.
func (h *Handler) effective(id string) int {
	best := 0
	for _, byHost := range h.wants {
		if ms := byHost[id]; ms > 0 && (best == 0 || ms < best) {
			best = ms
		}
	}
	if best == 0 {
		return defaultDetailMs
	}
	return best
}

// setWants replaces what one connection asks for and tells the agents whose
// interval changed as a result. Pass nil when the connection leaves or pauses.
func (h *Handler) setWants(conn int64, next map[string]int) {
	h.mu.Lock()
	changed := map[string]bool{}
	for host, ms := range h.wants[conn] {
		if next[host] != ms {
			changed[host] = true
		}
	}
	for host, ms := range next {
		if h.wants[conn][host] != ms {
			changed[host] = true
		}
	}
	if len(next) == 0 {
		delete(h.wants, conn)
	} else {
		h.wants[conn] = next
	}
	var hosts []string
	for host := range changed {
		if h.refs[host] > 0 {
			hosts = append(hosts, host)
		}
	}
	h.mu.Unlock()
	for _, host := range hosts {
		h.setDetail(host, true)
	}
}

// forget drops what we know about an agent's state, for when it reconnects.
func (h *Handler) forget(id string) {
	h.mu.Lock()
	delete(h.sent, id)
	h.mu.Unlock()
}

func (h *Handler) setDetail(id string, on bool) {
	if strings.HasPrefix(id, "ssh:") {
		return
	}
	h.mu.Lock()
	lock := h.calls[id]
	if lock == nil {
		lock = &sync.Mutex{}
		h.calls[id] = lock
	}
	h.mu.Unlock()
	go func() {
		lock.Lock()
		defer lock.Unlock()
		h.mu.Lock()
		wanted := (h.refs[id] > 0) == on
		ms := h.effective(id)
		if !on {
			ms = 0
		}
		last, known := h.sent[id]
		skip := known && last == ms
		announce := wanted && h.announced[id] != ms
		if announce {
			h.announced[id] = ms
		}
		h.mu.Unlock()
		if !wanted {
			return
		}
		if announce {
			// The hosts module reads this to decide how often to push samples.
			h.bus.Publish("host.metrics_interval", map[string]any{"hostId": id, "ms": ms})
		}
		if skip {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		params := protocol.MetricsDetailParams{On: on}
		if on {
			params.IntervalMs = ms
		}
		if err := h.agents.Call(ctx, id, protocol.MethodMetricsDetail, params, nil); err == nil {
			h.mu.Lock()
			h.sent[id] = ms
			h.mu.Unlock()
		}
	}()
}
