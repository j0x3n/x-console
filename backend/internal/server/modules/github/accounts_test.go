package github_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// B62: the GitHub page takes its token from a Git account.

type gitConnection struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	HasToken bool   `json:"hasToken"`
	Username string `json:"username"`
	// deprecated, still sent
	UseGithubModule bool `json:"useGithubModule"`
}

func createConnection(t *testing.T, env *testutil.Env, body map[string]any) gitConnection {
	t.Helper()
	var out struct {
		Connection gitConnection `json:"connection"`
	}
	env.MustDo(http.MethodPost, "/git-connections", body, &out)
	return out.Connection
}

func connections(t *testing.T, env *testutil.Env) []gitConnection {
	t.Helper()
	var out []gitConnection
	env.MustDo(http.MethodGet, "/git-connections", nil, &out)
	return out
}

func TestGitAccountMigration(t *testing.T) {
	env := testutil.New(t)
	gh := newFakeGitHub(t)
	ctx := context.Background()
	m := githubModule(t, env)
	// An old install: the token sits in the GitHub settings, a Git
	// connection borrows it, and the migration has not run yet.
	env.Elevate()
	env.MustDo(http.MethodPut, "/github/config", map[string]any{
		"token": gh.token, "repos": []string{repo}, "apiUrl": gh.URL(),
	}, nil)
	borrowed := createConnection(t, env, map[string]any{"kind": "github", "name": "旧连接", "useGithubModule": true})
	if err := env.App.Deps.Settings.Delete(ctx, "github.migrated_b62"); err != nil {
		t.Fatal(err)
	}

	if err := m.MigrateB62(ctx); err != nil {
		t.Fatal(err)
	}
	list := connections(t, env)
	if len(list) != 2 {
		t.Fatalf("connections after migration: %+v", list)
	}
	var imported gitConnection
	for _, c := range list {
		if c.ID != borrowed.ID {
			imported = c
		} else if c.UseGithubModule || !c.HasToken {
			t.Fatalf("borrowing connection kept borrowing: %+v", c)
		}
	}
	if imported.Kind != "github" || imported.Name != "GitHub" || !imported.HasToken {
		t.Fatalf("imported: %+v", imported)
	}
	var cfg api.GitHubConfig
	env.MustDo(http.MethodGet, "/github/config", nil, &cfg)
	if cfg.ConnectionId == nil || *cfg.ConnectionId != imported.ID || !cfg.HasToken {
		t.Fatalf("config after migration: %+v", cfg)
	}
	if st := syncNow(t, env); st.LastError != nil && *st.LastError != "" {
		t.Fatalf("sync with the imported account: %+v", st)
	}

	// Only once.
	if err := m.MigrateB62(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(connections(t, env)); n != 2 {
		t.Fatalf("second run made more connections: %d", n)
	}
}

func TestGitHubUsesGitAccount(t *testing.T) {
	env := testutil.New(t)
	gh := newFakeGitHub(t)
	env.Elevate()
	forgejo := createConnection(t, env, map[string]any{"kind": "forgejo", "name": "家里", "baseUrl": "https://git.example.com", "token": "fj"})
	account := createConnection(t, env, map[string]any{"kind": "github", "name": "GitHub", "baseUrl": gh.URL(), "token": gh.token})

	status, raw := env.Do(http.MethodPut, "/github/config", map[string]any{"repos": []string{repo}, "connectionId": forgejo.ID}, nil)
	if status != http.StatusBadRequest || errCode(raw) != "validation_failed" {
		t.Fatalf("forgejo account: %d %s", status, raw)
	}
	status, raw = env.Do(http.MethodPut, "/github/config", map[string]any{"repos": []string{repo}, "connectionId": 999}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("missing account: %d %s", status, raw)
	}

	var cfg api.GitHubConfig
	env.MustDo(http.MethodPut, "/github/config", map[string]any{"repos": []string{repo}, "connectionId": account.ID}, &cfg)
	if cfg.ConnectionId == nil || *cfg.ConnectionId != account.ID || !cfg.HasToken || cfg.ApiUrl != gh.URL() {
		t.Fatalf("config: %+v", cfg)
	}
	// The fake answers only with the account's token.
	gh.set(func(f *fakeGitHub) {
		f.pulls[repo] = []map[string]any{pull(7, "账号的令牌", "feature", "abc")}
	})
	syncNow(t, env)
	if got := pulls(t, env); len(got) != 1 || got[0].Number != 7 {
		t.Fatalf("pulls with the account token: %+v", got)
	}
}
