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
	gh.set(func(f *fakeGitHub) {
		f.billingUsage = readTestdata(t, "billing_usage.json")
		f.publicRepos["jo/x-console"] = true
	})

	got := usage(t, env, "")
	// Private Linux 70+50 is billed; public Windows 15 counts double separately.
	if got.Month != "2026-10" || got.UsedMinutes != 120 || got.PublicMinutes != 30 || got.IncludedMinutes != 2000 ||
		got.ByOS.Linux != 120 || got.ByOS.Windows != 0 || got.ByOS.Macos != 0 {
		t.Fatalf("usage: %+v", got)
	}
	gh.set(func(f *fakeGitHub) {
		if f.billingQuery != "month=10&year=2026" {
			t.Errorf("query: %s", f.billingQuery)
		}
	})
	checkRequests := func(want int) {
		t.Helper()
		gh.set(func(f *fakeGitHub) {
			if f.billingRequests != want || f.repoRequests["jo/app"] != want || f.repoRequests["jo/x-console"] != want {
				t.Fatalf("want %d requests each, billing: %d, repositories: %v", want, f.billingRequests, f.repoRequests)
			}
		})
	}
	checkRequests(1)
	usage(t, env, "")
	clock = clock.Add(9 * time.Minute)
	gh.set(func(f *fakeGitHub) { f.publicRepos["jo/x-console"] = false })
	if got := usage(t, env, ""); got.UsedMinutes != 120 || got.PublicMinutes != 30 {
		t.Fatalf("cached visibility: %+v", got)
	}
	checkRequests(1)
	if got := usage(t, env, "?refresh=true"); got.UsedMinutes != 150 || got.PublicMinutes != 0 || got.ByOS.Windows != 30 {
		t.Fatalf("refreshed visibility: %+v", got)
	}
	checkRequests(2)
	clock = clock.Add(11 * time.Minute)
	gh.set(func(f *fakeGitHub) { f.publicRepos["jo/x-console"] = true })
	if got := usage(t, env, ""); got.UsedMinutes != 120 || got.PublicMinutes != 30 {
		t.Fatalf("expired visibility: %+v", got)
	}
	checkRequests(3)

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

func TestActionsUsageRepositoryLookupFailureIsBilled(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			env, gh := setup(t)
			gh.set(func(f *fakeGitHub) {
				f.billingUsage = readTestdata(t, "billing_usage.json")
				f.publicRepos["jo/app"], f.publicRepos["jo/x-console"] = true, true
				f.repoStatuses["jo/app"] = status
			})
			got := usage(t, env, "")
			if got.UsedMinutes != 120 || got.PublicMinutes != 30 ||
				got.ByOS.Linux != 120 || got.ByOS.Windows != 0 || got.ByOS.Macos != 0 {
				t.Fatalf("failed lookup: %+v", got)
			}
			gh.set(func(f *fakeGitHub) {
				if f.repoRequests["jo/app"] != 1 || f.repoRequests["jo/x-console"] != 1 {
					t.Fatalf("requests: %v", f.repoRequests)
				}
				delete(f.repoStatuses, "jo/app")
			})
			if got := usage(t, env, ""); got.UsedMinutes != 120 || got.PublicMinutes != 30 {
				t.Fatalf("cached failed lookup: %+v", got)
			}
			if got := usage(t, env, "?refresh=true"); got.UsedMinutes != 0 || got.PublicMinutes != 150 {
				t.Fatalf("retried lookup: %+v", got)
			}
			gh.set(func(f *fakeGitHub) {
				if f.repoRequests["jo/app"] != 2 || f.repoRequests["jo/x-console"] != 2 {
					t.Fatalf("refresh requests: %v", f.repoRequests)
				}
			})
		})
	}
}

func TestActionsUsageMissingRepositoryNameAndPublicWeights(t *testing.T) {
	env, gh := setup(t)
	gh.set(func(f *fakeGitHub) {
		f.publicRepos["jo/x-console"] = true
		f.billingUsage = `{"usageItems":[
			{"product":"Actions","sku":"Actions Linux","quantity":120,"unitType":"minutes","repositoryName":""},
			{"product":"Actions","sku":"Actions Windows","quantity":15,"unitType":"minutes"},
			{"product":"Actions","sku":"Actions Linux","quantity":10,"unitType":"minutes","repositoryName":"jo/x-console"},
			{"product":"Actions","sku":"Actions Windows","quantity":15,"unitType":"minutes","repositoryName":"jo/x-console"},
			{"product":"Actions","sku":"Actions macOS","quantity":3,"unitType":"minutes","repositoryName":"jo/x-console"}
		]}`
	})
	got := usage(t, env, "")
	if got.UsedMinutes != 150 || got.PublicMinutes != 70 ||
		got.ByOS.Linux != 120 || got.ByOS.Windows != 30 || got.ByOS.Macos != 0 {
		t.Fatalf("usage: %+v", got)
	}
	gh.set(func(f *fakeGitHub) {
		if len(f.repoRequests) != 1 || f.repoRequests["jo/x-console"] != 1 {
			t.Fatalf("requests: %v", f.repoRequests)
		}
	})
}

func TestActionsUsageLegacyFallback(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		env, gh := setup(t)
		gh.set(func(f *fakeGitHub) {
			f.billingUsageStatus = status
			f.billingActions = readTestdata(t, "billing_actions.json")
		})
		got := usage(t, env, "")
		if got.UsedMinutes != 150 || got.PublicMinutes != 0 || got.IncludedMinutes != 2000 || got.ByOS.Linux != 120 || got.ByOS.Windows != 30 {
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

func TestCIQuotaNotificationsIgnorePublicMinutes(t *testing.T) {
	env, gh := setup(t)
	clock := time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)
	m := githubModule(t, env)
	m.SetNow(func() time.Time { return clock })
	gh.set(func(f *fakeGitHub) {
		f.publicRepos["jo/x-console"] = true
		f.billingUsage = `{"usageItems":[
			{"product":"Actions","sku":"Actions Linux","quantity":1580,"unitType":"minutes","repositoryName":"jo/app"},
			{"product":"Actions","sku":"Actions Windows","quantity":250,"unitType":"minutes","repositoryName":"jo/x-console"}
		]}`
	})
	if err := m.CheckCIQuota(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := usage(t, env, "")
	if got.UsedMinutes != 1580 || got.PublicMinutes != 500 {
		t.Fatalf("usage: %+v", got)
	}
	if countKind(notifications(t, env), "github.ci_quota_80") != 0 || countKind(notifications(t, env), "github.ci_quota_100") != 0 {
		t.Fatalf("public minutes triggered a notice: %v", notifications(t, env))
	}
}
