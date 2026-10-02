package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func (m *Module) finishStagedRestore(ctx context.Context, j *job) error {
	dir := m.d.Config.RestoreDir()
	if err := validateStageFiles(filepath.Join(dir, "files")); err != nil {
		return err
	}
	if err := validateStagedSnapshot(ctx, dir); err != nil {
		return err
	}
	var note restoreNote
	if raw, err := os.ReadFile(filepath.Join(dir, noteName)); err == nil {
		if err := json.Unmarshal(raw, &note); err != nil {
			return err
		}
	}
	err := applyStagedFiles(ctx, m.d.Files.Store(), filepath.Join(dir, "files"), func(done, total int64) {
		j.set(func(v *api.BackupJob) {
			v.Step = ptr(fmt.Sprintf("正在恢复文件（%d/%d）", done, total))
			v.DoneBytes, v.TotalBytes = &done, &total
		})
	})
	if err == nil {
		var keys []string
		if keys, err = unreadableSecrets(ctx, m.d.DB, m.d.Secrets); err == nil && len(keys) > 0 {
			j.set(func(v *api.BackupJob) { v.SecretsUnreadable = &keys })
		}
	}
	if err == nil {
		err = os.RemoveAll(dir)
	}
	m.d.Audit.Record(audit.WithActor(ctx, note.Actor), "backup.restore", note.Source, nil, err)
	return err
}

func applyStagedFiles(ctx context.Context, dst files.Store, srcDir string, progress func(int64, int64)) error {
	if err := validateStageFiles(srcDir); err != nil {
		return err
	}
	src := files.Local{Root: srcDir}
	var list []files.Info
	inPackage := map[string]bool{}
	for info, err := range src.List(ctx, "") {
		if err != nil {
			return err
		}
		if err := repo.ValidatePath(info.Key); err != nil {
			return err
		}
		list = append(list, info)
		inPackage[info.Key] = true
	}
	var stale []string
	for info, err := range dst.List(ctx, "") {
		if err != nil {
			return err
		}
		if files.Module(info.Key) == "backups" {
			continue
		}
		if err := repo.ValidatePath(info.Key); err != nil {
			return err
		}
		if !inPackage[info.Key] {
			stale = append(stale, info.Key)
		}
	}
	total := int64(len(list) + len(stale))
	var done int64
	for _, info := range list {
		rc, got, err := src.Get(ctx, info.Key)
		if err != nil {
			return err
		}
		err = dst.Put(ctx, info.Key, rc, got.Size)
		closeErr := rc.Close()
		if err != nil {
			return fmt.Errorf("写入恢复文件失败：%w", err)
		}
		if closeErr != nil {
			return closeErr
		}
		done++
		if progress != nil {
			progress(done, total)
		}
	}
	for _, key := range stale {
		if err := dst.Delete(ctx, key); err != nil {
			return fmt.Errorf("删除现场多余文件失败：%w", err)
		}
		done++
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}
