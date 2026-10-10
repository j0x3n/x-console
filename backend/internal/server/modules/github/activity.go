package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

// activitySource tells the journal (B118) about the user's own commits and
// pull requests. It reads what the module has cached (the latest commits of
// each watched repository and its recent pull requests), so it does not call
// the forge and does not reach further back than the cache does.
type activitySource struct{ m *Module }

func (s activitySource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	keys, err := s.m.selected(ctx, nil, nil)
	if err != nil {
		if errors.Is(err, httpx.ErrIntegrationMissing) {
			return nil, nil
		}
		return nil, err
	}
	var out []contracts.Activity
	logins := map[int64]string{}
	for _, k := range keys {
		login, ok := logins[k.ConnectionID]
		if !ok {
			cfg, err := s.m.accountConfig(ctx, k.ConnectionID)
			if err != nil {
				continue
			}
			login = strings.ToLower(cfg.Login)
			logins[k.ConnectionID] = login
		}
		// without a known login there is no telling which commits are the user's
		if login == "" {
			continue
		}
		commits, err := s.m.cached(ctx, k, "commit")
		if err != nil {
			return nil, err
		}
		for _, row := range commits {
			var c api.GitHubCommit
			if json.Unmarshal(row.Data, &c) != nil || strings.ToLower(c.Author) != login || !inRange(c.CommittedAt, from, until) {
				continue
			}
			first, _, _ := strings.Cut(strings.TrimSpace(c.Message), "\n")
			short := c.Sha
			if len(short) > 7 {
				short = short[:7]
			}
			out = append(out, contracts.Activity{
				Ref: fmt.Sprintf("commit:%d:%s:%s", k.ConnectionID, k.Repo, c.Sha), Module: "github", Kind: "commit", At: c.CommittedAt,
				Title: first, Detail: k.Repo + " · " + short, Link: c.Url,
			})
		}
		pulls, err := s.m.cached(ctx, k, "pull")
		if err != nil {
			return nil, err
		}
		for _, row := range pulls {
			var p cachedPull
			if json.Unmarshal(row.Data, &p) != nil || strings.ToLower(p.Author) != login {
				continue
			}
			ref := fmt.Sprintf("%d:%s:%d", k.ConnectionID, k.Repo, p.Number)
			if inRange(p.CreatedAt, from, until) {
				out = append(out, contracts.Activity{
					Ref: "pr:" + ref + ":open", Module: "github", Kind: "pr", At: p.CreatedAt,
					Title: fmt.Sprintf("开了 PR #%d %s", p.Number, p.Title), Detail: k.Repo, Link: p.Url,
				})
			}
			if p.State == api.GitHubPullState("merged") && inRange(p.UpdatedAt, from, until) {
				out = append(out, contracts.Activity{
					Ref: "pr:" + ref + ":merged", Module: "github", Kind: "pr", At: p.UpdatedAt,
					Title: fmt.Sprintf("合并了 PR #%d %s", p.Number, p.Title), Detail: k.Repo, Link: p.Url,
				})
			}
		}
	}
	return out, nil
}

func inRange(t, from, until time.Time) bool { return !t.Before(from) && t.Before(until) }
