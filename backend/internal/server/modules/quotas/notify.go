package quotas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Notifications (B112). Each kind is sent once for one window in one period:
// the state row remembers the period's end, and is dropped as soon as the
// condition is over, so the next period can notify again.
const (
	settingsKey = "quotas.notify"

	kindLow     = "quota.low"
	kindEmpty   = "quota.empty"
	kindBalance = "quota.balance_low"
	kindFailed  = "quota.read_failed"

	// lowBelow is the share left, in percent, at and under which a window is
	// said to be running out.
	lowBelow = 10
	// failedAfter is how many reads in a row must fail before the user is
	// told. A Claude read starts a process and is made less often.
	failedAfter       = 3
	failedAfterClaude = 2
)

func (m *Module) notifySettings(ctx context.Context) api.QuotaNotifySettings {
	s := api.QuotaNotifySettings{Low: true, Empty: true, Balance: true, Failed: true}
	if err := m.d.Settings.Get(ctx, settingsKey, &s); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load quota notify settings", "err", err)
	}
	return s
}

// GetQuotaNotify implements api.ServerInterface.
func (m *Module) GetQuotaNotify(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.notifySettings(r.Context()))
}

// PutQuotaNotify implements api.ServerInterface.
func (m *Module) PutQuotaNotify(w http.ResponseWriter, r *http.Request) {
	var body api.QuotaNotifySettings
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.d.Settings.Set(r.Context(), settingsKey, body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

// once sends n unless the state row already says it was sent for this period.
// The row is written only after a send that did not fail.
func (m *Module) once(ctx context.Context, a db.QuotaAccount, window, event, period string, n notify.Notification) {
	prev, err := m.q.GetQuotaNotifyState(ctx, db.GetQuotaNotifyStateParams{AccountID: a.ID, WindowName: window, Event: event})
	if err == nil && prev == period {
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		m.d.Log.Warn("quota notify state", "account", a.ID, "err", err)
		return
	}
	n.Source = "quotas"
	n.Link = "/quotas"
	if _, err := m.d.Notify.Send(ctx, n); err != nil {
		m.d.Log.Warn("quota notify", "account", a.ID, "kind", n.Kind, "err", err)
		return
	}
	if err := m.q.PutQuotaNotifyState(ctx, db.PutQuotaNotifyStateParams{AccountID: a.ID, WindowName: window, Event: event, PeriodEnd: period, SentAt: m.now().UTC()}); err != nil {
		m.d.Log.Warn("quota notify state", "account", a.ID, "err", err)
	}
}

// mark records that an event counts as sent without sending it.
func (m *Module) mark(ctx context.Context, a db.QuotaAccount, window, event, period string) {
	_ = m.q.PutQuotaNotifyState(ctx, db.PutQuotaNotifyStateParams{AccountID: a.ID, WindowName: window, Event: event, PeriodEnd: period, SentAt: m.now().UTC()})
}

func (m *Module) clear(ctx context.Context, a db.QuotaAccount, window, event string) {
	_ = m.q.DeleteQuotaNotifyState(ctx, db.DeleteQuotaNotifyStateParams{AccountID: a.ID, WindowName: window, Event: event})
}

// notifyReading looks at a good reading: windows running out or used up, a
// balance under its limit. A good reading also ends a run of failures.
func (m *Module) notifyReading(ctx context.Context, a db.QuotaAccount, r result) {
	m.clear(ctx, a, "read", kindFailed)
	cfg := m.notifySettings(ctx)
	now := m.now()
	for _, w := range r.windows {
		if w.Aside != nil && *w.Aside {
			continue
		}
		m.checkWindow(ctx, a, w, cfg, now)
	}
	if a.Kind == "deepseek" {
		m.checkBalance(ctx, a, r.balances, cfg)
	}
}

func (m *Module) checkWindow(ctx context.Context, a db.QuotaAccount, w api.QuotaWindow, cfg api.QuotaNotifySettings, now time.Time) {
	period := ""
	reset := ""
	if w.ResetsAt != nil {
		period = w.ResetsAt.UTC().Format(time.RFC3339)
		if !w.ResetsAt.After(now) {
			// already over: the numbers are the old period's
			m.clear(ctx, a, w.Name, kindLow)
			m.clear(ctx, a, w.Name, kindEmpty)
			return
		}
		reset = "，" + durationZh(w.ResetsAt.Sub(now)) + "后重置"
	}
	left := int(math.Round(100 - w.UsedPercent))
	switch {
	case left <= 0:
		if cfg.Empty {
			m.once(ctx, a, w.Name, kindEmpty, period, notify.Notification{
				Kind: kindEmpty, Priority: notify.PriorityHigh,
				Title: fmt.Sprintf("AI 额度已用完：%s %s", a.Name, w.Name),
				Body:  fmt.Sprintf("%s的%s额度用完了%s。", serviceName(a.Kind), w.Name, reset),
			})
		}
		// jumping straight to empty counts as having said "running out"
		m.mark(ctx, a, w.Name, kindLow, period)
	case left <= lowBelow:
		if cfg.Low {
			m.once(ctx, a, w.Name, kindLow, period, notify.Notification{
				Kind:  kindLow,
				Title: fmt.Sprintf("AI 额度快用完：%s %s", a.Name, w.Name),
				Body:  fmt.Sprintf("%s的%s额度只剩 %d%%%s。", serviceName(a.Kind), w.Name, left, reset),
			})
		}
		m.clear(ctx, a, w.Name, kindEmpty)
	default:
		m.clear(ctx, a, w.Name, kindLow)
		m.clear(ctx, a, w.Name, kindEmpty)
	}
}

func (m *Module) checkBalance(ctx context.Context, a db.QuotaAccount, balances []api.QuotaBalance, cfg api.QuotaNotifySettings) {
	limit, err := strconv.ParseFloat(strings.TrimSpace(a.BalanceLow), 64)
	if err != nil || len(balances) == 0 {
		m.clear(ctx, a, "balance", kindBalance)
		return
	}
	first := balances[0]
	amount, err := strconv.ParseFloat(first.Amount, 64)
	if err != nil {
		return
	}
	if amount >= limit {
		m.clear(ctx, a, "balance", kindBalance)
		return
	}
	if !cfg.Balance {
		return
	}
	m.once(ctx, a, "balance", kindBalance, "", notify.Notification{
		Kind: kindBalance, Priority: notify.PriorityHigh,
		Title: fmt.Sprintf("DeepSeek 余额不足：%s", a.Name),
		Body:  fmt.Sprintf("余额 %s %s，低于你设的 %s。", first.Amount, first.Currency, strings.TrimSpace(a.BalanceLow)),
	})
}

// notifyFailure tells once when reads fail several times in a row. A machine
// that is off is not a failed read and never gets here.
func (m *Module) notifyFailure(ctx context.Context, a db.QuotaAccount, count int64, msg string) {
	need := int64(failedAfter)
	if a.Kind == protocol.QuotaKindClaude {
		need = failedAfterClaude
	}
	if count < need || !m.notifySettings(ctx).Failed {
		return
	}
	m.once(ctx, a, "read", kindFailed, "", notify.Notification{
		Kind:  kindFailed,
		Title: fmt.Sprintf("额度读取失败：%s", a.Name),
		Body:  fmt.Sprintf("%s的额度连续 %d 次读取失败：%s", serviceName(a.Kind), count, msg),
	})
}

func serviceName(kind string) string {
	switch kind {
	case "claude":
		return "Claude"
	case "codex":
		return "Codex"
	case "grok":
		return "Grok"
	case "deepseek":
		return "DeepSeek"
	}
	return kind
}

// durationZh writes a time left the short way: "2 小时 14 分", "3 天 4 小时".
func durationZh(d time.Duration) string {
	minutes := int(d.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	days, hours, mins := minutes/1440, minutes%1440/60, minutes%60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	case days > 0:
		return fmt.Sprintf("%d 天", days)
	case hours > 0 && mins > 0 && hours < 10:
		return fmt.Sprintf("%d 小时 %d 分", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%d 小时", hours)
	}
	return fmt.Sprintf("%d 分", mins)
}

// validBalanceLow checks the "notify below" number. Empty means off.
func validBalanceLow(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return "", httpx.Invalid("“余额低于”要填一个不小于 0 的数字")
	}
	return s, nil
}
