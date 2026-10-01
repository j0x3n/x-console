package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

// B43 API 令牌：外部 AI 通过 MCP 接口调用动作时用。
// 令牌只对 TokenPath 有效，永远不是提升权限状态，也不经过 CSRF 检查。

// TokenPath is the only API path that accepts API tokens.
const TokenPath = "/api/v1/mcp"

// Token access levels.
const (
	AccessRead        = "read"
	AccessWrite       = "write"
	AccessWriteDelete = "write_delete"
)

// tokenPrefix starts every API token, so it is easy to recognise in logs
// and secret scanners.
const tokenPrefix = "xc_"

// tokenCallsPerMinute limits each token.
const tokenCallsPerMinute = 60

// TokenInfo is the API token behind a request.
type TokenInfo struct {
	ID      int64
	Name    string
	Access  string
	Modules []string
}

// TokenFrom returns the API token of the request, or nil for a cookie session.
func TokenFrom(ctx context.Context) *TokenInfo {
	if s := FromContext(ctx); s != nil {
		return s.Token
	}
	return nil
}

func validAccess(a string) bool {
	return a == AccessRead || a == AccessWrite || a == AccessWriteDelete
}

// NewToken is a created token. Secret is shown once.
type NewToken struct {
	Row    db.ApiToken
	Secret string
}

// CreateToken makes a token. It always asks for a fresh verification (B48).
func (s *Service) CreateToken(ctx context.Context, name, access string, modules []string, expiresAt *time.Time) (out NewToken, err error) {
	defer func() {
		s.audit.Record(ctx, "auth.token.create", name, map[string]any{"access": access, "modules": modules}, err)
	}()
	if err := RequireStrictElevated(ctx); err != nil {
		return out, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 60 {
		return out, httpx.Invalid("令牌名称要 1 到 60 个字")
	}
	if !validAccess(access) {
		return out, httpx.Invalid("权限只能是 read、write 或 write_delete")
	}
	if expiresAt != nil && !expiresAt.After(s.now()) {
		return out, httpx.Invalid("过期时间要在以后")
	}
	if modules == nil {
		modules = []string{}
	}
	raw, _ := json.Marshal(modules)
	secret := tokenPrefix + secrets.RandomToken(32)
	row, err := s.q.CreateAPIToken(ctx, db.CreateAPITokenParams{
		Name: name, Prefix: secret[:8], TokenHash: secrets.Hash(secret), Access: access, Modules: string(raw),
		ExpiresAt: expiresAt, CreatedAt: s.now(),
	})
	if err != nil {
		return out, err
	}
	return NewToken{Row: row, Secret: secret}, nil
}

// ListTokens returns every token, revoked ones last.
func (s *Service) ListTokens(ctx context.Context) ([]db.ApiToken, error) {
	if FromContext(ctx) == nil || TokenFrom(ctx) != nil {
		return nil, httpx.ErrUnauthorized
	}
	return s.q.ListAPITokens(ctx)
}

// RevokeToken stops a token at once.
func (s *Service) RevokeToken(ctx context.Context, id int64) (err error) {
	defer func() { s.audit.Record(ctx, "auth.token.revoke", "", map[string]any{"id": id}, err) }()
	if err := RequireElevated(ctx); err != nil {
		return err
	}
	now := s.now()
	n, err := s.q.RevokeAPIToken(ctx, db.RevokeAPITokenParams{RevokedAt: &now, ID: id})
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	return nil
}

// bearer returns the token of an "Authorization: Bearer" header, or "".
func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

var errBadToken = httpx.NewError(http.StatusUnauthorized, "invalid_token", "令牌无效、已吊销或已过期")

// authenticateToken turns a bearer token into a session. The session is
// never elevated and its audit actor is "token:<name>".
func (s *Service) authenticateToken(r *http.Request, secret string) (*Session, error) {
	ip := clientIP(r)
	if !s.tokenFails.allowed(ip) {
		return nil, httpx.ErrTooManyRequests
	}
	row, err := s.q.GetAPITokenByHash(r.Context(), secrets.Hash(secret))
	if errors.Is(err, sql.ErrNoRows) || err == nil && (row.RevokedAt != nil || row.ExpiresAt != nil && !row.ExpiresAt.After(s.now())) {
		s.tokenFails.fail(ip)
		return nil, errBadToken
	}
	if err != nil {
		return nil, err
	}
	key := "token:" + row.Prefix
	if !s.tokenRate.allowed(key) {
		return nil, httpx.ErrTooManyRequests
	}
	s.tokenRate.fail(key) // counts the call
	// Remember the last use, at most once a minute.
	if row.LastUsedAt == nil || s.now().Sub(*row.LastUsedAt) > time.Minute || row.LastUsedIp != ip {
		now := s.now()
		_ = s.q.TouchAPIToken(r.Context(), db.TouchAPITokenParams{LastUsedAt: &now, LastUsedIp: ip, ID: row.ID})
	}
	var modules []string
	_ = json.Unmarshal([]byte(row.Modules), &modules)
	return &Session{
		ID: "token:" + row.Prefix, Username: "token:" + row.Name, ViaToken: true,
		Token: &TokenInfo{ID: row.ID, Name: row.Name, Access: row.Access, Modules: modules},
	}, nil
}
