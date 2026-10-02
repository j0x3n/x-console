package contracts

import (
	"context"
	"encoding/json"
)

const CodingControlKey = "coding.control"

// CodingControl exposes existing task operations to AI-agent runs.
type CodingControl interface {
	ResolveBoardRepo(context.Context, int64, string, GitRepository) (int64, error)
	CancelCoding(context.Context, int64) error
	ResumeCoding(context.Context, int64, string) error
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
