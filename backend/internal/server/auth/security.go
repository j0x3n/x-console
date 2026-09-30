package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/pquerna/otp/totp"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const pendingTOTPKey = "auth.pending_totp"

func (s *Service) TOTPEnabled(ctx context.Context) (bool, error) {
	sess := FromContext(ctx)
	if sess == nil {
		return false, httpx.ErrUnauthorized
	}
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return false, err
	}
	return u.TotpEnabled == 1, nil
}

// SkipSetupTOTP finishes setup without two-step login. It is a public
// endpoint, so it asks for the password set a moment ago.
func (s *Service) SkipSetupTOTP(ctx context.Context, w http.ResponseWriter, r *http.Request, password string) error {
	ip := clientIP(r)
	if !s.fails.allowed(ip) {
		return httpx.ErrTooManyRequests
	}
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
	valid, err := CheckPassword(u.PasswordHash, password)
	if err != nil {
		return err
	}
	if !valid {
		s.fails.fail(ip)
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "密码不正确")
	}
	n, err := s.q.FinishSetupWithoutTOTP(ctx, u.ID)
	if err != nil {
		return err
	}
	if n != 1 {
		return httpx.NewError(http.StatusConflict, "already_setup", "已经初始化过了")
	}
	s.audit.Record(ctx, "auth.setup", u.Username, map[string]any{"totpEnabled": false}, nil)
	return s.startSession(ctx, w, r, u)
}

func (s *Service) EnrollTOTP(ctx context.Context) (string, string, error) {
	if err := RequireElevated(ctx); err != nil {
		return "", "", err
	}
	sess := FromContext(ctx)
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return "", "", err
	}
	if u.TotpEnabled == 1 {
		return "", "", httpx.NewError(http.StatusConflict, "totp_enabled", "两步验证已经开启")
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: u.Username})
	if err != nil {
		return "", "", err
	}
	if err := s.settings.SetSecret(ctx, pendingTOTPKey, key.Secret()); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, code string) error {
	sess := FromContext(ctx)
	if sess == nil {
		return httpx.ErrUnauthorized
	}
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return err
	}
	if u.TotpEnabled == 1 {
		return httpx.NewError(http.StatusConflict, "totp_enabled", "两步验证已经开启")
	}
	var secret string
	if err := s.settings.Get(ctx, pendingTOTPKey, &secret); err != nil {
		if errors.Is(err, settings.ErrNotSet) {
			return httpx.NewError(http.StatusConflict, "enrollment_missing", "请先生成两步验证密钥")
		}
		return err
	}
	if !totp.Validate(strings.TrimSpace(code), secret) {
		return httpx.NewError(http.StatusUnauthorized, "invalid_code", "验证码不正确")
	}
	sealed, err := s.box.Seal(secret)
	if err != nil {
		return err
	}
	n, err := s.q.ConfirmPendingTOTP(ctx, db.ConfirmPendingTOTPParams{TotpSecret: sealed, ID: u.ID})
	if err != nil {
		return err
	}
	if n != 1 {
		return httpx.NewError(http.StatusConflict, "totp_enabled", "两步验证已经开启")
	}
	if err := s.settings.Delete(ctx, pendingTOTPKey); err != nil {
		return err
	}
	s.audit.Record(ctx, "auth.totp.enable", u.Username, nil, nil)
	return nil
}

func (s *Service) DisableTOTP(ctx context.Context, password, code string) error {
	sess := FromContext(ctx)
	if sess == nil {
		return httpx.ErrUnauthorized
	}
	if !s.fails.allowed("totp:disable:" + sess.ID) {
		return httpx.ErrTooManyRequests
	}
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return err
	}
	if u.TotpEnabled != 1 {
		return httpx.NewError(http.StatusConflict, "totp_disabled", "两步验证已经关闭")
	}
	valid, err := CheckPassword(u.PasswordHash, password)
	if err != nil {
		return err
	}
	if !valid || !s.checkTOTP(u, code) {
		s.fails.fail("totp:disable:" + sess.ID)
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "密码或验证码不正确")
	}
	if err := s.q.DisableTOTP(ctx, u.ID); err != nil {
		return err
	}
	s.fails.reset("totp:disable:" + sess.ID)
	s.audit.Record(ctx, "auth.totp.disable", u.Username, nil, nil)
	return nil
}

func (s *Service) ChangePassword(ctx context.Context, oldPassword, newPassword string) error {
	sess := FromContext(ctx)
	if sess == nil {
		return httpx.ErrUnauthorized
	}
	if !s.fails.allowed("password:" + sess.ID) {
		return httpx.ErrTooManyRequests
	}
	if len(newPassword) < 10 {
		return httpx.Invalid("新密码至少 10 位")
	}
	u, err := s.q.GetUser(ctx, sess.UserID)
	if err != nil {
		return err
	}
	valid, err := CheckPassword(u.PasswordHash, oldPassword)
	if err != nil {
		return err
	}
	if !valid {
		s.fails.fail("password:" + sess.ID)
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "旧密码不正确")
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.q.UpdatePassword(ctx, db.UpdatePasswordParams{PasswordHash: hash, ID: u.ID}); err != nil {
		return err
	}
	if err := s.q.DeleteOtherSessions(ctx, db.DeleteOtherSessionsParams{UserID: u.ID, ID: sess.ID}); err != nil {
		return err
	}
	s.fails.reset("password:" + sess.ID)
	s.audit.Record(ctx, "auth.password.change", u.Username, nil, nil)
	return nil
}
