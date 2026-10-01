// Package notes is M6: Markdown notes with tags, pinning, full-text search
// (FTS5 trigram, LIKE for short queries), and turning a note into an issue
// or a reminder. It offers contracts.Notes to other modules.
package notes

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
)

// Module implements api.ServerInterface.
type Module struct {
	d             *module.Deps
	q             *db.Queries
	now           func() time.Time
	files         files.Store // the notes' own part of the site's file store
	aiMu          sync.Mutex
	aiTimers      map[int64]*aiTimer
	aiDelay       func(time.Duration, func()) func()
	aiCtx         context.Context
	shareMu       sync.Mutex
	shareRates    map[string]noteShareRate
	shareFailures map[string]noteShareFailure
	shareVisits   map[[32]byte]time.Time
	shareNow      func() time.Time
}

var _ api.ServerInterface = (*Module)(nil)

// New builds the module and registers its contract and actions.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, q: db.New(d.DB), now: func() time.Time { return time.Now().UTC() }, files: d.Files.For("notes"), aiTimers: map[int64]*aiTimer{}, aiCtx: context.Background()}
	m.shareRates = map[string]noteShareRate{}
	m.shareFailures = map[string]noteShareFailure{}
	m.shareVisits = map[[32]byte]time.Time{}
	m.aiDelay = func(delay time.Duration, fn func()) func() {
		timer := time.AfterFunc(delay, fn)
		return func() { timer.Stop() }
	}
	module.Provide[contracts.Notes](d.Registry, contracts.NotesKey, &notesService{m})
	module.Provide[*Module](d.Registry, "notes.module", m)
	m.registerActions()
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "notes" }

func (m *Module) Start(ctx context.Context) error {
	m.aiMu.Lock()
	m.aiCtx = ctx
	m.aiMu.Unlock()
	go func() {
		<-ctx.Done()
		m.aiMu.Lock()
		for id, timer := range m.aiTimers {
			timer.cancel()
			delete(m.aiTimers, id)
		}
		m.aiMu.Unlock()
	}()
	return nil
}

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// notesService implements contracts.Notes.
type notesService struct{ m *Module }

func (s *notesService) Create(ctx context.Context, title, body string, tags []string) (int64, error) {
	n, err := s.m.createNote(auth.WithoutVault(ctx), title, body, tags, false, false, false)
	return n.Id, err
}

func notFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}

// tx runs fn in a write transaction (BEGIN IMMEDIATE on one connection).
// A deferred transaction that reads first and writes later fails at once
// with SQLITE_BUSY when another writer got in between; taking the write lock
// up front makes it wait for busy_timeout instead. Inside fn only use the
// given queries: tests run on a single connection.
func (m *Module) tx(ctx context.Context, fn func(q *db.Queries) error) error {
	c, err := m.d.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	rollback := func() { _, _ = c.ExecContext(context.WithoutCancel(ctx), "ROLLBACK") }
	if err := fn(db.New(c)); err != nil {
		rollback()
		return err
	}
	if _, err := c.ExecContext(ctx, "COMMIT"); err != nil {
		rollback()
		return err
	}
	return nil
}

const maxTagLen = 40

// cleanTags trims tags, drops a leading #, removes blanks and duplicates.
func cleanTags(tags []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, t := range tags {
		t = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(t), "#"))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > maxTagLen {
			return nil, httpx.Invalid("标签太长，最多 40 个字")
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

var (
	mdLink    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdLine    = regexp.MustCompile(`(?m)^\s{0,3}(?:#{1,6}\s+|>\s?|[-*+]\s+(?:\[[ xX]\]\s+)?|\d+[.)]\s+|` + "```" + `.*$)`)
	mdInline  = regexp.MustCompile("[*_`~]+")
	mdSpacing = regexp.MustCompile(`\s+`)
)

// plainText strips common Markdown so lists can show a readable excerpt.
func plainText(md string) string {
	s := mdLink.ReplaceAllString(md, "$1")
	s = mdLine.ReplaceAllString(s, "")
	s = mdInline.ReplaceAllString(s, "")
	return strings.TrimSpace(mdSpacing.ReplaceAllString(s, " "))
}

// truncate cuts s to n runes and adds an ellipsis.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// displayTitle is the title, or the first line of the body when empty.
func displayTitle(title, body string) string {
	if t := strings.TrimSpace(title); t != "" {
		return t
	}
	for _, line := range strings.Split(body, "\n") {
		if t := plainText(line); t != "" {
			return truncate(t, 60)
		}
	}
	return "无标题笔记"
}

func notePath(id int64) string { return "/notes/" + strconv.FormatInt(id, 10) }

// issuePath turns "XC-12" into the in-app page /projects/XC/12.
func issuePath(key string) string {
	i := strings.LastIndexByte(key, '-')
	if i <= 0 {
		return "/projects"
	}
	return "/projects/" + key[:i] + "/" + key[i+1:]
}
