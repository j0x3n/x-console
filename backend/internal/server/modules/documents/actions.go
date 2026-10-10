package documents

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "documents.list",
		Title: "查看证件档案",
		Description: "List the user's papers and warranties (passport, ID, visa, contract, insurance, items) sorted by expiry. " +
			"Each has kind, name, holder, expiresOn (YYYY-MM-DD, empty if none), daysLeft (negative when expired) and status expired, soon, ok or none. " +
			"Document numbers are not returned. Optional `kind` and `q` filter the list.",
		Input:  actions.Schema(`{"type":"object","properties":{"kind":{"type":"string","description":"passport, id_card, driver_license, visa, contract, insurance, item, other, or c:<name> for a type the user added"},"q":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Kind *api.DocumentKind `json:"kind"`
				Q    string            `json:"q"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			items, sum, err := m.list(ctx, in.Kind, in.Q, false)
			if err != nil {
				return nil, err
			}
			for i := range items {
				items[i].Number = ""
			}
			return map[string]any{"items": items, "summary": sum}, nil
		},
	})
}
