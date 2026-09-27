package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	vaultPasswordKey = "vault.password_hash"
	vaultTTL         = 15 * time.Minute
)

func (s *Service) VaultStatus(ctx context.Context) (bool, *time.Time, error) {
	configured, err := s.settings.Has(ctx, vaultPasswordKey)
	if err != nil {
		return false, nil, err
	}
	if VaultUnlocked(ctx) {
		return configured, FromContext(ctx).VaultUntil, nil
	}
	return configured, nil, nil
}

func (s *Service) SetupVault(ctx context.Context, password string) (*time.Time, error) {
	sess := FromContext(ctx)
	if sess == nil {
		return nil, httpx.ErrUnauthorized
	}
	if len(password) < 6 {
		return nil, httpx.Invalid("隐藏密码至少 6 位")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	value, err := json.Marshal(hash)
	if err != nil {
		return nil, err
	}
	n, err := s.q.InsertVaultPassword(ctx, db.InsertVaultPasswordParams{Value: string(value), UpdatedAt: s.now()})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, httpx.NewError(http.StatusConflict, "already_setup", "隐藏密码已经设置")
	}
	until, err := s.setVaultUntil(ctx, sess)
	if err != nil {
		return nil, err
	}
	s.audit.Record(ctx, "vault.setup", sess.ID, nil, nil)
	return &until, nil
}

func (s *Service) UnlockVault(ctx context.Context, r *http.Request, password string) (*time.Time, error) {
	sess := FromContext(ctx)
	if sess == nil {
		return nil, httpx.ErrUnauthorized
	}
	key := "vault:" + clientIP(r)
	if !s.vaultFails.allowed(key) {
		return nil, httpx.ErrTooManyRequests
	}
	var hash string
	if err := s.settings.Get(ctx, vaultPasswordKey, &hash); err != nil {
		if errors.Is(err, settings.ErrNotSet) {
			return nil, httpx.NewError(http.StatusConflict, "vault_not_configured", "请先设置隐藏密码")
		}
		return nil, err
	}
	valid, err := CheckPassword(hash, password)
	if err != nil {
		return nil, err
	}
	if !valid {
		s.vaultFails.fail(key)
		s.audit.Record(ctx, "vault.unlock", sess.ID, nil, errors.New("invalid credentials"))
		return nil, httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "隐藏密码不正确")
	}
	until, err := s.setVaultUntil(ctx, sess)
	if err != nil {
		return nil, err
	}
	s.vaultFails.reset(key)
	s.audit.Record(ctx, "vault.unlock", sess.ID, nil, nil)
	return &until, nil
}

func (s *Service) LockVault(ctx context.Context) error {
	sess := FromContext(ctx)
	if sess == nil {
		return httpx.ErrUnauthorized
	}
	if err := s.q.SetVaultUntil(ctx, db.SetVaultUntilParams{ID: sess.ID}); err != nil {
		return err
	}
	sess.VaultUntil = nil
	s.audit.Record(ctx, "vault.lock", sess.ID, nil, nil)
	return nil
}

func (s *Service) ChangeVaultPassword(ctx context.Context, oldPassword, newPassword string) error {
	sess := FromContext(ctx)
	if sess == nil {
		return httpx.ErrUnauthorized
	}
	key := "vault:" + sess.ID
	if !s.vaultFails.allowed(key) {
		return httpx.ErrTooManyRequests
	}
	if len(newPassword) < 6 {
		return httpx.Invalid("新隐藏密码至少 6 位")
	}
	var hash string
	if err := s.settings.Get(ctx, vaultPasswordKey, &hash); err != nil {
		if errors.Is(err, settings.ErrNotSet) {
			return httpx.NewError(http.StatusConflict, "vault_not_configured", "请先设置隐藏密码")
		}
		return err
	}
	valid, err := CheckPassword(hash, oldPassword)
	if err != nil {
		return err
	}
	if !valid {
		s.vaultFails.fail(key)
		s.audit.Record(ctx, "vault.password.change", sess.ID, nil, errors.New("invalid credentials"))
		return httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "旧隐藏密码不正确")
	}
	if err := s.resetVaultPassword(ctx, newPassword); err != nil {
		return err
	}
	sess.VaultUntil = nil
	s.vaultFails.reset(key)
	s.audit.Record(ctx, "vault.password.change", sess.ID, nil, nil)
	return nil
}

func (s *Service) setVaultUntil(ctx context.Context, sess *Session) (time.Time, error) {
	until := s.now().Add(vaultTTL)
	if err := s.q.SetVaultUntil(ctx, db.SetVaultUntilParams{VaultUntil: &until, ID: sess.ID}); err != nil {
		return time.Time{}, err
	}
	sess.VaultUntil = &until
	return until, nil
}

func ResetVaultPassword(ctx context.Context, conn *sql.DB, password string) error {
	return resetVaultPassword(ctx, conn, password, time.Now().UTC())
}

func (s *Service) resetVaultPassword(ctx context.Context, password string) error {
	return resetVaultPassword(ctx, s.conn, password, s.now())
}

func resetVaultPassword(ctx context.Context, conn *sql.DB, password string, now time.Time) error {
	if len(password) < 6 {
		return httpx.Invalid("隐藏密码至少 6 位")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	value, err := json.Marshal(hash)
	if err != nil {
		return err
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := db.New(tx)
	if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: vaultPasswordKey, Value: string(value), UpdatedAt: now}); err != nil {
		return err
	}
	if err := q.ClearVaultSessions(ctx); err != nil {
		return err
	}
	return tx.Commit()
}
