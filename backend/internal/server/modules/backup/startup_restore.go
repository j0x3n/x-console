package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/events"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/j0x3n/x-console/backend/internal/server/store"
)

func PrepareBeforeRestore(ctx context.Context, cfg config.Config) error {
	dir := cfg.RestoreDir()
	if !pending(dir) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, dbName)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Stat(cfg.DBPath()); err != nil {
		return fmt.Errorf("恢复前的数据库无法读取，已停止恢复：%w", err)
	}
	conn, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer conn.Close()
	m, err := startupRestoreModule(ctx, cfg, conn)
	if err != nil {
		return err
	}
	return m.PreparePending(ctx)
}

func startupRestoreModule(ctx context.Context, cfg config.Config, conn *sql.DB) (*Module, error) {
	box, err := secrets.NewBox(cfg.MasterKey)
	if err != nil {
		return nil, err
	}
	st := settings.New(conn, box)
	var backend string
	if err := st.Get(ctx, "storage.backend", &backend); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return nil, err
	}
	var current files.Store = files.Local{Root: cfg.FilesDir()}
	switch backend {
	case "", "local":
	case "s3":
		c, complete, err := storage.S3Config(ctx, st, slog.Default())
		if err != nil {
			return nil, err
		}
		if !complete {
			return nil, fmt.Errorf("恢复使用的 S3 凭据无法读取，已停止恢复")
		}
		s3, err := files.NewS3(c)
		if err != nil {
			return nil, err
		}
		if err := s3.Check(ctx); err != nil {
			return nil, fmt.Errorf("恢复使用的文件存储无法连接，已停止恢复：%w", err)
		}
		current = s3
	default:
		return nil, fmt.Errorf("恢复使用的文件存储类型不支持：%s", backend)
	}
	if err := os.MkdirAll(cfg.TmpDir(), 0o700); err != nil {
		return nil, err
	}
	m := &Module{d: &module.Deps{Config: cfg, DB: conn, Settings: st, Secrets: box, Files: files.NewManager(current), Log: slog.Default(), Audit: audit.New(conn), Bus: events.NewBus()}, local: files.Local{Root: cfg.BackupsDir()}, now: time.Now}
	return m, nil
}

func FinishBeforeStart(ctx context.Context, cfg config.Config, conn *sql.DB) error {
	if !pending(cfg.RestoreDir()) {
		return nil
	}
	if err := validateStageFiles(filepath.Join(cfg.RestoreDir(), "files")); err != nil {
		return err
	}
	if err := validateStagedSnapshot(ctx, cfg.RestoreDir()); err != nil {
		return err
	}
	if err := os.RemoveAll(cfg.FilesCacheDir()); err != nil {
		return err
	}
	m, err := startupRestoreModule(ctx, cfg, conn)
	if err != nil {
		return err
	}
	return m.FinishPending(ctx)
}

func (m *Module) FinishPending(ctx context.Context) error {
	if !pending(m.d.Config.RestoreDir()) {
		return nil
	}
	kind := api.BackupJobKindRestore
	j := &job{v: api.BackupJob{Kind: &kind, State: api.Running, StartedAt: ptr(m.now().UTC()), Step: ptr("正在恢复文件")}}
	m.mu.Lock()
	m.cur, m.busy = j, true
	m.mu.Unlock()
	err := m.finishStagedRestore(ctx, j)
	m.end(j, publicBackupError(err))
	if err == nil {
		m.d.Bus.Publish("backup.restored", nil)
	}
	return err
}

func EnsureReadyBeforeApp(ctx context.Context, cfg config.Config, conn *sql.DB) error {
	if pending(cfg.RestoreDir()) {
		if _, err := os.Stat(filepath.Join(cfg.RestoreDir(), dbName)); err == nil {
			return fmt.Errorf("恢复数据库尚未应用，请通过服务端启动流程恢复")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return FinishBeforeStart(ctx, cfg, conn)
}
