package monitoring

import (
	"context"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

type reminderSource struct{ m *Module }

func (s reminderSource) Upcoming(ctx context.Context, from, until time.Time) ([]contracts.ExternalReminder, error) {
	subscriptions, err := s.m.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	loc := s.m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	out := []contracts.ExternalReminder{}
	for _, item := range subscriptions {
		if item.ArchivedAt != nil {
			continue
		}
		date, err := parseDate(item.NextRenewal)
		if err != nil {
			return nil, err
		}
		at := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
		if !at.Before(until) {
			continue
		}
		out = append(out, contracts.ExternalReminder{ID: "subscription:" + strconv.FormatInt(item.ID, 10), Source: "subscription", SourceLabel: "订阅", Title: item.Name + " 续费", At: at, Link: "/monitoring/subscriptions"})
	}
	monitors, err := s.m.q.ListMonitors(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range monitors {
		if item.Enabled != 1 || item.ExpiresAt == nil || !item.ExpiresAt.Before(until) {
			continue
		}
		source, label, title := "", "", ""
		switch item.Kind {
		case kindTLS:
			source, label, title = "certificate", "证书", item.Target+" 证书到期"
		case kindDomain:
			source, label, title = "domain", "域名", item.Target+" 域名到期"
		default:
			continue
		}
		out = append(out, contracts.ExternalReminder{ID: source + ":" + strconv.FormatInt(item.ID, 10), Source: source, SourceLabel: label, Title: title, At: *item.ExpiresAt, Link: "/monitoring/certs"})
	}
	return out, nil
}
