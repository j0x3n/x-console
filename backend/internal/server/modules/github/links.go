package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
)

func (m *Module) linkPullScoped(ctx context.Context, k repoKey, number int, title, branch, htmlURL string) {
	save := func(kind, ref string) error {
		return m.putObject(ctx, k, "link", object(fmt.Sprintf("%d:%s:%s", number, kind, ref), db.GithubLink{Repo: k.Repo, Number: int64(number), Kind: kind, Ref: ref, CreatedAt: m.now()}))
	}
	if id := codingTaskID(branch); id > 0 {
		if err := save("coding_task", strconv.FormatInt(id, 10)); err != nil {
			m.log.Warn("github coding link", "err", err)
		}
	}
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return
	}
	ref := fmt.Sprintf("%s#%d", k.Repo, number)
	for _, key := range issueKeysIn(title, branch) {
		var l db.GithubLink
		if m.cachedOne(ctx, k, "link", fmt.Sprintf("%d:issue:%s", number, key), &l) == nil {
			continue
		}
		if _, err := issues.Get(ctx, key); err != nil {
			continue
		}
		if err := issues.AttachLink(ctx, key, contracts.IssueLink{Kind: "pull_request", Title: ref + " " + title, URL: htmlURL, Ref: ref}); err != nil {
			m.log.Warn("github issue link", "err", err)
			continue
		}
		if err := save("issue", key); err != nil {
			m.log.Warn("github save link", "err", err)
		}
	}
}
