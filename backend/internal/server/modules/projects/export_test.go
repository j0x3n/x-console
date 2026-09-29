package projects

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// RunDueForTest invokes the scheduler job at a controlled time.
func RunDueForTest(d *module.Deps, now time.Time) error {
	m := &Module{d: d, q: db.New(d.DB), now: func() time.Time { return now }}
	return m.sendDue(context.Background())
}
