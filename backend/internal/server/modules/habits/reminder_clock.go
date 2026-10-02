package habits

import (
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

type reminderClock struct {
	mu     sync.Mutex
	states map[int64]activityReminder
}

func newReminderClock() *reminderClock {
	return &reminderClock{states: map[int64]activityReminder{}}
}

func (c *reminderClock) evaluate(id int64, rules scheduleRules, policy reminderPolicy, now time.Time, hosts []contracts.HostPresence, commit bool) (bool, *time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[id]
	due, next := state.evaluate(rules, policy, now, hosts)
	if commit {
		c.states[id] = state
	}
	return due, next
}

func (c *reminderClock) reset(id int64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[id]
	state.checkin(now)
	c.states[id] = state
}

func (c *reminderClock) snooze(id int64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[id]
	state.snooze(now)
	c.states[id] = state
}

func (c *reminderClock) discard(id int64) {
	c.mu.Lock()
	delete(c.states, id)
	c.mu.Unlock()
}

func (c *reminderClock) clear() {
	c.mu.Lock()
	c.states = map[int64]activityReminder{}
	c.mu.Unlock()
}
