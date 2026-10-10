package contacts

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Hooks for the integration tests in package contacts_test, which cannot
// live in this package because testutil imports app, which imports contacts.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func RemindAll(m *Module, ctx context.Context) error { return m.remindAll(ctx) }

// SetHTTPClient makes the address book requests use c, which can trust the
// certificate of a test server.
func SetHTTPClient(m *Module, c *http.Client) { m.httpClient = c }

// ParseVCards exposes the card reader for the unit tests.
func ParseVCards(raw string) (names []string, dates [][]string, bad int) {
	people, bad := parseVCards(strings.NewReader(raw))
	for _, p := range people {
		names = append(names, p.name)
		var d []string
		for _, e := range p.events {
			d = append(d, e.ID+"="+string(e.Kind)+":"+e.Label+":"+e.Date)
		}
		dates = append(dates, d)
	}
	return names, dates, bad
}

// SyncJob runs the scheduled sync once.
func SyncJob(m *Module, ctx context.Context) error { return m.syncJob(ctx) }
