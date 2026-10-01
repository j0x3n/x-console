// Package mail is B53: read Gmail, Aliyun enterprise mail and other IMAP
// inboxes, and push new mail. The first version only receives.
//
// Every account has one background goroutine (worker.go) that keeps an IMAP
// connection open, syncs the inbox into mail_messages and waits for new mail
// with IDLE. Opening a message or changing its flags uses a short extra
// connection, because the worker's connection is busy idling.
package mail

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail/db"
)

// ServiceKey finds the module in module.Registry, for tests.
const ServiceKey = "mail.service"

type Module struct {
	d     *module.Deps
	q     *db.Queries
	files files.Store
	now   func() time.Time

	// tlsConfig is used for every IMAP connection. Tests trust their own
	// certificate here.
	tlsConfig *tls.Config
	// retry is the wait between reconnects, longer each time.
	retry []time.Duration
	// poll is how often a server without IDLE is asked for new mail.
	poll time.Duration
	// idleRestart is how long one IDLE lasts before it is renewed.
	idleRestart time.Duration

	mu      sync.Mutex
	ctx     context.Context // set by Start; workers stop when it ends
	workers map[int64]*worker
	states  map[int64]accountState
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), files: d.Files.For("mail"), now: time.Now,
		retry:       []time.Duration{10 * time.Second, 30 * time.Second, time.Minute, 5 * time.Minute},
		poll:        time.Minute,
		idleRestart: 25 * time.Minute,
		workers:     map[int64]*worker{},
		states:      map[int64]accountState{},
	}
	module.Provide[*Module](d.Registry, ServiceKey, m)
	return m, nil
}

func (m *Module) Name() string { return "mail" }

func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start starts one worker per account. They stop when ctx ends.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	accounts, err := m.q.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, a := range accounts {
		m.startWorker(a.ID)
	}
	return nil
}
