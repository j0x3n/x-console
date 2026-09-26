// Package settings is a typed key/value store for module configuration.
// Keys are "<module>.<name>", for example "ha.url". Values are JSON.
// Secret values (tokens, passwords) are encrypted with secrets.Box.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core/db"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

// ErrNotSet is returned when a key has no value.
var ErrNotSet = errors.New("settings: not set")

// Store reads and writes settings.
type Store struct {
	q   *db.Queries
	box *secrets.Box
}

// New builds a Store.
func New(conn *sql.DB, box *secrets.Box) *Store {
	return &Store{q: db.New(conn), box: box}
}

// Get decodes the value of key into v. Returns ErrNotSet when missing.
func (s *Store) Get(ctx context.Context, key string, v any) error {
	row, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotSet
	}
	if err != nil {
		return err
	}
	raw := row.Value
	if row.Encrypted == 1 {
		if raw, err = s.box.Open(raw); err != nil {
			return err
		}
	}
	return json.Unmarshal([]byte(raw), v)
}

// Set stores v as plain JSON.
func (s *Store) Set(ctx context.Context, key string, v any) error {
	return s.put(ctx, key, v, false)
}

// SetSecret stores v encrypted. Use it for tokens and passwords.
func (s *Store) SetSecret(ctx context.Context, key string, v any) error {
	return s.put(ctx, key, v, true)
}

// Delete removes key.
func (s *Store) Delete(ctx context.Context, key string) error {
	return s.q.DeleteSetting(ctx, key)
}

// Has reports whether key has a value.
func (s *Store) Has(ctx context.Context, key string) (bool, error) {
	_, err := s.q.GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) put(ctx context.Context, key string, v any, encrypt bool) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	value := string(raw)
	var flag int64
	if encrypt {
		if value, err = s.box.Seal(value); err != nil {
			return err
		}
		flag = 1
	}
	return s.q.UpsertSetting(ctx, db.UpsertSettingParams{Key: key, Value: value, Encrypted: flag, UpdatedAt: time.Now().UTC()})
}
