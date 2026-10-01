package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	RatesURLKey     = "monitoring.rates_url"
	ratesKey        = "monitoring.rates"
	defaultRatesURL = "https://open.er-api.com/v6/latest/USD"
	ratesFreshFor   = 20 * time.Hour
	// The job runs every hour and skips while the stored rates are fresh.
	// The timer starts over on every restart, so a long interval could miss
	// the refresh for days on a server that is deployed often.
	ratesEvery   = time.Hour
	ratesTimeout = 15 * time.Second
	ratesMaxBody = 1 << 20
)

type rateTable struct {
	At    time.Time          `json:"at"`
	Rates map[string]float64 `json:"rates"`
}

func (m *Module) loadRates(ctx context.Context) (*rateTable, error) {
	var stored rateTable
	err := m.d.Settings.Get(ctx, ratesKey, &stored)
	if errors.Is(err, settings.ErrNotSet) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(stored.Rates) == 0 {
		return nil, nil
	}
	return &stored, nil
}

func (m *Module) ratesURL(ctx context.Context) string {
	var u string
	if err := m.d.Settings.Get(ctx, RatesURLKey, &u); err == nil && strings.TrimSpace(u) != "" {
		return strings.TrimSpace(u)
	}
	return defaultRatesURL
}

func (m *Module) refreshRates(ctx context.Context, now time.Time) error {
	stored, err := m.loadRates(ctx)
	if err != nil {
		return err
	}
	if stored != nil && !stored.At.IsZero() && now.Sub(stored.At) < ratesFreshFor {
		return nil
	}
	fresh, err := m.fetchRates(ctx)
	if err != nil {
		return err
	}
	fresh.At = now.UTC()
	return m.d.Settings.Set(ctx, ratesKey, fresh)
}

func (m *Module) fetchRates(ctx context.Context) (rateTable, error) {
	ctx, cancel := context.WithTimeout(ctx, ratesTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.ratesURL(ctx), nil)
	if err != nil {
		return rateTable{}, err
	}
	resp, err := (&http.Client{Transport: m.transport(), Timeout: ratesTimeout}).Do(req)
	if err != nil {
		return rateTable{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, ratesMaxBody+1))
	if err != nil {
		return rateTable{}, err
	}
	if len(raw) > ratesMaxBody {
		return rateTable{}, fmt.Errorf("汇率数据太大")
	}
	if resp.StatusCode != http.StatusOK {
		return rateTable{}, fmt.Errorf("汇率接口返回 %d", resp.StatusCode)
	}
	var body struct {
		Result string             `json:"result"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return rateTable{}, err
	}
	if body.Result != "success" || len(body.Rates) == 0 {
		return rateTable{}, fmt.Errorf("汇率接口没有返回数据")
	}
	return rateTable{Rates: body.Rates}, nil
}

func rateOf(rates map[string]float64, currency string) (float64, bool) {
	if currency == "USD" {
		return 1, true
	}
	r, ok := rates[currency]
	if !ok || r <= 0 {
		return 0, false
	}
	return r, true
}

func applyRates(out api.SubscriptionSummary, rows []db.Subscription, rates *rateTable) api.SubscriptionSummary {
	type acc struct {
		monthly float64
		count   int
	}
	totals := map[string]*acc{"CNY": {}, "USD": {}}
	missing := []string{}
	seen := map[string]bool{}
	for _, s := range rows {
		if s.ArchivedAt != nil {
			continue
		}
		src, ok := rateOf(rates.Rates, s.Currency)
		if !ok {
			if !seen[s.Currency] {
				seen[s.Currency] = true
				missing = append(missing, s.Currency)
			}
			continue
		}
		usd := monthlyCost(s.Amount, int(s.CycleCount), s.CycleUnit) / src
		for _, target := range []string{"CNY", "USD"} {
			dst, ok := rateOf(rates.Rates, target)
			if !ok {
				continue
			}
			totals[target].monthly += usd * dst
			totals[target].count++
		}
	}
	sort.Strings(missing)
	converted := make([]api.ConvertedTotal, 0, 2)
	for _, currency := range []string{"CNY", "USD"} {
		monthly := round2(totals[currency].monthly)
		converted = append(converted, api.ConvertedTotal{
			Currency: api.ConvertedTotalCurrency(currency),
			Monthly:  monthly,
			Yearly:   round2(monthly * 12),
			Count:    totals[currency].count,
		})
	}
	at := rates.At
	out.Converted = &converted
	out.Unconverted = &missing
	out.RatesAt = &at
	return out
}
