package coding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding/api"
)

// autoPR only handles tasks the user assigned with openPr enabled.
func (m *Module) autoPR(ctx context.Context, id int64) {
	m.autoPRMu.Lock()
	defer m.autoPRMu.Unlock()
	row, err := m.row(ctx, id)
	if err != nil || row.AutoOpenPr == 0 || row.Status != statusReview || row.WaitingQuestion != "" || m.building(id) {
		return
	}
	if row.AiAgentID == nil {
		return
	}
	agents, ok := module.Lookup[contracts.AIAgents](m.d.Registry, contracts.AIAgentsKey)
	if !ok {
		return
	}
	a, err := agents.Get(ctx, *row.AiAgentID)
	if err != nil || !a.Enabled {
		return
	}
	if a.AutoBuild {
		if _, err := m.stepsFor(row); err == nil && row.BuildStatus != buildPassed {
			return
		}
	}
	title := titleOf(stripIssueHeader(row.Prompt, row.IssueKey))
	body := fmt.Sprintf("卡片：%s\n\n改动：\n", row.IssueKey)
	if issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey); ok && row.IssueKey != "" {
		if issue, err := issues.Get(ctx, row.IssueKey); err == nil {
			title = issue.Title
		}
	}
	var files []api.ChangedFile
	_ = json.Unmarshal([]byte(row.ChangedFiles), &files)
	for _, f := range files {
		body += "- " + f.Path + "\n"
	}
	if strings.HasSuffix(body, "改动：\n") {
		body += "见 PR 文件改动。\n"
	}
	err = m.commit(ctx, id, row.IssueKey+": "+title)
	if err == nil {
		err = m.openPR(ctx, id, api.PullRequestRequest{Title: &title, Body: &body})
	}
	if err != nil {
		m.d.Log.Warn("coding: automatic PR", "task", id, "err", err)
		_, _ = m.d.DB.ExecContext(ctx, "UPDATE coding_tasks SET error=? WHERE id=?", "自动提 PR 失败："+err.Error(), id)
	}
	m.publishTask(ctx, id)
}
