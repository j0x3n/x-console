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

// Handler shares detail demand across browser connections.
type Handler struct {
	bus    *events.Bus
	agents agentCaller
	mu     sync.Mutex
	refs   map[string]int
	timers map[string]*time.Timer
	calls  map[string]*sync.Mutex
	grace  time.Duration
}

func New(bus *events.Bus, agents agentCaller) *Handler {
	return &Handler{bus: bus, agents: agents, refs: map[string]int{}, timers: map[string]*time.Timer{}, calls: map[string]*sync.Mutex{}, grace: time.Minute}
}

type command struct {
	Type   string   `json:"type"`
	Topics []string `json:"topics"`
}

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
		h.mu.Unlock()
		if !wanted {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = h.agents.Call(ctx, id, protocol.MethodMetricsDetail, protocol.MetricsDetailParams{On: on}, nil)
	}()
}
