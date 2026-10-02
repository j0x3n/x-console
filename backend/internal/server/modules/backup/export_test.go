package backup

import (
	"context"
	"database/sql"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

// Hooks for the external test package.

func SetNow(m *Module, now func() time.Time) { m.now = now }

func SetExit(m *Module, exit func()) { m.exit = exit }

func Tick(m *Module, ctx context.Context) error { return m.tick(ctx) }

// Pending tells whether a staged restore waits in dir.
func Pending(dir string) bool { return pending(dir) }

// FinishRestore runs the file phase the module starts after a restart.
func FinishRestore(m *Module, ctx context.Context) error { return m.finishRestore(ctx, &job{}) }

// UnreadableSecrets lists the encrypted settings a key cannot open.
func UnreadableSecrets(ctx context.Context, db *sql.DB, box *secrets.Box) ([]string, error) {
	return unreadableSecrets(ctx, db, box)
}

// MigrateRemotes runs the B69 move of the B63 drive accounts.
func MigrateRemotes(m *Module, ctx context.Context) error { return m.migrateRemotes(ctx) }

func PreparePending(m *Module, ctx context.Context) error { return m.PreparePending(ctx) }
