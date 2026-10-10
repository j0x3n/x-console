package credentials

import (
	"context"
	"encoding/json"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/api"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "credentials.list",
		Title: "查看密钥",
		Description: "List the user's API keys, access tokens and SSH keys (facts only, the stored secret is never returned) sorted by what needs attention first. " +
			"Each has kind, name, platform, account, usedBy (servers and projects it is used on), scopes, hint (last characters, to recognise it), " +
			"expiresOn and rotatedOn (YYYY-MM-DD, empty if none), expiresIn and rotateDueIn (days, negative when overdue) and status expired, soon, stale, ok or none. " +
			"Use `q` to find what is used on a server or project, or what belongs to a platform. Optional `kind` filters the list.",
		Input:  actions.Schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["api_key","access_token","ssh_key","signing_key","other"]},"q":{"type":"string"}},"additionalProperties":false}`),
		Effect: actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Kind *api.CredentialKind `json:"kind"`
				Q    string              `json:"q"`
			}
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &in); err != nil {
					return nil, err
				}
			}
			if in.Kind != nil && !in.Kind.Valid() {
				return nil, httpx.Invalid("不认识的类型")
			}
			items, sum, err := m.list(ctx, in.Kind, in.Q, false)
			if err != nil {
				return nil, err
			}
			return map[string]any{"items": items, "summary": sum}, nil
		},
	})
}
