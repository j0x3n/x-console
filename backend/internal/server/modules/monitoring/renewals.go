package monitoring

import (
	"context"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// Upcoming returns unarchived subscriptions due before until, including
// manual renewals that are overdue. Dates stay as UTC civil dates.
func (m *Module) Upcoming(ctx context.Context, until time.Time) ([]contracts.RenewalRef, error) {
	rows, err := m.q.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	limit := until.Format(dateLayout)
	out := make([]contracts.RenewalRef, 0, len(rows))
	for _, row := range rows {
		if row.ArchivedAt != nil || row.NextRenewal >= limit {
			continue
		}
		date, err := parseDate(row.NextRenewal)
		if err != nil {
			return nil, err
		}
		out = append(out, contracts.RenewalRef{
			Name: row.Name, Date: date, Amount: row.Amount, Currency: row.Currency,
		})
	}
	return out, nil
}
