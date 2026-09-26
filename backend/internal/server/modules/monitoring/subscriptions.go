package monitoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// Subscription cycles and categories.
const (
	cycleMonthly = "monthly"
	cycleYearly  = "yearly"
	cycleCustom  = "custom_days"
)

var categories = []string{"server", "domain", "saas", "other"}

// Subscription history kinds.
const (
	eventCreated  = "created"
	eventReminded = "reminded"
	eventRenewed  = "renewed"
	eventUpdated  = "updated"
)

func intList(raw string) []int {
	out := []int{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func (m *Module) loc() *time.Location {
	if m.d.Config.Location != nil {
		return m.d.Config.Location
	}
	return time.UTC
}

func toAPISubscription(s db.Subscription, day time.Time) api.Subscription {
	out := api.Subscription{Id: s.ID, Name: s.Name, Category: api.SubscriptionCategory(s.Category), Amount: s.Amount,
		Currency: s.Currency, Cycle: api.SubscriptionCycle(s.Cycle), CycleDays: int(s.CycleDays),
		RemindDaysBefore: intList(s.RemindDaysBefore), Url: s.Url, Note: s.Note, AutoRenew: s.AutoRenew == 1,
		ArchivedAt: s.ArchivedAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		MonthlyCost: round2(monthlyCost(s.Amount, s.Cycle, int(s.CycleDays)))}
	if next, err := parseDate(s.NextRenewal); err == nil {
		out.NextRenewal = openapi_types.Date{Time: next}
		out.DaysLeft = daysBetween(day, next)
	}
	return out
}

// subFields is a validated subscription.
type subFields struct {
	name, category, currency, cycle, url, note string
	amount                                     float64
	cycleDays                                  int
	next                                       time.Time
	remind                                     []int
	autoRenew                                  bool
}

func (f *subFields) validate() error {
	f.name = strings.TrimSpace(f.name)
	f.currency = strings.ToUpper(strings.TrimSpace(f.currency))
	switch {
	case f.name == "":
		return httpx.Invalid("请填写名称")
	case f.amount < 0:
		return httpx.Invalid("金额不能是负数")
	case f.currency == "" || len(f.currency) > 8:
		return httpx.Invalid("请填写币种，例如 CNY")
	case !slices.Contains(categories, f.category):
		return httpx.Invalid("分类只能是 server、domain、saas 或 other")
	case f.next.IsZero():
		return httpx.Invalid("请填写下次续费日期")
	}
	switch f.cycle {
	case cycleMonthly, cycleYearly:
		f.cycleDays = 0
	case cycleCustom:
		if f.cycleDays < 1 || f.cycleDays > 3660 {
			return httpx.Invalid("自定义周期要在 1 到 3660 天之间")
		}
	default:
		return httpx.Invalid("周期只能是 monthly、yearly 或 custom_days")
	}
	clean := []int{}
	for _, d := range f.remind {
		if d < 0 || d > 365 {
			return httpx.Invalid("提前提醒的天数要在 0 到 365 之间")
		}
		if !slices.Contains(clean, d) {
			clean = append(clean, d)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(clean)))
	f.remind = clean
	return nil
}

func (m *Module) getSubscription(ctx context.Context, id int64) (db.Subscription, error) {
	s, err := m.q.GetSubscription(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.Subscription{}, httpx.ErrNotFound
	}
	return s, err
}

func (m *Module) subEvent(ctx context.Context, id int64, at time.Time, kind, detail string) {
	if _, err := m.q.InsertSubscriptionEvent(ctx, db.InsertSubscriptionEventParams{SubscriptionID: id, At: at, Kind: kind, Detail: detail}); err != nil {
		m.d.Log.Warn("subscription history", "subscription", id, "err", err)
	}
}

// ListSubscriptions is GET /subscriptions.
func (m *Module) ListSubscriptions(w http.ResponseWriter, r *http.Request, params api.ListSubscriptionsParams) {
	rows, err := m.q.ListSubscriptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	archived := params.Archived != nil && *params.Archived
	day := today(m.now(), m.loc())
	out := make([]api.Subscription, 0, len(rows))
	for _, s := range rows {
		if (s.ArchivedAt != nil) != archived {
			continue
		}
		out = append(out, toAPISubscription(s, day))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateSubscription is POST /subscriptions.
func (m *Module) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var body api.SubscriptionInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f := subFields{name: body.Name, category: "other", currency: "CNY", cycle: string(body.Cycle), amount: body.Amount,
		next: body.NextRenewal.Time, remind: []int{7, 1}}
	if body.Category != nil {
		f.category = string(*body.Category)
	}
	if body.Currency != nil {
		f.currency = *body.Currency
	}
	if body.CycleDays != nil {
		f.cycleDays = *body.CycleDays
	}
	if body.RemindDaysBefore != nil {
		f.remind = *body.RemindDaysBefore
	}
	if body.Url != nil {
		f.url = *body.Url
	}
	if body.Note != nil {
		f.note = *body.Note
	}
	if body.AutoRenew != nil {
		f.autoRenew = *body.AutoRenew
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	s, err := m.q.CreateSubscription(r.Context(), db.CreateSubscriptionParams{Name: f.name, Category: f.category, Amount: f.amount,
		Currency: f.currency, Cycle: f.cycle, CycleDays: int64(f.cycleDays), NextRenewal: f.next.Format(dateLayout),
		RemindDaysBefore: mustJSON(f.remind), Url: f.url, Note: f.note, AutoRenew: boolInt(f.autoRenew), CreatedAt: now, UpdatedAt: now})
	m.d.Audit.Record(r.Context(), "subscription.create", "", map[string]any{"name": f.name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.subEvent(r.Context(), s.ID, now, eventCreated, "下次续费 "+s.NextRenewal)
	out := toAPISubscription(s, today(now, m.loc()))
	m.d.Bus.Publish("subscription.created", out)
	httpx.JSON(w, http.StatusCreated, out)
}

// GetSubscription is GET /subscriptions/{subscriptionId}.
func (m *Module) GetSubscription(w http.ResponseWriter, r *http.Request, id int64) {
	s, err := m.getSubscription(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPISubscription(s, today(m.now(), m.loc())))
}

// UpdateSubscription is PATCH /subscriptions/{subscriptionId}.
func (m *Module) UpdateSubscription(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.SubscriptionPatch
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	cur, err := m.getSubscription(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next, _ := parseDate(cur.NextRenewal)
	f := subFields{name: cur.Name, category: cur.Category, currency: cur.Currency, cycle: cur.Cycle, url: cur.Url, note: cur.Note,
		amount: cur.Amount, cycleDays: int(cur.CycleDays), next: next, remind: intList(cur.RemindDaysBefore), autoRenew: cur.AutoRenew == 1}
	if body.Name != nil {
		f.name = *body.Name
	}
	if body.Category != nil {
		f.category = string(*body.Category)
	}
	if body.Amount != nil {
		f.amount = *body.Amount
	}
	if body.Currency != nil {
		f.currency = *body.Currency
	}
	if body.Cycle != nil {
		f.cycle = string(*body.Cycle)
	}
	if body.CycleDays != nil {
		f.cycleDays = *body.CycleDays
	}
	if body.NextRenewal != nil {
		f.next = body.NextRenewal.Time
	}
	if body.RemindDaysBefore != nil {
		f.remind = *body.RemindDaysBefore
	}
	if body.Url != nil {
		f.url = *body.Url
	}
	if body.Note != nil {
		f.note = *body.Note
	}
	if body.AutoRenew != nil {
		f.autoRenew = *body.AutoRenew
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	archived := cur.ArchivedAt
	if body.Archived != nil {
		switch {
		case *body.Archived && archived == nil:
			archived = &now
		case !*body.Archived:
			archived = nil
		}
	}
	nextStr := f.next.Format(dateLayout)
	reminded := cur.Reminded
	if nextStr != cur.NextRenewal {
		reminded = "[]" // a new renewal date gets its own reminders
	}
	s, err := m.q.UpdateSubscription(ctx, db.UpdateSubscriptionParams{ID: id, Name: f.name, Category: f.category, Amount: f.amount,
		Currency: f.currency, Cycle: f.cycle, CycleDays: int64(f.cycleDays), NextRenewal: nextStr, RemindDaysBefore: mustJSON(f.remind),
		Reminded: reminded, Url: f.url, Note: f.note, AutoRenew: boolInt(f.autoRenew), ArchivedAt: archived, UpdatedAt: now})
	m.d.Audit.Record(ctx, "subscription.update", itoa(id), map[string]any{"name": f.name, "nextRenewal": nextStr}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if nextStr != cur.NextRenewal {
		m.subEvent(ctx, id, now, eventUpdated, "续费日期 "+cur.NextRenewal+" → "+nextStr)
	}
	out := toAPISubscription(s, today(now, m.loc()))
	m.d.Bus.Publish("subscription.updated", out)
	httpx.JSON(w, http.StatusOK, out)
}

// DeleteSubscription is DELETE /subscriptions/{subscriptionId}.
func (m *Module) DeleteSubscription(w http.ResponseWriter, r *http.Request, id int64) {
	err := notFound(m.q.DeleteSubscription(r.Context(), id))
	m.d.Audit.Record(r.Context(), "subscription.delete", itoa(id), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("subscription.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// ListSubscriptionEvents is GET /subscriptions/{subscriptionId}/events.
func (m *Module) ListSubscriptionEvents(w http.ResponseWriter, r *http.Request, id int64) {
	if _, err := m.getSubscription(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rows, err := m.q.ListSubscriptionEvents(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.SubscriptionEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, api.SubscriptionEvent{Id: e.ID, SubscriptionId: e.SubscriptionID, At: e.At,
			Kind: api.SubscriptionEventKind(e.Kind), Detail: e.Detail})
	}
	httpx.JSON(w, http.StatusOK, out)
}

// summary adds up active subscriptions per currency and category.
func summary(rows []db.Subscription) api.SubscriptionSummary {
	type key struct{ cat, cur string }
	totals := map[string]*api.SpendTotal{}
	cats := map[key]*api.CategorySpend{}
	for _, s := range rows {
		if s.ArchivedAt != nil {
			continue
		}
		mo := monthlyCost(s.Amount, s.Cycle, int(s.CycleDays))
		t, ok := totals[s.Currency]
		if !ok {
			t = &api.SpendTotal{Currency: s.Currency}
			totals[s.Currency] = t
		}
		t.Monthly += mo
		t.Count++
		k := key{s.Category, s.Currency}
		c, ok := cats[k]
		if !ok {
			c = &api.CategorySpend{Category: api.SubscriptionCategory(s.Category), Currency: s.Currency}
			cats[k] = c
		}
		c.Monthly += mo
	}
	out := api.SubscriptionSummary{Totals: []api.SpendTotal{}, ByCategory: []api.CategorySpend{}}
	for _, t := range totals {
		t.Yearly = round2(t.Monthly * 12)
		t.Monthly = round2(t.Monthly)
		out.Totals = append(out.Totals, *t)
	}
	for _, c := range cats {
		c.Yearly = round2(c.Monthly * 12)
		c.Monthly = round2(c.Monthly)
		out.ByCategory = append(out.ByCategory, *c)
	}
	sort.Slice(out.Totals, func(i, j int) bool { return out.Totals[i].Currency < out.Totals[j].Currency })
	sort.Slice(out.ByCategory, func(i, j int) bool {
		a, b := out.ByCategory[i], out.ByCategory[j]
		if a.Currency != b.Currency {
			return a.Currency < b.Currency
		}
		return a.Monthly > b.Monthly
	})
	return out
}

// GetSubscriptionSummary is GET /subscriptions/summary.
func (m *Module) GetSubscriptionSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListSubscriptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, summary(rows))
}

// scanSubscriptions moves auto-renewing subscriptions past their renewal
// date to the next cycle and sends due renewal reminders. It is idempotent:
// running it many times a day sends each reminder once.
func (m *Module) scanSubscriptions(ctx context.Context, now time.Time) error {
	rows, err := m.q.ListSubscriptions(ctx)
	if err != nil {
		return err
	}
	loc := m.loc()
	day := today(now, loc)
	remindNow := now.In(loc).Hour() >= reminderHour
	for _, s := range rows {
		if s.ArchivedAt != nil {
			continue
		}
		next, err := parseDate(s.NextRenewal)
		if err != nil {
			continue
		}
		reminded := intList(s.Reminded)
		changed := false
		var renewedFrom string
		if s.AutoRenew == 1 && next.Before(day) {
			renewedFrom = s.NextRenewal
			next = renewUntil(next, day, s.Cycle, int(s.CycleDays))
			reminded = []int{}
			changed = true
		}
		left := daysBetween(day, next)
		var remind bool
		if remindNow {
			var marked []int
			if remind, marked = reminderDue(left, intList(s.RemindDaysBefore), reminded); remind {
				reminded = marked
				changed = true
			}
		}
		if !changed {
			continue
		}
		s, err = m.q.UpdateSubscription(ctx, db.UpdateSubscriptionParams{ID: s.ID, Name: s.Name, Category: s.Category, Amount: s.Amount,
			Currency: s.Currency, Cycle: s.Cycle, CycleDays: s.CycleDays, NextRenewal: next.Format(dateLayout),
			RemindDaysBefore: s.RemindDaysBefore, Reminded: mustJSON(reminded), Url: s.Url, Note: s.Note, AutoRenew: s.AutoRenew,
			ArchivedAt: s.ArchivedAt, UpdatedAt: now})
		if err != nil {
			return err
		}
		out := toAPISubscription(s, day)
		if renewedFrom != "" {
			m.subEvent(ctx, s.ID, now, eventRenewed, "自动续期 "+renewedFrom+" → "+s.NextRenewal)
			m.d.Bus.Publish("subscription.renewed", out)
		}
		if remind {
			m.subEvent(ctx, s.ID, now, eventReminded, fmt.Sprintf("提前 %d 天提醒", left))
			m.d.Bus.Publish("subscription.due", out)
			m.sendRenewalReminder(ctx, s, left)
		}
	}
	return nil
}

func (m *Module) sendRenewalReminder(ctx context.Context, s db.Subscription, left int) {
	title := fmt.Sprintf("%s 还有 %d 天续费", s.Name, left)
	if left == 0 {
		title = s.Name + " 今天续费"
	}
	body := fmt.Sprintf("%s，金额 %.2f %s。", s.NextRenewal, s.Amount, s.Currency)
	if s.AutoRenew == 1 {
		body += "会自动续费。"
	} else {
		body += "记得手动续费。"
	}
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: "subscription.due", Title: title, Body: body,
		Link: "/monitoring/subscriptions?subscription=" + itoa(s.ID), Source: "monitoring",
		Data: map[string]any{"subscriptionId": s.ID}})
	if err != nil {
		m.d.Log.Warn("renewal reminder failed", "subscription", s.ID, "err", err)
	}
}
