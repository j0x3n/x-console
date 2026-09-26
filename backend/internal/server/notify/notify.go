// Package notify creates notifications and delivers them.
//
// Every notification is stored for the in-app notification center. M7 adds
// external channels (Web Push, Telegram, Bark, ServerChan) by registering
// Channel implementations and a Router that decides which channels get what.
package notify

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// Priorities.
const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"
)

// Action is a button shown on channels that support it (Telegram, Web Push).
// ID is routed to the ActionHandler registered for its prefix, for example
// "reminder.done:42" goes to the handler registered for "reminder.".
type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Notification is what modules send.
type Notification struct {
	Kind     string         // event type, for example "reminder.due"
	Title    string
	Body     string
	Link     string         // in-app path, for example "/reminders"
	Priority string         // defaults to normal
	Source   string         // module name
	Actions  []Action
	Data     map[string]any
}

// Stored is a notification after it was saved.
type Stored struct {
	ID        int64
	CreatedAt time.Time
	Notification
}

// Channel delivers notifications somewhere outside the app.
type Channel interface {
	Name() string
	Send(ctx context.Context, n Stored) error
}

// Router picks the external channels for a notification. The default sends
// nothing outside the app.
type Router interface {
	Route(ctx context.Context, n Stored) []string
}

// ActionHandler handles a notification button press.
type ActionHandler func(ctx context.Context, actionID string) error

// Service stores and dispatches notifications.
type Service struct {
	q   *db.Queries
	bus *events.Bus

	mu       sync.RWMutex
	channels map[string]Channel
	router   Router
	actions  map[string]ActionHandler
}

// New builds a Service.
func New(conn *sql.DB, bus *events.Bus) *Service {
	return &Service{q: db.New(conn), bus: bus, channels: map[string]Channel{}, actions: map[string]ActionHandler{}}
}

// RegisterChannel adds an external channel.
func (s *Service) RegisterChannel(c Channel) {
	s.mu.Lock()
	s.channels[c.Name()] = c
	s.mu.Unlock()
}

// Channel returns a registered channel, used for "send test message" buttons.
func (s *Service) Channel(name string) (Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.channels[name]
	return c, ok
}

// ChannelNames lists registered channels.
func (s *Service) ChannelNames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.channels))
	for name := range s.channels {
		out = append(out, name)
	}
	return out
}

// SetRouter replaces the routing policy.
func (s *Service) SetRouter(r Router) {
	s.mu.Lock()
	s.router = r
	s.mu.Unlock()
}

// OnAction registers the handler for action ids starting with prefix.
func (s *Service) OnAction(prefix string, h ActionHandler) {
	s.mu.Lock()
	s.actions[prefix] = h
	s.mu.Unlock()
}

// HandleAction runs the handler registered for actionID.
func (s *Service) HandleAction(ctx context.Context, actionID string) error {
	s.mu.RLock()
	var best string
	var h ActionHandler
	for prefix, fn := range s.actions {
		if strings.HasPrefix(actionID, prefix) && len(prefix) > len(best) {
			best, h = prefix, fn
		}
	}
	s.mu.RUnlock()
	if h == nil {
		return httpx.ErrNotFound
	}
	return h(ctx, actionID)
}

// Send stores n, pushes it to the browser and delivers it to routed channels
// in the background.
func (s *Service) Send(ctx context.Context, n Notification) (Stored, error) {
	if n.Priority == "" {
		n.Priority = PriorityNormal
	}
	data := map[string]any{}
	for k, v := range n.Data {
		data[k] = v
	}
	if len(n.Actions) > 0 {
		data["actions"] = n.Actions
	}
	raw, _ := json.Marshal(data)
	row, err := s.q.InsertNotification(ctx, db.InsertNotificationParams{
		CreatedAt: time.Now().UTC(), Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link,
		Priority: n.Priority, Source: n.Source, Data: string(raw),
	})
	if err != nil {
		return Stored{}, err
	}
	stored := Stored{ID: row.ID, CreatedAt: row.CreatedAt, Notification: n}
	s.bus.Publish("notification.created", ToAPI(row))

	s.mu.RLock()
	router := s.router
	s.mu.RUnlock()
	if router != nil {
		go s.deliver(context.WithoutCancel(ctx), router, stored)
	}
	return stored, nil
}

func (s *Service) deliver(ctx context.Context, router Router, n Stored) {
	for _, name := range router.Route(ctx, n) {
		c, ok := s.Channel(name)
		if !ok {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := c.Send(sendCtx, n)
		cancel()
		if err != nil {
			slog.Warn("notification delivery failed", "channel", name, "id", n.ID, "err", err)
			s.bus.Publish("notification.delivery_failed", map[string]any{"id": n.ID, "channel": name, "error": err.Error()})
		}
	}
}

// Queries exposes the generated queries to the core HTTP handlers.
func (s *Service) Queries() *db.Queries { return s.q }

// API is the JSON shape of a notification (matches the OpenAPI schema).
type API struct {
	ID        int64      `json:"id"`
	CreatedAt time.Time  `json:"createdAt"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	Link      string     `json:"link,omitempty"`
	Priority  string     `json:"priority"`
	Source    string     `json:"source"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
}

// ToAPI converts a row.
func ToAPI(n db.Notification) API {
	return API{ID: n.ID, CreatedAt: n.CreatedAt, Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link,
		Priority: n.Priority, Source: n.Source, ReadAt: n.ReadAt}
}
