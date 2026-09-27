// Package auth implements the single-user login: password + TOTP, session
// cookies, and short "elevated" windows for dangerous operations.
package auth

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	// CookieName is the session cookie.
	CookieName = "xc_session"
	// CSRFHeader must be sent with every mutating cookie-authenticated request.
	CSRFHeader     = "X-Requested-With"
	csrfValue      = "x-console"
	sessionTTL     = 30 * 24 * time.Hour
	elevationTTL   = 5 * time.Minute
	totpIssuer     = "X Console"
	loginFailLimit = 5
)

// Session is the authenticated session stored in the request context.
type Session struct {
	ID            string
	UserID        int64
	Username      string
	ElevatedUntil *time.Time
	VaultUntil    *time.Time
}

// Elevated reports whether dangerous operations are allowed right now.
func (s *Session) Elevated() bool {
	return s.ElevatedUntil != nil && s.ElevatedUntil.After(time.Now())
}

func VaultUnlocked(ctx context.Context) bool {
	s := FromContext(ctx)
	return s != nil && s.VaultUntil != nil && s.VaultUntil.After(time.Now()) && ctx.Value(vaultDisabledKey{}) == nil
}

type vaultDisabledKey struct{}

func WithoutVault(ctx context.Context) context.Context {
	return context.WithValue(ctx, vaultDisabledKey{}, true)
}

type sessionKey struct{}

// FromContext returns the session, or nil for unauthenticated requests.
func FromContext(ctx context.Context) *Session {
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}

// WithSession stores s in ctx. Tests use it to fake a logged-in user.
func WithSession(ctx context.Context, s *Session) context.Context {
	ctx = context.WithValue(ctx, sessionKey{}, s)
	return audit.WithActor(ctx, s.Username)
}

func RequireElevated(ctx context.Context) error {
	s := FromContext(ctx)
	if s == nil {
		return httpx.ErrUnauthorized
	}
	if !s.Elevated() {
		return httpx.ErrElevationRequired
	}
	return nil
}

// Service owns users and sessions.
type Service struct {
	conn       *sql.DB
	q          *db.Queries
	settings   *settings.Store
	box        *secrets.Box
	audit      *audit.Log
	secure     bool
	fails      *limiter
	vaultFails *limiter
	now        func() time.Time
}

// NewService builds the auth service. secureCookies should be false only in dev.
func NewService(conn *sql.DB, box *secrets.Box, log *audit.Log, secureCookies bool) *Service {
	return &Service{conn: conn, q: db.New(conn), settings: settings.New(conn, box), box: box, audit: log, secure: secureCookies,
		fails: newLimiter(loginFailLimit, 15*time.Minute), vaultFails: newLimiter(loginFailLimit, 15*time.Minute),
		now: func() time.Time { return time.Now().UTC() }}
}

// SetupRequired is true until the first user exists and has TOTP enabled.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	u, err := s.q.GetFirstUser(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return u.SetupCompleted == 0, nil
}

// Setup creates the user (or replaces an unconfirmed one) and returns the TOTP enrollment.
func (s *Service) Setup(ctx context.Context, username, password string) (secret, url string, err error) {
	required, err := s.SetupRequired(ctx)
	if err != nil {
		return "", "", err
	}
	if !required {
		return "", "", httpx.NewError(http.StatusConflict, "already_setup", "已经初始化过了")
	}
	if len(password) < 10 {
		return "", "", httpx.Invalid("密码至少 10 位")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: username})
	if err != nil {
		return "", "", err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", "", err
	}
	sealed, err := s.box.Seal(key.Secret())
	if err != nil {
		return "", "", err
	}
	// Only one user exists. An unconfirmed earlier attempt is replaced.
	if _, err := s.q.GetFirstUser(ctx); err == nil {
		if err := s.q.DeleteAllUsers(ctx); err != nil {
			return "", "", err
		}
	}
	if _, err := s.q.CreateUser(ctx, db.CreateUserParams{Username: username, PasswordHash: hash, TotpSecret: sealed, CreatedAt: s.now()}); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ConfirmSetup verifies the first TOTP code, enables TOTP and starts a session.
func (s *Service) ConfirmSetup(ctx context.Context, w http.ResponseWriter, r *http.Request, code string) error {
	u, err := s.q.GetFirstUser(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.NewError(http.StatusConflict, "setup_not_started", "请先设置用户名和密码")
	}
	if err != nil {
		return err
	}
	if u.SetupCompleted == 1 {
		return httpx.NewError(http.StatusConflict, "already_setup", "已经初始化过了")
	}
	if u.TotpSecret == "" {
		return httpx.NewError(http.StatusConflict, "already_setup", "请先完成初始化")
	}
	if !s.checkTOTP(u, code) {
		return httpx.NewError(http.StatusUnauthorized, "invalid_code", "验证码不正确")
	}
	n, err := s.q.EnableTOTP(ctx, u.ID)
	if err != nil {
		return err
	}
	if n != 1 {
		return httpx.NewError(http.StatusConflict, "already_setup", "已经初始化过了")
	}
	s.audit.Record(audit.WithActor(ctx, u.Username), "auth.setup", u.Username, nil, nil)
	return s.startSession(ctx, w, r, u)
}

// Login checks password and TOTP and sets the session cookie.
func (s *Service) Login(ctx context.Context, w http.ResponseWriter, r *http.Request, username, password, code string) error {
	ip := clientIP(r)
	if !s.fails.allowed(ip) {
		return httpx.ErrTooManyRequests
	}
	u, err := s.q.GetUserByUsername(ctx, username)
	ok := err == nil && u.SetupCompleted == 1
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if ok {
		ok, err = CheckPassword(u.PasswordHash, password)
		if err != nil {
			return err
		}
	}
	if ok && u.TotpEnabled == 1 && code == "" {
		// Password is right but no code yet: the login page asks for it next.
		// Not counted as a failure.
		return httpx.NewError(http.StatusUnauthorized, "totp_required", "请输入两步验证码")
	}
	if ok && u.TotpEnabled == 1 {
		ok = s.checkTOTP(u, code)
	}
	if !ok {
		s.fails.fail(ip)
		s.audit.Record(audit.WithActor(ctx, username), "auth.login", ip, nil, errors.New("invalid credentials"))
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "用户名、密码或验证码不正确")
	}
	s.fails.reset(ip)
	s.audit.Record(audit.WithActor(ctx, u.Username), "auth.login", ip, nil, nil)
	return s.startSession(ctx, w, r, u)
}

// Logout deletes the current session and clears the cookie.
func (s *Service) Logout(ctx context.Context, w http.ResponseWriter) error {
	if sess := FromContext(ctx); sess != nil {
		if err := s.q.DeleteSession(ctx, sess.ID); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode})
	return nil
}

// Elevate checks a TOTP code and opens a 5 minute elevated window.
func (s *Service) Elevate(ctx context.Context, code, password string) (time.Time, error) {
	sess := FromContext(ctx)
	if sess == nil {
		return time.Time{}, httpx.ErrUnauthorized
	}
	if !s.fails.allowed("elevate:" + sess.ID) {
		return time.Time{}, httpx.ErrTooManyRequests
	}
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return time.Time{}, err
	}
	valid := false
	if u.TotpEnabled == 1 {
		valid = s.checkTOTP(u, code)
	} else {
		valid, err = CheckPassword(u.PasswordHash, password)
		if err != nil {
			return time.Time{}, err
		}
	}
	if !valid {
		s.fails.fail("elevate:" + sess.ID)
		return time.Time{}, httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "验证信息不正确")
	}
	s.fails.reset("elevate:" + sess.ID)
	until := s.now().Add(elevationTTL)
	if err := s.q.ElevateSession(ctx, db.ElevateSessionParams{ElevatedUntil: &until, ID: sess.ID}); err != nil {
		return time.Time{}, err
	}
	s.audit.Record(ctx, "auth.elevate", "", nil, nil)
	return until, nil
}

// Authenticate resolves the session cookie. It returns nil without error when
// there is no valid session.
func (s *Service) Authenticate(r *http.Request) (*Session, error) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	id := secrets.Hash(c.Value)
	row, err := s.q.GetSession(r.Context(), db.GetSessionParams{ID: id, ExpiresAt: s.now()})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Sliding expiry, refreshed at most once a day.
	if row.ExpiresAt.Sub(s.now()) < sessionTTL-24*time.Hour {
		_ = s.q.TouchSession(r.Context(), db.TouchSessionParams{ExpiresAt: s.now().Add(sessionTTL), ID: id})
	}
	return &Session{ID: id, UserID: row.UserID, Username: row.Username, ElevatedUntil: row.ElevatedUntil, VaultUntil: row.VaultUntil}, nil
}

// Middleware attaches the session and rejects unauthenticated requests,
// except for paths where public(path) is true. Mutating requests must carry
// the CSRF header.
func (s *Service) Middleware(public func(path string) bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, err := s.Authenticate(r)
			if err != nil {
				httpx.Fail(w, r, err)
				return
			}
			if sess != nil {
				r = r.WithContext(WithSession(r.Context(), sess))
			}
			if public(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if sess == nil {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			if isMutating(r.Method) && r.Header.Get(CSRFHeader) != csrfValue {
				httpx.Fail(w, r, httpx.NewError(http.StatusForbidden, "csrf", "缺少 X-Requested-With 请求头"))
				return
			}
			if VaultUnlocked(r.Context()) {
				until := s.now().Add(vaultTTL)
				if err := s.q.SetVaultUntil(r.Context(), db.SetVaultUntilParams{VaultUntil: &until, ID: sess.ID}); err != nil {
					httpx.Fail(w, r, err)
					return
				}
				sess.VaultUntil = &until
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CleanupExpired deletes expired sessions. The scheduler calls it hourly.
func (s *Service) CleanupExpired(ctx context.Context) error {
	return s.q.DeleteExpiredSessions(ctx, s.now())
}

func (s *Service) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, u db.User) error {
	token := secrets.RandomToken(32)
	now := s.now()
	if err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID: secrets.Hash(token), UserID: u.ID, CreatedAt: now, ExpiresAt: now.Add(sessionTTL),
		UserAgent: r.UserAgent(), Ip: clientIP(r),
	}); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: CookieName, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode})
	return nil
}

func (s *Service) checkTOTP(u db.User, code string) bool {
	secret, err := s.box.Open(u.TotpSecret)
	if err != nil {
		return false
	}
	return totp.Validate(strings.TrimSpace(code), secret)
}

func isMutating(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func clientIP(r *http.Request) string {
	// Caddy sets X-Forwarded-For; the server only listens on localhost behind it.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
