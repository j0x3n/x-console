package backup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func repositoryStore(t target) files.Store {
	return files.Scoped(files.Scoped(t.store, "backups"), "x-console-repo")
}

func repoError(err error) error {
	switch {
	case errors.Is(err, files.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, files.ErrBadKey):
		return httpx.Invalid("快照名称不正确")
	case errors.Is(err, repo.ErrLocked), errors.Is(err, repo.ErrLostLock):
		return httpx.NewError(http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, repo.ErrKey):
		return httpx.NewError(http.StatusBadRequest, "backup_invalid", repo.ErrKey.Error())
	case errors.Is(err, repo.ErrCorrupt):
		return httpx.NewError(http.StatusBadRequest, "backup_invalid", repo.ErrCorrupt.Error())
	case errors.Is(err, repo.ErrMissing):
		return httpx.NewError(http.StatusPreconditionFailed, "backup_repo_missing", err.Error())
	default:
		return err
	}
}

func (m *Module) createIncremental(ctx context.Context, j *job, dest target, retention repo.Retention) (repo.Snapshot, error) {
	var snapshot repo.Snapshot
	err := m.withStableStorage(ctx, func(ctx context.Context) error {
		var err error
		snapshot, err = m.createIncrementalSite(ctx, j, dest, retention)
		return err
	})
	return snapshot, err
}

func (m *Module) createIncrementalSite(ctx context.Context, j *job, dest target, retention repo.Retention) (repo.Snapshot, error) {
	var out repo.Snapshot
	if dest.gdrive != nil {
		if _, err := dest.gdrive.Folder(ctx); err != nil {
			return out, err
		}
	}
	r, lease, err := repo.Acquire(ctx, repositoryStore(dest), m.d.Config.MasterKey, m.d.Config.DataDir, "backup", true)
	if err != nil {
		return out, repoError(err)
	}
	defer lease.Close()
	ctx = lease.Context()
	tmp, err := os.MkdirTemp(m.d.Config.TmpDir(), "incremental-")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmp)
	j.step("正在备份数据库")
	dbPath := filepath.Join(tmp, dbName)
	releaseTemp := contracts.TrackTemporaryFile(dbPath)
	defer releaseTemp()
	_, size, err := snapshotDB(ctx, m.d.DB, dbPath)
	if err != nil {
		return out, err
	}
	migration, err := latestMigration(ctx, m.d.DB)
	if err != nil {
		return out, err
	}
	list, _, err := m.siteFiles(ctx)
	if err != nil {
		return out, err
	}
	f, err := os.Open(dbPath)
	if err != nil {
		return out, err
	}
	defer f.Close()
	j.step("正在上传新增内容块")
	out, err = r.CreateSnapshot(ctx, repo.Input{Database: f, DatabaseSize: size, Files: m.d.Files.Store(), Entries: list, CreatedAt: m.now().UTC(), Migration: migration, Version: versionText(), Progress: func(done, total int64) { j.set(func(v *api.BackupJob) { v.DoneBytes, v.TotalBytes = &done, &total }) }})
	if err != nil {
		return out, err
	}
	j.set(func(v *api.BackupJob) { v.BackupId = &out.ID })
	m.recordRun(ctx, out.ID, out.UploadedBytes, nil)
	j.step("备份成功，正在清理过期快照")
	pruned, pruneErr := r.Prune(ctx, retention, m.now(), m.d.Config.Location)
	j.set(func(v *api.BackupJob) {
		v.Prune = &api.BackupPruneResult{Snapshots: pruned.Snapshots, Blocks: pruned.Blocks, Bytes: pruned.Bytes}
	})
	m.d.Audit.Record(ctx, "backup.prune", out.ID, map[string]any{"snapshots": pruned.Snapshots, "blocks": pruned.Blocks, "bytes": pruned.Bytes}, pruneErr)
	if pruneErr != nil {
		m.log().Warn("backup: incremental prune failed", "error", pruneErr)
		field := "本次备份已成功，过期快照清理失败，请检查存储后重试"
		j.set(func(v *api.BackupJob) { v.Warning = &field })
	}
	return out, nil
}

func (m *Module) ListBackupSnapshots(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	dest, err := m.remote(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	repository, err := repo.Open(ctx, repositoryStore(dest), m.d.Config.MasterKey)
	if errors.Is(err, repo.ErrMissing) {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []repo.Snapshot{}, "stats": repo.Stats{}})
		return
	}
	if err != nil {
		httpx.Fail(w, r, repoError(err))
		return
	}
	all, err := repository.ListSnapshots(ctx)
	if err != nil {
		httpx.Fail(w, r, repoError(err))
		return
	}
	stats, err := repository.Stats(ctx, all)
	if err != nil {
		httpx.Fail(w, r, repoError(err))
		return
	}
	items := make([]map[string]any, 0, len(all))
	for _, s := range all {
		visible := m.visibleChanges(ctx, s.Changes)
		added, modified, deleted := 0, 0, 0
		if m.pathsVisible(ctx) {
			added, modified, deleted = s.Added, s.Modified, s.Deleted
		} else {
			for _, c := range visible {
				switch c.Kind {
				case "added":
					added++
				case "modified":
					modified++
				case "deleted":
					deleted++
				}
			}
		}
		items = append(items, map[string]any{"id": s.ID, "createdAt": s.CreatedAt, "version": s.Version, "files": len(s.Files), "sizeBytes": s.SizeBytes, "uploadedBytes": s.UploadedBytes, "added": added, "modified": modified, "deleted": deleted, "changesTruncated": s.ChangesTruncated})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "stats": stats})
}

func (m *Module) visibleChanges(ctx context.Context, changes []repo.Change) []repo.Change {
	out := []repo.Change{}
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	for _, c := range changes {
		mod := files.Module(c.Path)
		if !auth.VaultUnlocked(ctx) && (mod == "notes" || mod == "drive" || mod == "drive-versions" || ok && h.Hidden(ctx, mod)) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (m *Module) pathsVisible(ctx context.Context) bool {
	if auth.VaultUnlocked(ctx) {
		return true
	}
	return false
}

func (m *Module) GetBackupSnapshotChanges(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
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
	httpx.JSON(w, http.StatusOK, map[string]any{"items": m.visibleChanges(ctx, s.Changes), "truncated": s.ChangesTruncated})
}

func (m *Module) CheckBackup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	j, err := m.begin(api.BackupJobKind("check"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		dest, err := m.remote(bg)
		if err == nil {
			var repository *repo.Repository
			var lease *repo.Lease
			repository, lease, err = repo.Acquire(bg, repositoryStore(dest), m.d.Config.MasterKey, m.d.Config.DataDir, "check", false)
			if err == nil {
				j.step("正在检查快照和块的存在及大小")
				result, checkErr := repository.Check(lease.Context())
				err = checkErr
				if err == nil {
					j.set(func(v *api.BackupJob) {
						v.Check = &api.BackupCheckResult{Snapshots: result.Snapshots, Blocks: result.Blocks, CheckedAt: result.CheckedAt}
					})
					j.step(fmt.Sprintf("已检查 %d 个快照、%d 个块", result.Snapshots, result.Blocks))
				}
				lease.Close()
			}
		}
		m.d.Audit.Record(bg, "backup.check", "", nil, err)
		m.end(j, publicBackupError(err))
	}()
	httpx.JSON(w, http.StatusAccepted, j.snapshot())
}

func snapshotSource(id string) string { return "snapshot:" + strings.TrimSpace(id) }
