// Package audit records every write action and every remote execution.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
)

type actorKey struct{}

// WithActor stores the acting user (or "system", "automation:<id>") in ctx.
func WithActor(ctx context.Context, actor string) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// Actor returns the actor stored in ctx, or "system".
func Actor(ctx context.Context) string {
	if a, ok := ctx.Value(actorKey{}).(string); ok && a != "" {
		return a
	}
	return "system"
}

// Log writes audit entries.
type Log struct{ q *db.Queries }

// New builds a Log.
func New(conn *sql.DB) *Log { return &Log{q: db.New(conn)} }

// Record writes one entry. err nil means result "ok". Failures to write the
// audit log are logged but never fail the caller.
func (l *Log) Record(ctx context.Context, action, target string, detail map[string]any, err error) {
	if detail == nil {
		detail = map[string]any{}
	}
	raw, _ := json.Marshal(detail)
	result := "ok"
	if err != nil {
		result = "error: " + err.Error()
	}
	if werr := l.q.InsertAudit(context.WithoutCancel(ctx), db.InsertAuditParams{
		At: time.Now().UTC(), Actor: Actor(ctx), Action: action, Target: target, Detail: string(raw), Result: result,
	}); werr != nil {
		slog.Error("audit write failed", "action", action, "err", werr)
	}
}
