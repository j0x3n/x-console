package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func (m *Module) RestoreBackupSnapshot(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
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
	dest, err := m.remote(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	repository, err := repo.Open(ctx, repositoryStore(dest), m.d.Config.MasterKey)
	if err != nil {
		httpx.Fail(w, r, repoError(err))
		return
	}
	s, err := repository.ReadSnapshot(ctx, id)
	if err != nil {
		httpx.Fail(w, r, repoError(err))
		return
	}
	current, err := latestMigration(ctx, m.d.DB)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if newerMigration(s.Migration, current) {
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
		err := m.stageSnapshot(bg, j, dest, id, current)
		if err != nil {
			m.d.Audit.Record(bg, "backup.restore", id, nil, err)
			m.end(j, publicBackupError(err))
			return
		}
		m.d.Audit.Record(bg, "backup.restore.staged", id, nil, nil)
		m.exit()
	}()
	httpx.JSON(w, http.StatusAccepted, j.snapshot())
}

func (m *Module) stageSnapshot(ctx context.Context, j *job, dest target, id, current string) error {
	repository, lease, err := repo.Acquire(ctx, repositoryStore(dest), m.d.Config.MasterKey, m.d.Config.DataDir, "restore", false)
	if err != nil {
		return err
	}
	defer lease.Close()
	ctx = lease.Context()
	dir := m.d.Config.RestoreDir()
	if pending(dir) {
		return errBusy
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, dbName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	j.step("正在下载并验证快照")
	s, err := repository.Restore(ctx, id, f, files.Local{Root: filepath.Join(dir, "files")}, func(done, total int64) {
		j.set(func(v *api.BackupJob) {
			v.Step = ptr(fmt.Sprintf("正在准备恢复文件（%d/%d）", done, total))
			v.DoneBytes, v.TotalBytes = &done, &total
		})
	})
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if newerMigration(s.Migration, current) {
		return errNewer
	}
	if err := validateStagedDB(ctx, filepath.Join(dir, dbName)); err != nil {
		return err
	}
	if err := writeRestoreNote(dir, restoreNote{Source: snapshotSource(id), Actor: audit.Actor(ctx)}); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := writeStageData(ctx, dir, "snapshot.json", raw); err != nil {
		return err
	}
	if err := writeStageData(ctx, dir, readyName, nil); err != nil {
		return err
	}
	j.step("正在停止服务，启动时备份现场并恢复")
	return nil
}

func writeRestoreNote(dir string, note restoreNote) error {
	raw, err := json.Marshal(note)
	if err != nil {
		return err
	}
	return writeStageData(context.Background(), dir, noteName, raw)
}

func (m *Module) PreparePending(ctx context.Context) error {
	dir := m.d.Config.RestoreDir()
	if !pending(dir) {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, dbName)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateStagedDB(ctx, filepath.Join(dir, dbName)); err != nil {
		return err
	}
	if err := validateStageFiles(filepath.Join(dir, "files")); err != nil {
		return err
	}
	if err := validateStagedSnapshot(ctx, dir); err != nil {
		return err
	}
	j := &job{v: api.BackupJob{State: api.Running}}
	name, _, err := m.create(ctx, kindPreRestore, m.local, "", false, j)
	if err != nil {
		return fmt.Errorf("恢复前备份现场失败，数据库和文件没有替换：%w", err)
	}
	m.pruneLocal(ctx)
	return writeStageData(ctx, dir, "pre-backup", []byte(name))
}
