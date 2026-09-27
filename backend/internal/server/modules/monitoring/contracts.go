package monitoring

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// Upcoming returns active subscriptions due before until in renewal date order.
// Past due subscriptions remain visible until the user marks them renewed.
func (m *Module) Upcoming(ctx context.Context, until time.Time) ([]contracts.RenewalRef, error) {
	rows, err := m.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.RenewalRef, 0)
	for _, s := range rows {
		if s.ArchivedAt != nil {
			continue
		}
		day, err := parseDate(s.NextRenewal)
		if err != nil {
			return nil, err
		}
		date := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, m.loc())
		if date.Before(until) {
			out = append(out, contracts.RenewalRef{
				Name: s.Name, Date: date, Amount: s.Amount, Currency: s.Currency,
			})
		}
	}
	return out, nil
}

var _ contracts.Renewals = (*Module)(nil)
