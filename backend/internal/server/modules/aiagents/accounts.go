package aiagents

import (
	"context"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/db"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

// B62: the Git connections are the one place for Git tokens. The GitHub
// module takes its token from here.

var _ contracts.GitAccounts = (*Module)(nil)

// Account implements contracts.GitAccounts.
func (m *Module) Account(ctx context.Context, id int64) (contracts.GitAccount, error) {
	c, err := m.connection(ctx, id)
	if err != nil {
		return contracts.GitAccount{}, err
	}
	return contracts.GitAccount{ID: c.ID, Kind: c.Kind, Name: c.Name, Username: c.Username}, nil
}

// Credentials implements contracts.GitAccounts. A connection that still
// borrows the GitHub module's token has none of its own: the GitHub module
// asking for it would go round in a circle.
func (m *Module) Credentials(ctx context.Context, id int64) (string, string, error) {
	c, err := m.connection(ctx, id)
	if err != nil {
		return "", "", err
	}
	if c.TokenEnc == "" {
		return "", "", httpx.Invalid("这个 Git 账号没有令牌")
	}
	token, err := m.d.Secrets.Open(c.TokenEnc)
	if err != nil {
		return "", "", err
	}
	return apiBase(c.Kind, c.BaseUrl), token, nil
}

// ImportGitHub implements contracts.GitAccounts.
func (m *Module) ImportGitHub(ctx context.Context, apiURL, token, login string) (int64, error) {
	base, err := normalizeBaseURL(kindGitHub, apiURL)
	if err != nil {
		return 0, err
	}
	tokenEnc, err := m.d.Secrets.Seal(strings.TrimSpace(token))
	if err != nil {
		return 0, err
	}
	secretEnc, err := m.d.Secrets.Seal(secrets.RandomToken(24))
	if err != nil {
		return 0, err
	}
	var id int64
	err = func() error {
		tx, err := m.d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		q := m.q.WithTx(tx)
		c, err := q.CreateConnection(ctx, db.CreateConnectionParams{Kind: kindGitHub, Name: "GitHub", BaseUrl: base,
			TokenEnc: tokenEnc, WebhookSecretEnc: secretEnc, CreatedAt: m.now()})
		if err != nil {
			return err
		}
		id = c.ID
		if login != "" {
			if err := q.SetConnectionCheck(ctx, db.SetConnectionCheckParams{Username: login, ID: c.ID}); err != nil {
				return err
			}
		}
		if err := q.AdoptModuleToken(ctx, tokenEnc); err != nil {
			return err
		}
		return tx.Commit()
	}()
	m.d.Audit.Record(ctx, "git_connection.import", "", map[string]any{"id": id}, err)
	return id, err
}
