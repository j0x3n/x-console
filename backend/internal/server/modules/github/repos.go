package github

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

const (
	repoListTTL      = 10 * time.Minute
	repoListPages    = 20 // 100 per page, so at most 2000 repositories
	repoDescMaxRunes = 200
)

// availableRepo and availableRepos mirror api.GitHubAvailableRepos.
type availableRepo struct {
	FullName    string     `json:"fullName"`
	Private     bool       `json:"private"`
	Description *string    `json:"description,omitempty"`
	PushedAt    *time.Time `json:"pushedAt,omitempty"`
}

type availableRepos struct {
	FetchedAt time.Time       `json:"fetchedAt"`
	Repos     []availableRepo `json:"repos"`
}

// repoCache keeps the last list for one token and API address. A different
// token or address never sees it, which is also how changing them clears it.
type repoCache struct {
	mu      sync.Mutex
	entries map[string]availableRepos
}

func (c *repoCache) get(key string, now time.Time) (availableRepos, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	list, ok := c.entries[key]
	if !ok || now.Sub(list.FetchedAt) >= repoListTTL {
		return availableRepos{}, false
	}
	return list, true
}

func (c *repoCache) put(key string, list availableRepos) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]availableRepos{}
	}
	if len(c.entries) > 100 {
		c.entries = map[string]availableRepos{}
	}
	c.entries[key] = list
}

type ghUserRepo struct {
	FullName    string     `json:"full_name"`
	Private     bool       `json:"private"`
	Description *string    `json:"description"`
	PushedAt    *time.Time `json:"pushed_at"`
}

// fetchRepos lists the repositories the token can see, most recently pushed
// first. Fine-grained tokens only return the repositories they were given.
func (m *Module) fetchRepos(ctx context.Context, c *restClient) (availableRepos, error) {
	out := availableRepos{FetchedAt: m.now(), Repos: []availableRepo{}}
	for page := 1; page <= repoListPages; page++ {
		var batch []ghUserRepo
		q := pageQuery(c, 100, page)
		if c.forge != "forgejo" {
			q.Set("sort", "pushed")
			q.Set("affiliation", "owner,collaborator,organization_member")
		}
		more, err := c.getPage(ctx, "/user/repos", q, &batch)
		if err != nil {
			return availableRepos{}, err
		}
		for _, r := range batch {
			item := availableRepo{FullName: r.FullName, Private: r.Private, PushedAt: r.PushedAt}
			if r.Description != nil && *r.Description != "" {
				d := truncateRunes(*r.Description, repoDescMaxRunes)
				item.Description = &d
			}
			out.Repos = append(out.Repos, item)
		}
		if !more && (c.forge != "forgejo" || len(batch) < 100) {
			break
		}
	}
	sort.SliceStable(out.Repos, func(i, j int) bool {
		a, b := out.Repos[i].PushedAt, out.Repos[j].PushedAt
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.After(*b)
	})
	return out, nil
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// ListGitHubAvailableRepos is GET /github/available-repos.
func (m *Module) ListGitHubAvailableRepos(w http.ResponseWriter, r *http.Request, params api.ListGitHubAvailableReposParams) {
	legacy, err := m.loadLegacyConfig(r.Context())
	id := legacy.ConnectionID
	if params.ConnectionId != nil {
		id = *params.ConnectionId
	}
	cfg, err := m.accountConfig(r.Context(), id)
	if err == nil && cfg.Token == "" {
		err = httpx.ErrIntegrationMissing
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	key := cfg.APIURL + "\x00" + cfg.Token
	refresh := params.Refresh != nil && *params.Refresh
	if !refresh {
		if list, ok := m.repos.get(key, m.now()); ok {
			httpx.JSON(w, http.StatusOK, list)
			return
		}
	}
	list, err := m.fetchRepos(r.Context(), m.client(cfg))
	if err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "github_unavailable", err.Error()))
		return
	}
	m.repos.put(key, list)
	httpx.JSON(w, http.StatusOK, list)
}
