package contracts

import (
	"context"
	"encoding/json"
	"time"
)

const CodingControlKey = "coding.control"

// CodingControl exposes existing task operations to AI-agent runs.
type CodingControl interface {
	ResolveBoardRepo(context.Context, int64, string, GitRepository) (int64, error)
	CancelCoding(context.Context, int64) error
	ResumeCoding(context.Context, int64, string) error
	// CodingTasks returns the tasks that still exist, by id.
	CodingTasks(context.Context, []int64) (map[int64]CodingTaskInfo, error)
	// CodingTaskEvents returns events after seq, at most the last 5000.
	CodingTaskEvents(ctx context.Context, id, after int64) ([]CodingTaskEvent, error)
}

// CodingTaskInfo is what an AI-agent run shows of its coding task.
type CodingTaskInfo struct {
	Status, PrURL, Error, WaitingQuestion string
	StartedAt, FinishedAt                 *time.Time
}

type CodingTaskEvent struct {
	Seq        int64
	At         time.Time
	Kind, Text string
}

const AgentRunsKey = "aiagents.runs"

// AgentRuns links a new coding task to the AI-agent run that launched it.
// coding calls it before the task can start, so no task event is missed.
type AgentRuns interface {
	LinkRunTask(ctx context.Context, runID, agentID, taskID int64) error
}

const AssistantDecisionsKey = "ai.decisions"

const CodingQuestionsKey = "aiagents.questions"

type CodingQuestions interface {
	ReceiveCodingQuestion(context.Context, int64, ToolQuestion) error
}

type AssistantDecisions interface {
	AnswerAssistantDecision(context.Context, int64, bool) error
}

// ToolObserver records progress of an autonomous built-in agent.
type ToolObserver interface {
	RecordToolEvent(context.Context, string, string, string, bool)
}

type ToolQuestion struct {
	Title   string   `json:"title"`
	Detail  string   `json:"detail,omitempty"`
	Options []string `json:"options,omitempty"`
}

// ToolDecider pauses a job for a user's answer. The returned context belongs
// to the approving user only for that single confirmed action.
type ToolDecider interface {
	ConfirmTool(context.Context, string, json.RawMessage, bool) (context.Context, bool, error)
	AskUser(context.Context, ToolQuestion) (string, error)
}
