package aiagents

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents/db"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

func webhookPath(id int64) string { return "/hooks/git/" + strconv.FormatInt(id, 10) }

func toConnection(c db.GitConnection) api.GitConnection {
	return api.GitConnection{Id: c.ID, Kind: api.GitConnectionKind(c.Kind), Name: c.Name, BaseUrl: c.BaseUrl,
		Username: c.Username, UseGithubModule: c.UseGithubModule == 1, HasToken: c.TokenEnc != "" || c.UseGithubModule == 1,
		CreatedAt: c.CreatedAt, LastCheckedAt: c.LastCheckedAt, LastError: c.LastError, WebhookPath: webhookPath(c.ID)}
}

func (m *Module) connection(ctx context.Context, id int64) (db.GitConnection, error) {
	c, err := m.q.GetConnection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return c, httpx.ErrNotFound
	}
	return c, err
}

// client builds the API client of a connection with its token.
func (m *Module) client(ctx context.Context, c db.GitConnection) (*gitAPI, error) {
	g := &gitAPI{kind: c.Kind, base: apiBase(c.Kind, c.BaseUrl), hc: m.hc}
	if c.UseGithubModule == 1 {
		creds, ok := module.Lookup[contracts.GitHubCredentials](m.d.Registry, contracts.GitHubCredentialsKey)
		if !ok {
			return nil, httpx.NewError(http.StatusNotImplemented, "feature_unavailable", "GitHub 模块没有启用")
		}
		base, token, err := creds.Credentials(ctx)
		if err != nil {
			return nil, err
		}
		g.base, g.token = strings.TrimRight(base, "/"), token
		return g, nil
	}
	if c.TokenEnc == "" {
		return nil, httpx.Invalid("这个连接没有令牌")
	}
	token, err := m.d.Secrets.Open(c.TokenEnc)
	if err != nil {
		return nil, err
	}
	g.token = token
	return g, nil
}

// check asks the service who the token belongs to and stores the answer.
func (m *Module) check(ctx context.Context, c db.GitConnection) (db.GitConnection, error) {
	username, errText := "", ""
	g, err := m.client(ctx, c)
	if err == nil {
		username, err = g.user(ctx)
	}
	if err != nil {
		errText = err.Error()
		var he *httpx.Error
		if errors.As(err, &he) {
			errText = he.Message
		}
	}
	now := m.now()
	if err := m.q.SetConnectionCheck(ctx, db.SetConnectionCheckParams{Username: username, LastCheckedAt: &now, LastError: errText, ID: c.ID}); err != nil {
		return c, err
	}
	return m.connection(ctx, c.ID)
}

func (m *Module) ListGitConnections(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListConnections(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := make([]api.GitConnection, 0, len(rows))
	for _, c := range rows {
		out = append(out, toConnection(c))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateGitConnection(w http.ResponseWriter, r *http.Request) {
	var body api.CreateGitConnectionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, secret, err := m.createConnection(r.Context(), body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("git_connection.changed", toConnection(c))
	httpx.JSON(w, http.StatusCreated, map[string]any{"connection": toConnection(c), "webhookSecret": secret})
}

func (m *Module) createConnection(ctx context.Context, body api.CreateGitConnectionJSONRequestBody) (c db.GitConnection, secret string, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "git_connection.create", body.Name, map[string]any{"kind": body.Kind, "id": c.ID}, err)
	}()
	if err := auth.RequireStrictElevated(ctx); err != nil {
		return c, "", err
	}
	kind := string(body.Kind)
	if kind != kindGitHub && kind != kindForgejo {
		return c, "", httpx.Invalid("类型只能是 github 或 forgejo")
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > 60 {
		return c, "", httpx.Invalid("名称要 1 到 60 个字")
	}
	base := ""
	if body.BaseUrl != nil {
		base = *body.BaseUrl
	}
	if base, err = normalizeBaseURL(kind, base); err != nil {
		return c, "", err
	}
	useModule := body.UseGithubModule != nil && *body.UseGithubModule
	if useModule && kind != kindGitHub {
		return c, "", httpx.Invalid("只有 GitHub 连接能用 GitHub 模块的令牌")
	}
	tokenEnc := ""
	if !useModule {
		token := ""
		if body.Token != nil {
			token = strings.TrimSpace(*body.Token)
		}
		if token == "" {
			return c, "", httpx.Invalid("请填写访问令牌")
		}
		if tokenEnc, err = m.d.Secrets.Seal(token); err != nil {
			return c, "", err
		}
	}
	secret = secrets.RandomToken(24)
	secretEnc, err := m.d.Secrets.Seal(secret)
	if err != nil {
		return c, "", err
	}
	c, err = m.q.CreateConnection(ctx, db.CreateConnectionParams{Kind: kind, Name: name, BaseUrl: base, TokenEnc: tokenEnc,
		UseGithubModule: boolInt(useModule), WebhookSecretEnc: secretEnc, CreatedAt: m.now()})
	if err != nil {
		return c, "", err
	}
	c, err = m.check(ctx, c)
	return c, secret, err
}

func (m *Module) UpdateGitConnection(w http.ResponseWriter, r *http.Request, id int64) {
	var body api.UpdateGitConnectionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.updateConnection(r.Context(), id, body)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("git_connection.changed", toConnection(c))
	httpx.JSON(w, http.StatusOK, toConnection(c))
}

func (m *Module) updateConnection(ctx context.Context, id int64, body api.UpdateGitConnectionJSONRequestBody) (c db.GitConnection, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "git_connection.update", strconv.FormatInt(id, 10), map[string]any{"token": body.Token != nil}, err)
	}()
	check := auth.RequireElevated
	if body.Token != nil {
		check = auth.RequireStrictElevated
	}
	if err := check(ctx); err != nil {
		return c, err
	}
	if c, err = m.connection(ctx, id); err != nil {
		return c, err
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len([]rune(name)) > 60 {
			return c, httpx.Invalid("名称要 1 到 60 个字")
		}
		if err := m.q.RenameConnection(ctx, db.RenameConnectionParams{Name: name, ID: id}); err != nil {
			return c, err
		}
	}
	if body.Token != nil {
		if c.UseGithubModule == 1 {
			return c, httpx.Invalid("这个连接用的是 GitHub 模块的令牌，去 GitHub 设置里改")
		}
		token := strings.TrimSpace(*body.Token)
		if token == "" {
			return c, httpx.Invalid("令牌不能为空")
		}
		enc, err := m.d.Secrets.Seal(token)
		if err != nil {
			return c, err
		}
		if err := m.q.SetConnectionToken(ctx, db.SetConnectionTokenParams{TokenEnc: enc, ID: id}); err != nil {
			return c, err
		}
	}
	if c, err = m.connection(ctx, id); err != nil {
		return c, err
	}
	if body.Token != nil {
		return m.check(ctx, c)
	}
	return c, nil
}

func (m *Module) DeleteGitConnection(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	err := func() (err error) {
		defer func() { m.d.Audit.Record(ctx, "git_connection.delete", strconv.FormatInt(id, 10), nil, err) }()
		if err := auth.RequireElevated(ctx); err != nil {
			return err
		}
		tx, err := m.d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		q := m.q.WithTx(tx)
		if err := q.DetachConnectionRepos(ctx, &id); err != nil {
			return err
		}
		n, err := q.DeleteConnection(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return httpx.ErrNotFound
		}
		return tx.Commit()
	}()
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("git_connection.changed", map[string]any{"id": id, "deleted": true})
	httpx.NoContent(w)
}

func (m *Module) CheckGitConnection(w http.ResponseWriter, r *http.Request, id int64) {
	c, err := m.connection(r.Context(), id)
	if err == nil {
		c, err = m.check(r.Context(), c)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toConnection(c))
}

func (m *Module) ListRemoteRepos(w http.ResponseWriter, r *http.Request, id int64, params api.ListRemoteReposParams) {
	ctx := r.Context()
	c, err := m.connection(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	g, err := m.client(ctx, c)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	q := ""
	if params.Q != nil {
		q = *params.Q
	}
	repos, err := g.repos(ctx, q)
	if err != nil {
		httpx.Fail(w, r, gitFailure(err))
		return
	}
	httpx.JSON(w, http.StatusOK, repos)
}

func (m *Module) GetGitWebhook(w http.ResponseWriter, r *http.Request, id int64) {
	ctx := r.Context()
	if err := auth.RequireStrictElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.connection(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	secret, err := m.d.Secrets.Open(c.WebhookSecretEnc)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"path": webhookPath(id), "secret": secret})
}

// gitFailure turns an answer of the Git service into an API error the user
// can read: 502 with the service's message.
func gitFailure(err error) error {
	var ge *gitError
	if errors.As(err, &ge) {
		return httpx.NewError(http.StatusBadGateway, "git_service_error", ge.Error())
	}
	return err
}

// CloneAuth implements contracts.GitConnections.
func (m *Module) CloneAuth(ctx context.Context, id int64) (string, string, error) {
	c, err := m.connection(ctx, id)
	if err != nil {
		return "", "", err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return "", "", err
	}
	user := c.Username
	if user == "" || c.Kind == kindGitHub {
		// GitHub accepts any user name with a token.
		user = "x-access-token"
	}
	return user, g.token, nil
}

// CreatePR implements contracts.GitConnections.
func (m *Module) CreatePR(ctx context.Context, id int64, in contracts.CreatePR) (string, int, error) {
	c, err := m.connection(ctx, id)
	if err != nil {
		return "", 0, err
	}
	g, err := m.client(ctx, c)
	if err != nil {
		return "", 0, err
	}
	url, number, err := g.createPR(ctx, in)
	return url, number, gitFailure(err)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
