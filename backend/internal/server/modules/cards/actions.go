package cards

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "cards.create",
		Title: "新建复习卡片",
		Description: "Create a review card (a question and its answer) that is scheduled with spaced repetition. " +
			"Keep it to one fact: the question must be answerable on its own and the answer short. " +
			"`deck` defaults to 默认. `sourceTitle` and `sourceLink` (an in-app path like /notes/3 or a web address) say where it came from. Returns the card with its id.",
		Input:  actions.Schema(`{"type":"object","properties":{"question":{"type":"string"},"answer":{"type":"string"},"deck":{"type":"string"},"sourceTitle":{"type":"string"},"sourceLink":{"type":"string"}},"required":["question","answer"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in api.CardInput
			if err := json.Unmarshal(raw, &in); err != nil {
				return nil, httpx.Invalid("参数不正确")
			}
			card, err := m.createCard(ctx, in)
			if err != nil {
				return nil, err
			}
			return toAPI(card), nil
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:  "cards.due_count",
		Title: "今天待复习的卡片数",
		Description: "How many review cards are left for today, after the daily limits. Returns due (all), dueReview (cards seen before) and dueNew (new cards), " +
			"plus reviewedToday. The user reviews them on the cards page.",
		Input:  actions.Schema(`{"type":"object","additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, _ json.RawMessage) (any, error) {
			s, err := m.summary(ctx, "")
			if err != nil {
				return nil, err
			}
			return map[string]int{"due": s.Due, "dueReview": s.DueReview, "dueNew": s.DueNew, "reviewedToday": s.ReviewedToday + s.LearnedToday}, nil
		},
	})
}
