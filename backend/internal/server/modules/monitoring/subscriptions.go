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
	"unicode/utf8"

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

// categoryOther is the built-in category that deleted categories fall back to.
const categoryOther = "other"

// maxCycleCount is the largest "every N units" a subscription accepts.
const maxCycleCount = 1000

// catInfo is what a subscription needs to show its category.
type catInfo struct {
	name    string
	builtin string
}

func (m *Module) categoryNames(ctx context.Context) (map[int64]catInfo, error) {
	rows, err := m.q.ListSubscriptionCategories(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]catInfo, len(rows))
	for _, c := range rows {
		info := catInfo{name: c.Name}
		if c.Builtin != nil {
			info.builtin = *c.Builtin
		}
		out[c.ID] = info
	}
	return out, nil
}

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

func toAPISubscription(s db.Subscription, day time.Time, cats map[int64]catInfo) api.Subscription {
	legacy, legacyDays := legacyCycle(int(s.CycleCount), s.CycleUnit)
	count, unit := int(s.CycleCount), api.SubscriptionCycleUnit(s.CycleUnit)
	account := s.Account
	out := api.Subscription{Id: s.ID, Name: s.Name, Account: &account, Category: api.SubscriptionCategory(s.Category), Amount: s.Amount,
		Currency: s.Currency, Cycle: api.SubscriptionCycle(legacy), CycleDays: legacyDays, CycleCount: &count, CycleUnit: &unit,
		RemindDaysBefore: intList(s.RemindDaysBefore), Url: s.Url, Note: s.Note, AutoRenew: s.AutoRenew == 1,
		ArchivedAt: s.ArchivedAt, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		MonthlyCost: round2(monthlyCost(s.Amount, count, s.CycleUnit))}
	if s.CategoryID != nil {
		out.CategoryId = s.CategoryID
		if c, ok := cats[*s.CategoryID]; ok {
			out.CategoryName = &c.name
		}
	}
	if next, err := parseDate(s.NextRenewal); err == nil {
		out.NextRenewal = openapi_types.Date{Time: next}
		out.DaysLeft = daysBetween(day, next)
	}
	return out
}

// subFields is a validated subscription.
type subFields struct {
	name, currency, url, note, account string
	categoryID                         int64
	builtin                            string // built-in key of the category, or "other" for custom ones
	amount                             float64
	cycleCount                         int
	cycleUnit                          string
	next                               time.Time
	remind                             []int
	autoRenew                          bool
}

func (f *subFields) validate() error {
	f.name = strings.TrimSpace(f.name)
	f.account = strings.TrimSpace(f.account)
	f.currency = strings.ToUpper(strings.TrimSpace(f.currency))
	if utf8.RuneCountInString(f.account) > 200 {
		return httpx.Invalid("账号最多 200 个字")
	}
	switch {
	case f.name == "":
		return httpx.Invalid("请填写名称")
	case f.amount < 0:
		return httpx.Invalid("金额不能是负数")
	case f.currency == "" || len(f.currency) > 8:
		return httpx.Invalid("请填写币种，例如 CNY")
	case f.next.IsZero():
		return httpx.Invalid("请填写下次续费日期")
	case !slices.Contains(cycleUnits, f.cycleUnit):
		return httpx.Invalid("周期单位只能是分钟、小时、天、周、月或年")
	case f.cycleCount < 1 || f.cycleCount > maxCycleCount:
		return httpx.Invalid("周期数字要在 1 到 1000 之间")
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

// resolveCategory finds the category a request means. categoryId wins over
// the old category field. With neither, current (or "other" when zero) stays.
func (m *Module) resolveCategory(ctx context.Context, id *int64, builtin *api.SubscriptionCategory) (db.SubscriptionCategory, bool, error) {
	var c db.SubscriptionCategory
	var err error
	switch {
	case id != nil:
		c, err = m.q.GetSubscriptionCategory(ctx, *id)
	case builtin != nil:
		name := string(*builtin)
		c, err = m.q.GetSubscriptionCategoryByBuiltin(ctx, &name)
	default:
		return c, false, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return c, false, httpx.Invalid("分类不存在")
	}
	return c, true, err
}

func (f *subFields) setCategory(c db.SubscriptionCategory) {
	f.categoryID = c.ID
	f.builtin = categoryOther
	if c.Builtin != nil {
		f.builtin = *c.Builtin
	}
}

// legacyOrNew picks the cycle of a request. cycleCount and cycleUnit win;
// otherwise the old cycle and cycleDays are converted. changed is false when
// the request carries no cycle at all.
func legacyOrNew(count *int, unit *api.SubscriptionCycleUnit, cycle *api.SubscriptionCycle, days *int, curCount int, curUnit string) (int, string, bool, error) {
	switch {
	case count != nil || unit != nil:
		if count == nil || unit == nil {
			return 0, "", false, httpx.Invalid("周期的数字和单位要一起填")
		}
		return *count, string(*unit), true, nil
	case cycle != nil || days != nil:
		_, curDays := legacyCycle(curCount, curUnit)
		c := cycleMonthly
		if cycle != nil {
			c = string(*cycle)
		} else if lc, _ := legacyCycle(curCount, curUnit); lc != "" {
			c = lc
		}
		d := curDays
		if days != nil {
			d = *days
		}
		switch c {
		case cycleMonthly, cycleYearly:
		case cycleCustom:
			if d < 1 || d > 3660 {
				return 0, "", false, httpx.Invalid("自定义周期要在 1 到 3660 天之间")
			}
		default:
			return 0, "", false, httpx.Invalid("周期只能是 monthly、yearly 或 custom_days")
		}
		n, u := cycleFromLegacy(c, d)
		return n, u, true, nil
	}
	return curCount, curUnit, false, nil
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
	cats, err := m.categoryNames(r.Context())
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
		out = append(out, toAPISubscription(s, day, cats))
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
	ctx := r.Context()
	f := subFields{name: body.Name, currency: "CNY", amount: body.Amount, next: body.NextRenewal.Time, remind: []int{7, 3, 1}}
	other := api.SubscriptionCategory(categoryOther)
	if body.CategoryId == nil && body.Category == nil {
		body.Category = &other
	}
	cat, _, err := m.resolveCategory(ctx, body.CategoryId, body.Category)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	f.setCategory(cat)
	cycle := body.Cycle
	f.cycleCount, f.cycleUnit, _, err = legacyOrNew(body.CycleCount, body.CycleUnit, &cycle, body.CycleDays, 1, unitMonth)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Currency != nil {
		f.currency = *body.Currency
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
	if body.Account != nil {
		f.account = *body.Account
	}
	if body.AutoRenew != nil {
		f.autoRenew = *body.AutoRenew
	}
	if err := f.validate(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := m.now()
	legacy, legacyDays := legacyCycle(f.cycleCount, f.cycleUnit)
	s, err := m.q.CreateSubscription(ctx, db.CreateSubscriptionParams{Name: f.name, Category: f.builtin, CategoryID: &f.categoryID,
		Amount: f.amount, Currency: f.currency, Cycle: legacy, CycleDays: int64(legacyDays), CycleCount: int64(f.cycleCount),
		CycleUnit: f.cycleUnit, NextRenewal: f.next.Format(dateLayout), RemindDaysBefore: mustJSON(f.remind), Url: f.url,
		Note: f.note, Account: f.account, AutoRenew: boolInt(f.autoRenew), CreatedAt: now, UpdatedAt: now})
	m.d.Audit.Record(ctx, "subscription.create", "", map[string]any{"name": f.name}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.subEvent(ctx, s.ID, now, eventCreated, "下次续费 "+s.NextRenewal)
	cats, err := m.categoryNames(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPISubscription(s, today(now, m.loc()), cats)
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
	cats, err := m.categoryNames(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPISubscription(s, today(m.now(), m.loc()), cats))
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
	f := subFields{name: cur.Name, builtin: cur.Category, currency: cur.Currency, url: cur.Url, note: cur.Note, account: cur.Account,
		amount: cur.Amount, cycleCount: int(cur.CycleCount), cycleUnit: cur.CycleUnit, next: next,
		remind: intList(cur.RemindDaysBefore), autoRenew: cur.AutoRenew == 1}
	if cur.CategoryID != nil {
		f.categoryID = *cur.CategoryID
	}
	if body.Name != nil {
		f.name = *body.Name
	}
	cat, found, err := m.resolveCategory(ctx, body.CategoryId, body.Category)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if found {
		f.setCategory(cat)
	}
	if body.Amount != nil {
		f.amount = *body.Amount
	}
	if body.Currency != nil {
		f.currency = *body.Currency
	}
	f.cycleCount, f.cycleUnit, _, err = legacyOrNew(body.CycleCount, body.CycleUnit, body.Cycle, body.CycleDays, f.cycleCount, f.cycleUnit)
	if err != nil {
		httpx.Fail(w, r, err)
		return
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
	if body.Account != nil {
		f.account = *body.Account
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
	legacy, legacyDays := legacyCycle(f.cycleCount, f.cycleUnit)
	s, err := m.q.UpdateSubscription(ctx, db.UpdateSubscriptionParams{ID: id, Name: f.name, Category: f.builtin, CategoryID: &f.categoryID,
		Amount: f.amount, Currency: f.currency, Cycle: legacy, CycleDays: int64(legacyDays), CycleCount: int64(f.cycleCount),
		CycleUnit: f.cycleUnit, NextRenewal: nextStr, RemindDaysBefore: mustJSON(f.remind), Reminded: reminded, Url: f.url,
		Note: f.note, Account: f.account, AutoRenew: boolInt(f.autoRenew), ArchivedAt: archived, UpdatedAt: now})
	m.d.Audit.Record(ctx, "subscription.update", itoa(id), map[string]any{"name": f.name, "nextRenewal": nextStr}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if nextStr != cur.NextRenewal {
		m.subEvent(ctx, id, now, eventUpdated, "续费日期 "+cur.NextRenewal+" → "+nextStr)
	}
	cats, err := m.categoryNames(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := toAPISubscription(s, today(now, m.loc()), cats)
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
func summary(rows []db.Subscription, cats map[int64]catInfo, rates *rateTable) api.SubscriptionSummary {
	type key struct {
		cat int64
		cur string
	}
	totals := map[string]*api.SpendTotal{}
	by := map[key]*api.CategorySpend{}
	for _, s := range rows {
		if s.ArchivedAt != nil {
			continue
		}
		mo := monthlyCost(s.Amount, int(s.CycleCount), s.CycleUnit)
		t, ok := totals[s.Currency]
		if !ok {
			t = &api.SpendTotal{Currency: s.Currency}
			totals[s.Currency] = t
		}
		t.Monthly += mo
		t.Count++
		var id int64
		if s.CategoryID != nil {
			id = *s.CategoryID
		}
		k := key{id, s.Currency}
		c, ok := by[k]
		if !ok {
			c = &api.CategorySpend{Category: api.SubscriptionCategory(s.Category), Currency: s.Currency}
			if s.CategoryID != nil {
				c.CategoryId = s.CategoryID
				if info, ok := cats[id]; ok {
					c.CategoryName = &info.name
				}
			}
			by[k] = c
		}
		c.Monthly += mo
	}
	out := api.SubscriptionSummary{Totals: []api.SpendTotal{}, ByCategory: []api.CategorySpend{}}
	for _, t := range totals {
		t.Yearly = round2(t.Monthly * 12)
		t.Monthly = round2(t.Monthly)
		out.Totals = append(out.Totals, *t)
	}
	for _, c := range by {
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
	if rates != nil {
		out = applyRates(out, rows, rates)
	}
	return out
}

// GetSubscriptionSummary is GET /subscriptions/summary.
func (m *Module) GetSubscriptionSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListSubscriptions(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cats, err := m.categoryNames(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rates, err := m.loadRates(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, summary(rows, cats, rates))
}

// scanSubscriptions moves auto-renewing subscriptions past their renewal
// date to the next cycle and sends due renewal reminders. It is idempotent:
// running it many times a day sends each reminder once.
func (m *Module) scanSubscriptions(ctx context.Context, now time.Time) error {
	rows, err := m.q.ListSubscriptions(ctx)
	if err != nil {
		return err
	}
	cats, err := m.categoryNames(ctx)
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
			next = renewUntil(next, day, int(s.CycleCount), s.CycleUnit)
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
		s, err = m.q.UpdateSubscription(ctx, db.UpdateSubscriptionParams{ID: s.ID, Name: s.Name, Category: s.Category,
			CategoryID: s.CategoryID, Amount: s.Amount, Currency: s.Currency, Cycle: s.Cycle, CycleDays: s.CycleDays,
			CycleCount: s.CycleCount, CycleUnit: s.CycleUnit, NextRenewal: next.Format(dateLayout),
			RemindDaysBefore: s.RemindDaysBefore, Reminded: mustJSON(reminded), Url: s.Url, Note: s.Note, Account: s.Account, AutoRenew: s.AutoRenew,
			ArchivedAt: s.ArchivedAt, UpdatedAt: now})
		if err != nil {
			return err
		}
		out := toAPISubscription(s, day, cats)
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
