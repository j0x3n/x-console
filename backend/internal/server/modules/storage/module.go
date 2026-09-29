// Package storage decides where the site keeps its files (B24): a directory
// on this machine or an S3 bucket. It owns the settings, builds the
// files.Store the other modules use, and moves everything from one place to
// the other. See docs/specs/B24-B25.md.
package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Settings keys.
const (
	keyBackend    = "storage.backend"     // "local" or "s3"
	keyS3         = "storage.s3"          // s3Settings, not secret
	keyS3Secret   = "storage.s3_secret"   // encrypted secret access key
	keyCacheLimit = "storage.cache_limit" // bytes
	keyMigration  = "storage.migration"   // last move, api.StorageMigration

	// The old drive-only S3 sync stored its setup here.
	legacyDriveConfig = "drive.s3.config"
	legacyDriveSecret = "drive.s3.secret"
)

const (
	defaultCacheLimit = 1 << 30 // 1 GB
	usageTTL          = 10 * time.Minute
)

// s3Settings is stored under keyS3.
type s3Settings struct {
	Endpoint    string `json:"endpoint"`
	Region      string `json:"region"`
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix"`
	AccessKeyID string `json:"accessKeyId"`
	PathStyle   bool   `json:"pathStyle"`
}

func (s s3Settings) config(secret string) files.S3Config {
	return files.S3Config{Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket, Prefix: s.Prefix,
		AccessKeyID: s.AccessKeyID, SecretAccessKey: secret, PathStyle: s.PathStyle}
}

// complete reports whether every field a connection needs is filled in.
func (s s3Settings) complete(secret string) bool {
	return s.Endpoint != "" && s.Bucket != "" && s.AccessKeyID != "" && secret != ""
}

// Module implements api.ServerInterface.
type Module struct {
	d *module.Deps

	mu      sync.Mutex // guards everything below
	backend api.StorageBackend
	raw     files.Store   // the Store behind the Manager: Local or S3
	cache   *files.Cached // set while the backend is s3
	usage   usageCache
	move    *move // the running move, if any
	last    api.StorageMigration
}

var _ api.ServerInterface = (*Module)(nil)

// New reads the settings and puts the right Store into d.Files.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{d: d, last: api.StorageMigration{State: api.Idle, Target: api.Local}}
	ctx := context.Background()
	if err := m.migrateDriveSettings(ctx); err != nil {
		return nil, fmt.Errorf("storage: copy drive S3 settings: %w", err)
	}
	if err := m.load(ctx); err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "storage" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

func (m *Module) local() files.Local { return files.Local{Root: m.d.Config.FilesDir()} }

// get reads a setting; a missing one leaves dst as it is.
func (m *Module) get(ctx context.Context, key string, dst any) error {
	if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	return nil
}

// migrateDriveSettings copies the drive's old S3 setup, secret included, the
// first time this module starts. The location stays local: nothing moves
// until the user says so.
func (m *Module) migrateDriveSettings(ctx context.Context) error {
	if has, err := m.d.Settings.Has(ctx, keyS3); err != nil || has {
		return err
	}
	var old struct {
		Endpoint    string `json:"endpoint"`
		Region      string `json:"region"`
		Bucket      string `json:"bucket"`
		Prefix      string `json:"prefix"`
		AccessKeyID string `json:"accessKeyId"`
		PathStyle   bool   `json:"pathStyle"`
	}
	if err := m.get(ctx, legacyDriveConfig, &old); err != nil {
		return err
	}
	if old.Endpoint == "" && old.Bucket == "" {
		return nil
	}
	if err := m.d.Settings.Set(ctx, keyS3, s3Settings(old)); err != nil {
		return err
	}
	var secret string
	if err := m.get(ctx, legacyDriveSecret, &secret); err != nil {
		return err
	}
	if secret != "" {
		return m.d.Settings.SetSecret(ctx, keyS3Secret, secret)
	}
	return nil
}

func (m *Module) s3Settings(ctx context.Context) (s3Settings, string, error) {
	var s s3Settings
	var secret string
	if err := m.get(ctx, keyS3, &s); err != nil {
		return s, "", err
	}
	return s, secret, m.get(ctx, keyS3Secret, &secret)
}

func (m *Module) cacheLimit(ctx context.Context) (int64, error) {
	limit := int64(defaultCacheLimit)
	return limit, m.get(ctx, keyCacheLimit, &limit)
}

// load builds the Store the settings ask for and hands it to the Manager.
func (m *Module) load(ctx context.Context) error {
	backend := api.Local
	var stored string
	if err := m.get(ctx, keyBackend, &stored); err != nil {
		return err
	}
	if stored == string(api.S3) {
		backend = api.S3
	}
	if err := m.get(ctx, keyMigration, &m.last); err != nil {
		return err
	}
	if m.last.State == api.Running {
		// The server stopped in the middle of a move.
		m.last.State, m.last.Error = api.Failed, ptr("服务重启了，搬迁中断，请重新开始")
		now := time.Now().UTC()
		m.last.FinishedAt = &now
		if err := m.d.Settings.Set(ctx, keyMigration, m.last); err != nil {
			return err
		}
	}
	raw, cache, err := m.build(ctx, backend)
	if err != nil {
		return err
	}
	m.backend, m.raw, m.cache = backend, raw, cache
	m.d.Files.Swap(m.storeFor(raw, cache))
	return nil
}

// build makes the Store for a backend from the saved settings. For s3 it
// returns the bucket itself and the cache in front of it.
func (m *Module) build(ctx context.Context, backend api.StorageBackend) (raw files.Store, cache *files.Cached, err error) {
	if backend == api.Local {
		return m.local(), nil, nil
	}
	s, secret, err := m.s3Settings(ctx)
	if err != nil {
		return nil, nil, err
	}
	remote, err := files.NewS3(s.config(secret))
	if err != nil {
		return nil, nil, err
	}
	limit, err := m.cacheLimit(ctx)
	if err != nil {
		return nil, nil, err
	}
	cache, err = files.NewCached(remote, m.d.Config.FilesCacheDir(), limit)
	return remote, cache, err
}

// storeFor is the Store the site uses: the cache for s3, the directory for local.
func (m *Module) storeFor(raw files.Store, cache *files.Cached) files.Store {
	if cache != nil {
		return cache
	}
	return raw
}

// clearCache empties the cache directory, for when it may hold another
// bucket's files.
func (m *Module) clearCache() error {
	return os.RemoveAll(m.d.Config.FilesCacheDir())
}

func ptr[T any](v T) *T { return &v }
