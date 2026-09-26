package reminders

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
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
