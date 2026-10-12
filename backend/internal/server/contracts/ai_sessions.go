package contracts

import (
	"context"
	"time"
)

const AISessionRunsKey = "aiagents.session_runs"

// AISessionRun 是统一会话列表需要的执行记录。
type AISessionRun struct {
	ID, AgentID                                   int64
	AgentName, AgentKind, AgentAvatar, AgentColor string
	IssueKey, IssueTitle, Summary, Status         string
	TaskID                                        *int64
	PendingCount                                  int
	UpdatedAt                                     time.Time
}

// AISessionRuns 由 aiagents 提供，保留编码任务的实时状态。
type AISessionRuns interface {
	SessionRuns(context.Context) ([]AISessionRun, error)
}
