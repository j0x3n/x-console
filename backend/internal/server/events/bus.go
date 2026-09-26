// Package events is the in-process publish/subscribe bus.
//
// Modules publish domain events (for example "issue.updated"). The browser
// WebSocket forwards every event to the UI so it can refresh queries, and the
// automation engine (M12) subscribes to use them as triggers.
package events

import (
	"strings"
	"sync"
	"time"
)

// Event is one published message. Topic is "<entity>.<verb>", for example
// "host.metrics", "reminder.due", "coding_task.updated".
type Event struct {
	Topic string    `json:"topic"`
	Data  any       `json:"data,omitempty"`
	At    time.Time `json:"at"`
}

// Bus fans events out to subscribers. Slow subscribers drop events rather
// than block publishers.
type Bus struct {
	mu   sync.RWMutex
	next int
	subs map[int]*sub
}

type sub struct {
	prefix string
	ch     chan Event
}

// NewBus builds an empty bus.
func NewBus() *Bus { return &Bus{subs: map[int]*sub{}} }

// Publish sends an event to every subscriber whose prefix matches the topic.
func (b *Bus) Publish(topic string, data any) {
	ev := Event{Topic: topic, Data: data, At: time.Now().UTC()}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, s := range b.subs {
		if s.prefix != "" && !strings.HasPrefix(topic, s.prefix) {
			continue
		}
		select {
		case s.ch <- ev:
		default:
		}
	}
}

// Subscribe returns a channel of events whose topic starts with prefix
// ("" means all) and a cancel function that must be called when done.
func (b *Bus) Subscribe(prefix string, buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	s := &sub{prefix: prefix, ch: make(chan Event, buffer)}
	b.mu.Lock()
	id := b.next
	b.next++
	b.subs[id] = s
	b.mu.Unlock()
	var once sync.Once
	return s.ch, func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, id)
			b.mu.Unlock()
			close(s.ch)
		})
	}
}
