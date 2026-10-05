package github

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B109: this month's GitHub Actions minutes. See docs/specs/B109.md.

const (
	keyCIIncluded      = "github.ci_included_minutes"
	defaultCIIncluded  = 2000
	maxCIIncluded      = 1_000_000
	usageTTL           = 10 * time.Minute
	usageCheckInterval = 30 * time.Minute
)

var (
	errGitHubNotConfigured = httpx.NewError(http.StatusPreconditionFailed, "github_not_configured", "还没有设置 GitHub 账号")
	errBillingForbidden    = httpx.NewError(http.StatusForbidden, "github_billing_forbidden", "令牌没有读取账单的权限")
)

// Minutes on Windows and macOS runners count double and ten times.
var osWeight = map[string]float64{"linux": 1, "windows": 2, "macos": 10}

// usageResult is one fetch. includedMinutes is what GitHub said, 0 when it
// said nothing; the setting fills it in when answering.
type usageResult struct {
	login    string
	connID   int64
	month    string
	byOS     map[string]float64 // weighted minutes
	included int
	at       time.Time
}

// usageCache keeps the latest result and the login of each token in memory.
// The key holds the token, so another token never sees an old result.
type usageCache struct {
	mu     sync.Mutex
	key    string
	result usageResult
	logins map[string]string
}

func (c *usageCache) get(key string, now time.Time) (usageResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key != key || now.Sub(c.result.at) >= usageTTL {
		return usageResult{}, false
	}
	return c.result, true
}

func (c *usageCache) put(key string, r usageResult) {
	c.mu.Lock()
	c.key, c.result = key, r
	c.mu.Unlock()
}

func (c *usageCache) login(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logins[key]
}

func (c *usageCache) setLogin(key, login string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.logins == nil || len(c.logins) > 20 {
		c.logins = map[string]string{}
	}
	c.logins[key] = login
}

// GetGitHubActionsUsage answers this month's Actions minutes.
func (m *Module) GetGitHubActionsUsage(w http.ResponseWriter, r *http.Request, params api.GetGitHubActionsUsageParams) {
	res, err := m.actionsUsage(r.Context(), params.Refresh != nil && *params.Refresh)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.usageToAPI(r.Context(), res)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) usageToAPI(ctx context.Context, res usageResult) (api.GitHubActionsUsage, error) {
	included := res.included
	if included <= 0 {
		var err error
		if included, err = m.ciIncludedMinutes(ctx); err != nil {
			return api.GitHubActionsUsage{}, err
		}
	}
	out := api.GitHubActionsUsage{Month: res.month, IncludedMinutes: included, FetchedAt: res.at}
	out.ByOS.Linux = int(math.Round(res.byOS["linux"]))
	out.ByOS.Windows = int(math.Round(res.byOS["windows"]))
	out.ByOS.Macos = int(math.Round(res.byOS["macos"]))
	out.UsedMinutes = out.ByOS.Linux + out.ByOS.Windows + out.ByOS.Macos
	return out, nil
}

// ciIncludedMinutes is the free minutes per month from the settings.
func (m *Module) ciIncludedMinutes(ctx context.Context) (int, error) {
	n := defaultCIIncluded
	if err := m.d.Settings.Get(ctx, keyCIIncluded, &n); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return 0, err
	}
	if n <= 0 {
		n = defaultCIIncluded
	}
	return n, nil
}

func (m *Module) setCIIncludedMinutes(ctx context.Context, n int) error {
	if n < 1 || n > maxCIIncluded {
		return httpx.Invalid("每月免费 CI 分钟数要在 1 到 1000000 之间")
	}
	return m.d.Settings.Set(ctx, keyCIIncluded, n)
}

// usageAccount picks the GitHub account to read billing from: the one the
// repositories page used before B70, else the first watched GitHub account.
func (m *Module) usageAccount(ctx context.Context) (config, error) {
	cfg, err := m.loadConfig(ctx)
	if err != nil {
		return config{}, err
	}
	ids := []int64{cfg.ConnectionID}
	for _, w := range cfg.Watches {
		ids = append(ids, w.ConnectionId)
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		c, err := m.accountConfig(ctx, id)
		if err != nil {
			continue // deleted account: try the next one
		}
		if c.Forge == "github" && c.Token != "" {
			return c, nil
		}
	}
	return config{}, errGitHubNotConfigured
}

// actionsUsage reads this month's minutes, from the cache when it is fresh.
func (m *Module) actionsUsage(ctx context.Context, refresh bool) (usageResult, error) {
	cfg, err := m.usageAccount(ctx)
	if err != nil {
		return usageResult{}, err
	}
	now := m.now()
	month := now.Format("2006-01")
	tokenKey := cfg.APIURL + "\x00" + cfg.Token
	key := tokenKey + "\x00" + month
	if !refresh {
		if res, ok := m.usage.get(key, now); ok {
			return res, nil
		}
	}
	c := m.client(cfg)
	login := m.usage.login(tokenKey)
	if login == "" {
		var me ghUser
		if err := c.get(ctx, "/user", nil, &me); err != nil {
			return usageResult{}, usageError(err)
		}
		login = me.Login
		m.usage.setLogin(tokenKey, login)
	}
	res, err := fetchUsage(ctx, c, login, now)
	if err != nil {
		return usageResult{}, err
	}
	res.login, res.connID, res.month, res.at = login, cfg.ConnectionID, month, now
	m.usage.put(key, res)
	return res, nil
}

func usageError(err error) error {
	if statusOf(err) == http.StatusForbidden && !isRateLimited(err) {
		return errBillingForbidden
	}
	return httpx.NewError(http.StatusBadGateway, "github_unavailable", err.Error())
}

type ghBillingUsage struct {
	UsageItems []struct {
		Product  string  `json:"product"`
		SKU      string  `json:"sku"`
		Quantity float64 `json:"quantity"`
		UnitType string  `json:"unitType"`
	} `json:"usageItems"`
}

type ghLegacyActionsBilling struct {
	TotalMinutesUsed     float64            `json:"total_minutes_used"`
	IncludedMinutes      float64            `json:"included_minutes"`
	MinutesUsedBreakdown map[string]float64 `json:"minutes_used_breakdown"`
}

// fetchUsage tries the new billing API first and falls back to the old
// Actions billing API when the account does not have the new one.
func fetchUsage(ctx context.Context, c *restClient, login string, now time.Time) (usageResult, error) {
	user := "/users/" + url.PathEscape(login) + "/settings/billing"
	q := url.Values{"year": {strconv.Itoa(now.Year())}, "month": {strconv.Itoa(int(now.Month()))}}
	var fresh ghBillingUsage
	err := c.get(ctx, user+"/usage", q, &fresh)
	if err == nil {
		res := usageResult{byOS: map[string]float64{}}
		for _, it := range fresh.UsageItems {
			unit := strings.ToLower(it.UnitType)
			if !strings.EqualFold(it.Product, "actions") || (unit != "" && !strings.Contains(unit, "minute")) {
				continue // storage and other products
			}
			sys := runnerOS(it.SKU)
			res.byOS[sys] += it.Quantity * osWeight[sys]
		}
		return res, nil
	}
	if s := statusOf(err); s != http.StatusNotFound && s != http.StatusGone {
		return usageResult{}, usageError(err)
	}
	var old ghLegacyActionsBilling
	if err := c.get(ctx, user+"/actions", nil, &old); err != nil {
		if statusOf(err) == http.StatusNotFound {
			return usageResult{}, errBillingForbidden // GitHub hides what the token may not read
		}
		return usageResult{}, usageError(err)
	}
	res := usageResult{byOS: map[string]float64{}, included: int(old.IncludedMinutes)}
	for sku, minutes := range old.MinutesUsedBreakdown {
		sys := runnerOS(sku)
		res.byOS[sys] += minutes * osWeight[sys]
	}
	if len(old.MinutesUsedBreakdown) == 0 {
		res.byOS["linux"] = old.TotalMinutesUsed
	}
	return res, nil
}

// runnerOS reads the system from a SKU such as "Actions Windows" or
// "ubuntu_4_core". Unknown names count as Linux.
func runnerOS(sku string) string {
	s := strings.ToLower(sku)
	switch {
	case strings.Contains(s, "mac"):
		return "macos"
	case strings.Contains(s, "windows"):
		return "windows"
	}
	return "linux"
}

// checkCIQuota sends one notice at 80% and one at 100% each month. A jump
// straight past 100% sends only the 100% notice.
func (m *Module) checkCIQuota(ctx context.Context) error {
	ns, err := m.notifySettings(ctx)
	if err != nil {
		return err
	}
	if ns.CiQuota != nil && !*ns.CiQuota {
		return nil
	}
	res, err := m.actionsUsage(ctx, false)
	if err != nil {
		var he *httpx.Error
		if errors.As(err, &he) {
			return nil // not set up, no billing permission or GitHub is down: try next time
		}
		return err
	}
	u, err := m.usageToAPI(ctx, res)
	if err != nil {
		return err
	}
	pct := u.UsedMinutes * 100 / u.IncludedMinutes
	if pct < 80 {
		return nil
	}
	monthStart := time.Date(m.now().Year(), m.now().Month(), 1, 0, 0, 0, 0, time.UTC)
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	sentThisMonth := func(event string) (bool, error) {
		var at time.Time
		err := tx.QueryRowContext(ctx, `SELECT started_at FROM github_notify_windows WHERE connection_id=? AND repo=? AND event=?`, res.connID, res.login, event).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return err == nil && !at.Before(monthStart), err
	}
	mark := func(event string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO github_notify_windows(connection_id,repo,event,started_at,sent,suppressed) VALUES(?,?,?,?,1,0) ON CONFLICT(connection_id,repo,event) DO UPDATE SET started_at=excluded.started_at,sent=1,suppressed=0`, res.connID, res.login, event, m.now())
		return err
	}
	var send *notify.Notification
	body := fmt.Sprintf("已用 %d / %d 分钟（Linux %d · Windows %d · macOS %d）", u.UsedMinutes, u.IncludedMinutes, u.ByOS.Linux, u.ByOS.Windows, u.ByOS.Macos)
	data := map[string]any{"login": res.login, "month": u.Month, "usedMinutes": u.UsedMinutes, "includedMinutes": u.IncludedMinutes}
	done80, err := sentThisMonth("ci_quota_80")
	if err != nil {
		return err
	}
	if pct >= 100 {
		done100, err := sentThisMonth("ci_quota_100")
		if err != nil {
			return err
		}
		if !done100 {
			send = &notify.Notification{Kind: "github.ci_quota_100", Title: "本月 CI 时长已用完", Body: body, Link: "/github", Source: "github", Priority: notify.PriorityHigh, Data: data}
			if err := mark("ci_quota_100"); err != nil {
				return err
			}
		}
		if !done80 {
			if err := mark("ci_quota_80"); err != nil {
				return err
			}
		}
	} else if !done80 {
		send = &notify.Notification{Kind: "github.ci_quota_80", Title: "本月 CI 时长已用 80%", Body: body, Link: "/github", Source: "github", Data: data}
		if err := mark("ci_quota_80"); err != nil {
			return err
		}
	}
	if send == nil {
		return tx.Commit()
	}
	stored, err := m.d.Notify.SaveTx(ctx, tx, *send)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.d.Notify.Dispatch(ctx, stored)
	return nil
}
