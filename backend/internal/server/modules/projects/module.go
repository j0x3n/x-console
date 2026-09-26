// Package projects is M5: projects, issues, board order, labels, milestones,
// links and comments. It offers contracts.Issues and contracts.IssueSync to
// other modules.
package projects

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// Module implements api.ServerInterface.
type Module struct {
	d   *module.Deps
	q   *db.Queries
	now func() time.Time
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module and registers its contracts and actions.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), now: func() time.Time { return time.Now().UTC() }}
	module.Provide[contracts.Issues](d.Registry, contracts.IssuesKey, &issuesService{m})
	module.Provide[contracts.IssueSync](d.Registry, contracts.IssueSyncKey, &syncService{m})
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "projects" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Statuses in board order.
var statuses = []string{"backlog", "todo", "in_progress", "in_review", "done", "canceled"}

func validStatus(s string) bool {
	for _, v := range statuses {
		if v == s {
			return true
		}
	}
	return false
}

func closedStatus(s string) bool { return s == "done" || s == "canceled" }

var projectKeyRe = regexp.MustCompile(`^[A-Z]{2,5}$`)

// issueKey builds "XC-12".
func issueKey(projectKey string, number int64) string {
	return projectKey + "-" + strconv.FormatInt(number, 10)
}

// parseIssueKey splits "XC-12" (case-insensitive) into "XC" and 12.
func parseIssueKey(key string) (string, int64, error) {
	i := strings.LastIndexByte(key, '-')
	if i <= 0 {
		return "", 0, httpx.ErrNotFound
	}
	n, err := strconv.ParseInt(key[i+1:], 10, 64)
	if err != nil || n <= 0 {
		return "", 0, httpx.ErrNotFound
	}
	return strings.ToUpper(key[:i]), n, nil
}

// issuePath is the in-app page of an issue, for example /projects/XC/12.
func issuePath(key string) string {
	p, n, err := parseIssueKey(key)
	if err != nil {
		return "/projects"
	}
	return "/projects/" + p + "/" + strconv.FormatInt(n, 10)
}

// notFound turns sql.ErrNoRows into a 404.
func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

// tx runs fn in a transaction. Inside fn only use the given queries: tests
// run on a single connection, so touching m.q there would deadlock.
func (m *Module) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(m.q.WithTx(tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// today is the user's local date as YYYY-MM-DD.
func (m *Module) today() time.Time {
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.Local
	}
	now := m.now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
}

const dateLayout = "2006-01-02"

// validDate checks a YYYY-MM-DD string.
func validDate(s string) error {
	if _, err := time.Parse(dateLayout, s); err != nil {
		return httpx.Invalid("日期格式应为 YYYY-MM-DD")
	}
	return nil
}
