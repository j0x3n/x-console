package github_test

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B109: this month's Actions minutes.

func readTestdata(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func usage(t *testing.T, env *testutil.Env, query string) api.GitHubActionsUsage {
	t.Helper()
	var out api.GitHubActionsUsage
	env.MustDo(http.MethodGet, "/github/actions-usage"+query, nil, &out)
	return out
}

func TestActionsUsageNotConfigured(t *testing.T) {
	env := testutil.New(t)
	if code, raw := env.Do(http.MethodGet, "/github/actions-usage", nil, nil); code != http.StatusPreconditionFailed || errCode(raw) != "github_not_configured" {
		t.Fatalf("%d %s", code, raw)
	}
}

func TestActionsUsageNewAPIAndCache(t *testing.T) {
	env, gh := setup(t)
	clock := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	githubModule(t, env).SetNow(func() time.Time { return clock })
	gh.set(func(f *fakeGitHub) { f.billingUsage = readTestdata(t, "billing_usage.json") })

	got := usage(t, env, "")
	// Linux 70+50, Windows 15 counts double, storage and packages left out.
	if got.Month != "2026-10" || got.UsedMinutes != 150 || got.IncludedMinutes != 2000 ||
		got.ByOS.Linux != 120 || got.ByOS.Windows != 30 || got.ByOS.Macos != 0 {
		t.Fatalf("usage: %+v", got)
	}
	gh.set(func(f *fakeGitHub) {
		if f.billingQuery != "month=10&year=2026" {
			t.Errorf("query: %s", f.billingQuery)
		}
	})
	billing := func() (n int) {
		gh.set(func(f *fakeGitHub) { n = f.billingRequests })
		return n
	}
	if billing() != 1 {
		t.Fatalf("requests: %d", billing())
	}
	usage(t, env, "")
	clock = clock.Add(9 * time.Minute)
	usage(t, env, "")
	if billing() != 1 {
		t.Fatalf("cached for 10 minutes, requests: %d", billing())
	}
	usage(t, env, "?refresh=true")
	if billing() != 2 {
		t.Fatalf("refresh, requests: %d", billing())
	}
	clock = clock.Add(11 * time.Minute)
	usage(t, env, "")
	if billing() != 3 {
		t.Fatalf("expired, requests: %d", billing())
	}

	// The setting fills in the free minutes; only it changes.
	var cfg api.GitHubConfig
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"ciIncludedMinutes": 3000}, &cfg)
	if cfg.CiIncludedMinutes == nil || *cfg.CiIncludedMinutes != 3000 || len(cfg.Repos) != 1 || !cfg.HasToken {
		t.Fatalf("config: %+v", cfg)
	}
	if got := usage(t, env, ""); got.IncludedMinutes != 3000 {
		t.Fatalf("included: %+v", got)
	}
	if code, raw := env.Do(http.MethodPut, "/github/config", map[string]any{"ciIncludedMinutes": 0}, nil); code != http.StatusBadRequest {
		t.Fatalf("zero minutes: %d %s", code, raw)
	}
}

func TestActionsUsageLegacyFallback(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		env, gh := setup(t)
		gh.set(func(f *fakeGitHub) {
			f.billingUsageStatus = status
			f.billingActions = readTestdata(t, "billing_actions.json")
		})
		got := usage(t, env, "")
		if got.UsedMinutes != 150 || got.IncludedMinutes != 2000 || got.ByOS.Linux != 120 || got.ByOS.Windows != 30 {
			t.Fatalf("%d: %+v", status, got)
		}
	}
}

func TestActionsUsageForbidden(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) { f.billingUsageStatus = http.StatusForbidden })
	if code, raw := env.Do(http.MethodGet, "/github/actions-usage", nil, nil); code != http.StatusForbidden || errCode(raw) != "github_billing_forbidden" {
		t.Fatalf("%d %s", code, raw)
	}
	// The old API hides billing from tokens without permission with a 404.
	gh.set(func(f *fakeGitHub) {
		f.billingUsageStatus, f.billingActionsStatus = http.StatusNotFound, http.StatusNotFound
	})
	if code, raw := env.Do(http.MethodGet, "/github/actions-usage?refresh=true", nil, nil); code != http.StatusForbidden || errCode(raw) != "github_billing_forbidden" {
		t.Fatalf("%d %s", code, raw)
	}
	// The rest of the page still works.
	syncNow(t, env)
	pulls(t, env)
}

func billingAt(minutes int) string {
	return `{"usageItems":[{"product":"Actions","sku":"Actions Linux","quantity":` + strconv.Itoa(minutes) + `,"unitType":"minutes"}]}`
}

func TestCIQuotaNotifications(t *testing.T) {
	env, gh := setup(t)
	clock := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	m := githubModule(t, env)
	m.SetNow(func() time.Time { return clock })
	ctx := context.Background()
	check := func(minutes int) {
		t.Helper()
		gh.set(func(f *fakeGitHub) { f.billingUsage = billingAt(minutes) })
		clock = clock.Add(31 * time.Minute) // past the cache, like the scheduler
		if err := m.CheckCIQuota(ctx); err != nil {
			t.Fatal(err)
		}
	}
	count := func(kind string) int { return countKind(notifications(t, env), kind) }

	check(1580) // 79%
	if count("github.ci_quota_80") != 0 {
		t.Fatal("notice below 80%")
	}
	check(1600)
	check(1700)
	if count("github.ci_quota_80") != 1 || count("github.ci_quota_100") != 0 {
		t.Fatalf("80%%: %v", notifications(t, env))
	}
	check(2000)
	check(2100)
	if count("github.ci_quota_80") != 1 || count("github.ci_quota_100") != 1 {
		t.Fatalf("100%%: %v", notifications(t, env))
	}

	// Next month both are sent again. Straight past 100% sends only that one.
	clock = time.Date(2026, 11, 1, 0, 5, 0, 0, time.UTC)
	check(1650)
	if count("github.ci_quota_80") != 2 {
		t.Fatalf("next month: %v", notifications(t, env))
	}
	clock = time.Date(2026, 12, 1, 0, 5, 0, 0, time.UTC)
	check(2500)
	check(2600)
	if count("github.ci_quota_80") != 2 || count("github.ci_quota_100") != 2 {
		t.Fatalf("jump: %v", notifications(t, env))
	}

	// Turned off: nothing more, and the bell on one repository keeps it off.
	off := false
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}, CiQuota: &off}, nil)
	env.MustDo(http.MethodPut, "/github/notify", api.GitHubNotifySettings{Defaults: allNotify(), Repos: []api.RepoNotifyOverride{}}, nil)
	var ns api.GitHubNotifySettings
	env.MustDo(http.MethodGet, "/github/notify", nil, &ns)
	if ns.CiQuota == nil || *ns.CiQuota {
		t.Fatalf("ciQuota: %+v", ns.CiQuota)
	}
	clock = time.Date(2027, 1, 1, 0, 5, 0, 0, time.UTC)
	check(1900)
	if count("github.ci_quota_80") != 2 {
		t.Fatalf("off: %v", notifications(t, env))
	}
}
