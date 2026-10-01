package github

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

type accountState struct {
	credential string
	rate       *rateState
	last       time.Time
}

func (m *Module) accountState(cfg config) *accountState {
	m.accountMu.Lock()
	defer m.accountMu.Unlock()
	key := cfg.APIURL + "\x00" + cfg.Token
	s := m.accounts[cfg.ConnectionID]
	if s == nil || s.credential != key {
		r := &rateState{remaining: -1}
		if cfg.ConnectionID == 0 {
			r = m.rate
		}
		s = &accountState{credential: key, rate: r}
		m.accounts[cfg.ConnectionID] = s
	}
	return s
}
func (m *Module) accountConfig(ctx context.Context, id int64) (config, error) {
	if id == 0 {
		c, err := m.loadLegacyConfig(ctx)
		c.Forge = "github"
		c.ConnectionID = 0
		return c, err
	}
	svc, ok := module.Lookup[contracts.GitAccounts](m.d.Registry, contracts.GitAccountsKey)
	if !ok {
		return config{}, httpx.NewError(501, "feature_unavailable", "Git 账号功能没有启用")
	}
	a, err := svc.Account(ctx, id)
	if err != nil {
		return config{}, err
	}
	base, token, err := svc.Credentials(ctx, id)
	if err != nil {
		return config{}, err
	}
	return config{APIURL: base, Token: token, Login: a.Username, ConnectionID: id, Forge: a.Kind, Name: a.Name}, nil
}
func (m *Module) normalizeWatches(ctx context.Context, in []api.RepoWatch) ([]api.RepoWatch, error) {
	out := []api.RepoWatch{}
	seen := map[repoKey]bool{}
	for _, w := range in {
		if w.ConnectionId < 0 {
			return nil, httpx.Invalid("Git 账号编号不正确")
		}
		repo, err := normalizeRepo(w.Repo)
		if err != nil {
			return nil, err
		}
		if _, err := m.accountConfig(ctx, w.ConnectionId); err != nil {
			if errors.Is(err, httpx.ErrNotFound) {
				return nil, httpx.Invalid("没有这个 Git 账号")
			}
			return nil, err
		}
		k := repoKey{w.ConnectionId, strings.ToLower(repo)}
		if !seen[k] {
			seen[k] = true
			out = append(out, api.RepoWatch{ConnectionId: w.ConnectionId, Repo: repo})
		}
	}
	if len(out) > 100 {
		return nil, httpx.Invalid("最多关注 100 个仓库")
	}
	return out, nil
}
func (m *Module) syncAccounts(ctx context.Context, scheduled bool) error {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	if err := m.migrateB70(ctx); err != nil {
		return err
	}
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return err
	}
	m.setSyncing(true)
	defer m.setSyncing(false)
	grouped := map[int64][]string{}
	order := []int64{}
	for _, w := range cfg.Watches {
		if _, ok := grouped[w.ConnectionId]; !ok {
			order = append(order, w.ConnectionId)
		}
		grouped[w.ConnectionId] = append(grouped[w.ConnectionId], w.Repo)
	}
	if len(order) == 0 && !cfg.HasWatches && cfg.Token != "" {
		order = append(order, cfg.ConnectionID)
	}
	if len(order) == 0 && cfg.Token != "" {
		order = append(order, cfg.ConnectionID)
	}
	var errs []string
	for _, id := range order {
		ac, err := m.accountConfig(ctx, id)
		if err != nil {
			errs = append(errs, fmt.Sprintf("账号 %d：%s", id, err))
			continue
		}
		if ac.Token == "" {
			errs = append(errs, fmt.Sprintf("账号 %d 没有令牌", id))
			continue
		}
		s := m.accountState(ac)
		m.accountMu.Lock()
		last := s.last
		m.accountMu.Unlock()
		remaining, limit, reset := s.rate.info()
		if scheduled && ac.Forge == "github" && remaining >= 0 && limit > 0 && remaining < limit/10 && reset.After(m.now()) && m.now().Sub(last) < slowSyncInterval {
			continue
		}
		c := m.client(ac)
		var me ghUser
		if err := c.get(ctx, "/user", nil, &me); err != nil {
			errs = append(errs, fmt.Sprintf("账号 %d：%s", id, err))
			continue
		}
		ac.Login = me.Login
		if id == cfg.ConnectionID {
			if err := m.d.Settings.Set(ctx, keyLogin, me.Login); err != nil {
				return err
			}
		}
		for _, repo := range grouped[id] {
			k := repoKey{id, repo}
			err := m.syncRepository(ctx, c, ac, k)
			var info api.WatchedRepo
			_ = m.cachedOne(ctx, k, "repo", "info", &info)
			if err != nil {
				info.SyncError = ptr(err.Error())
				errs = append(errs, repo+"："+err.Error())
			} else {
				info.SyncError = nil
			}
			if info.Repo != "" {
				if saveErr := m.putObject(ctx, k, "repo", object("info", info)); saveErr != nil {
					return saveErr
				}
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if isRateLimited(err) {
				break
			}
		}
		m.accountMu.Lock()
		s.last = m.now()
		m.accountMu.Unlock()
	}
	result := lastSync{At: m.now(), Error: strings.Join(errs, "；")}
	if err := m.d.Settings.Set(ctx, keyLastSync, result); err != nil {
		return err
	}
	m.d.Bus.Publish("github.synced", map[string]any{"at": result.At, "error": result.Error})
	if result.Error != "" {
		return errors.New(result.Error)
	}
	return nil
}
