package habits

import (
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

func TestReminderPreviewDoesNotAdvanceActivity(t *testing.T) {
	clock := newReminderClock()
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	rules := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	hosts := []contracts.HostPresence{{Online: true}}
	clock.evaluate(1, rules, policy, now, hosts, false)
	if due, next := clock.evaluate(1, rules, policy, now.Add(15*time.Minute), hosts, true); due || next == nil || !next.Equal(now.Add(35*time.Minute)) {
		t.Fatalf("preview advanced activity: %v %v", due, next)
	}
	clock.reset(1, now.Add(17*time.Minute))
	if due, next := clock.evaluate(1, rules, policy, now.Add(18*time.Minute), hosts, false); due || next == nil || !next.Equal(now.Add(37*time.Minute)) {
		t.Fatalf("reset: %v %v", due, next)
	}
	clock.snooze(1, now.Add(20*time.Minute))
	if due, next := clock.evaluate(1, rules, policy, now.Add(21*time.Minute), hosts, false); due || next == nil || !next.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("snooze: %v %v", due, next)
	}
	clock.discard(1)
	if _, next := clock.evaluate(1, rules, policy, now.Add(22*time.Minute), hosts, false); next == nil || !next.Equal(now.Add(42*time.Minute)) {
		t.Fatalf("discard: %v", next)
	}
}

func TestReminderClockConcurrentAccess(t *testing.T) {
	clock := newReminderClock()
	now := scheduleMoment(t, "2026-10-05T12:00:00Z")
	rules := scheduleRules{Timezone: "UTC", IdleMinutes: 5}
	policy := reminderPolicy{When: []string{"active"}, Interval: 20 * time.Minute}
	hosts := []contracts.HostPresence{{Online: true}}
	var group sync.WaitGroup
	for n := range 20 {
		group.Go(func() {
			id := int64(n % 4)
			clock.evaluate(id, rules, policy, now, hosts, true)
			clock.evaluate(id, rules, policy, now, hosts, false)
			clock.reset(id, now)
			clock.snooze(id, now)
			clock.discard(id)
		})
	}
	group.Wait()
	clock.clear()
}
