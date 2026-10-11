package reminders

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// Hooks for the external test package, which cannot import this package's
// internals directly (testutil imports app, which imports this package).

const TelegramSecretKey = telegramSecretKey

func Scan(m *Module, ctx context.Context, now time.Time) error { return m.scan(ctx, now) }

func CreateAt(m *Module, ctx context.Context, in contracts.CreateReminder, now time.Time) (db.Reminder, error) {
	return m.create(ctx, in, now)
}

func CompleteAt(m *Module, ctx context.Context, id int64, now time.Time) (db.Reminder, error) {
	return m.complete(ctx, id, now)
}

func Router(m *Module) notify.Router { return &router{m: m} }

func SetPublicURL(m *Module, u string) { m.d.Config.PublicURL = u }

func PushService(endpoint string) api.WebPushService { return pushService(endpoint) }

func PushIcons(m *Module, ctx context.Context, n notify.Stored) (string, string) {
	return m.pushIcons(ctx, n)
}
func IconDraws(m *Module) int { m.iconMu.Lock(); defer m.iconMu.Unlock(); return m.iconDraws }

// B127 digest hooks.

func DigestTick(m *Module, ctx context.Context, now time.Time) error { return m.digestTick(ctx, now) }

func FlushDigest(m *Module, ctx context.Context, now time.Time, manual bool) (api.DigestFlushResult, error) {
	return m.flushDigest(ctx, now, manual)
}

func ResetDigestBackoff(m *Module) {
	m.digestMu.Lock()
	m.digestRetryAt = time.Time{}
	m.digestMu.Unlock()
}

func BuildDigest(items []db.NotifyDigestQueue, now time.Time, loc *time.Location, link func(string) string) notify.Notification {
	return buildDigest(items, now, loc, link)
}

func DigestCategoryOf(kind string) string { return digestCategoryOf(kind) }

func DigestDecision(enabled bool, modes map[string]string, n notify.Stored) (string, bool) {
	return digestDecision(digestConfig{Enabled: enabled, Modes: modes}, n)
}

func LatestSlot(times []string, now time.Time, loc *time.Location) (time.Time, bool) {
	return latestSlot(times, now, loc)
}

func NextSlot(times []string, now time.Time, loc *time.Location) (time.Time, bool) {
	return nextSlot(times, now, loc)
}
