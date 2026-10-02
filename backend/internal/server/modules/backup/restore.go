package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/secrets"
)

// Files inside the restore folder. readyName is written last, so a folder
// without it is an unpack that never finished and is ignored.
const (
	readyName = "ready"
	noteName  = "restore.json"
)

// restoreNote remembers where a staged restore came from, for the audit log
// written after the restart.
type restoreNote struct {
	Source string `json:"source"`
	Actor  string `json:"actor"`
}

// pending reports whether a staged restore waits in dir.
func pending(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, readyName))
	return err == nil
}

// stage unpacks a package into dir and checks it. dir is emptied first. The
// database is verified against the manifest before anything is marked ready.
func stage(ctx context.Context, src io.Reader, dir, currentMigration string, note restoreNote) (manifest, error) {
	return stagePackage(ctx, src, dir, currentMigration, note)
}

// unpackDB writes the database out and checks its checksum.
func unpackDB(r io.Reader, path, want string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("解包数据库失败：%w", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("备份里的数据库校验没通过，文件可能已损坏")
	}
	return nil
}

// ApplyPending puts the database of a staged restore in place. The server
// calls it at start, before the database is opened. It reports whether a
// restore is waiting, whose files the module copies once the server is up.
func ApplyPending(dataDir string) (bool, error) { return applyPreparedDatabase(dataDir) }

// restoreFolder is the folder name of Config.RestoreDir.
const restoreFolder = "restore-tmp"

var (
	errNewer = httpx.Invalid("备份来自更新的版本，请先升级")
)

// RestoreBackup is POST /backups/{backupId}/restore.
func (m *Module) RestoreBackup(w http.ResponseWriter, r *http.Request, id api.BackupId) {
	ctx := r.Context()
	// Restoring overwrites everything: always ask, whatever the mode (B48).
	if err := auth.RequireStrictElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.RestoreInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.Confirm != "恢复" {
		httpx.Fail(w, r, httpx.Invalid("请输入“恢复”确认"))
		return
	}
	e, err := m.find(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	man, err := m.peek(ctx, e)
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid(err.Error()))
		return
	}
	current, err := latestMigration(ctx, m.d.DB)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if newerMigration(man.Migration, current) {
		httpx.Fail(w, r, errNewer)
		return
	}
	j, err := m.begin(api.BackupJobKindRestore)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		if err := m.stageRestore(bg, j, e, current); err != nil {
			m.d.Audit.Record(bg, "backup.restore", id, nil, err)
			m.end(j, err)
		}
	}()
	httpx.JSON(w, http.StatusAccepted, j.snapshot())
}

// peek reads the manifest of a package without unpacking it.
func (m *Module) peek(ctx context.Context, e entry) (manifest, error) {
	rc, _, err := e.store.Get(ctx, e.key)
	if err != nil {
		return manifest{}, err
	}
	defer rc.Close()
	ar, err := openArchive(rc)
	if err != nil {
		return manifest{}, err
	}
	defer ar.Close()
	return ar.readManifest()
}

// stageRestore backs up the current data, unpacks the package and stops the
// server so the supervisor restarts it into the restored data.
func (m *Module) stageRestore(ctx context.Context, j *job, e entry, current string) error {
	j.step("正在解包备份")
	j.set(func(v *api.BackupJob) { v.DoneBytes, v.TotalBytes = nil, nil })
	rc, _, err := e.store.Get(ctx, e.key)
	if err != nil {
		return err
	}
	defer rc.Close()
	if _, err := stage(ctx, rc, m.d.Config.RestoreDir(), current, restoreNote{Source: baseName(e), Actor: audit.Actor(ctx)}); err != nil {
		return err
	}
	j.step("正在重启服务并载入数据")
	m.d.Audit.Record(ctx, "backup.restore.staged", baseName(e), nil, nil)
	m.exit()
	return nil
}

func baseName(e entry) string { return filepath.Base(e.key) }

func (m *Module) finishRestore(ctx context.Context, j *job) error {
	return m.finishStagedRestore(ctx, j)
}

func applyFiles(ctx context.Context, dst files.Store, srcDir string, progress func(done, total int64)) error {
	return applyStagedFiles(ctx, dst, srcDir, progress)
}

// unreadableSecrets lists the encrypted settings this server's master key
// cannot open, which happens when a backup comes from another installation.
func unreadableSecrets(ctx context.Context, db *sql.DB, box *secrets.Box) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings WHERE encrypted = 1 ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if _, err := box.Open(value); err != nil {
			keys = append(keys, key)
		}
	}
	return keys, rows.Err()
}
