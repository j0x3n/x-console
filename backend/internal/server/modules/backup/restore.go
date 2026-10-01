package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
	if err := os.RemoveAll(dir); err != nil {
		return manifest{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return manifest{}, err
	}
	man, err := stageInto(ctx, src, dir, currentMigration, note)
	if err != nil {
		_ = os.RemoveAll(dir)
	}
	return man, err
}

func stageInto(ctx context.Context, src io.Reader, dir, currentMigration string, note restoreNote) (manifest, error) {
	ar, err := openArchive(src)
	if err != nil {
		return manifest{}, err
	}
	defer ar.Close()
	man, err := ar.readManifest()
	if err != nil {
		return man, err
	}
	if newerMigration(man.Migration, currentMigration) {
		return man, errNewer
	}
	fileStore := files.Local{Root: filepath.Join(dir, "files")}
	haveDB := false
	for {
		hdr, err := ar.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return man, fmt.Errorf("备份文件已损坏：%w", err)
		}
		if err := ctx.Err(); err != nil {
			return man, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		switch {
		case hdr.Name == dbName:
			if err := unpackDB(ar.tr, filepath.Join(dir, dbName), man.DBSha256); err != nil {
				return man, err
			}
			haveDB = true
		case strings.HasPrefix(hdr.Name, filesPrefix):
			key := strings.TrimPrefix(hdr.Name, filesPrefix)
			if err := files.CheckKey(key); err != nil {
				return man, fmt.Errorf("备份里有不合法的文件名：%q", key)
			}
			if err := fileStore.Put(ctx, key, ar.tr, hdr.Size); err != nil {
				return man, fmt.Errorf("解包 %s 失败：%w", key, err)
			}
		}
	}
	if !haveDB {
		return man, errNotBackup
	}
	raw, _ := json.Marshal(note)
	if err := os.WriteFile(filepath.Join(dir, noteName), raw, 0o600); err != nil {
		return man, err
	}
	return man, os.WriteFile(filepath.Join(dir, readyName), nil, 0o600)
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
func ApplyPending(dataDir string) (bool, error) {
	dir := filepath.Join(dataDir, restoreFolder)
	if !pending(dir) {
		return false, nil
	}
	staged := filepath.Join(dir, dbName)
	if _, err := os.Stat(staged); errors.Is(err, fs.ErrNotExist) {
		return true, nil // applied by an earlier start that stopped before the files were copied
	}
	target := filepath.Join(dataDir, dbName)
	// A leftover log of the old database must not be replayed onto the new one.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(target + suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	if err := os.Rename(staged, target); err != nil {
		return false, fmt.Errorf("换上恢复的数据库失败：%w", err)
	}
	return true, nil
}

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
	if _, _, err := m.create(ctx, kindPreRestore, m.local, "", false, j); err != nil {
		return fmt.Errorf("恢复前备份当前数据失败，没有做任何改动：%w", err)
	}
	m.pruneLocal(ctx)
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

// startFinishRestore runs the file phase of a restore after the restart.
func (m *Module) startFinishRestore() {
	kind := api.BackupJobKindRestore
	j := &job{v: api.BackupJob{Kind: &kind, State: api.Running, StartedAt: ptr(m.now().UTC()), Step: ptr("正在恢复文件")}}
	m.cur, m.busy = j, true
	go func() {
		ctx := audit.WithActor(context.Background(), "system:restore")
		err := m.finishRestore(ctx, j)
		m.end(j, err)
		if err == nil {
			m.d.Bus.Publish("backup.restored", nil)
		}
	}()
}

func (m *Module) finishRestore(ctx context.Context, j *job) error {
	dir := m.d.Config.RestoreDir()
	defer os.RemoveAll(dir)
	var note restoreNote
	if raw, err := os.ReadFile(filepath.Join(dir, noteName)); err == nil {
		_ = json.Unmarshal(raw, &note)
	}
	err := applyFiles(ctx, m.d.Files.Store(), filepath.Join(dir, "files"), func(done, total int64) {
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
	m.d.Audit.Record(audit.WithActor(ctx, note.Actor), "backup.restore", note.Source, nil, err)
	return err
}

// applyFiles makes dst hold exactly the files of a package: the ones the
// package does not have are deleted, the others are written.
func applyFiles(ctx context.Context, dst files.Store, srcDir string, progress func(done, total int64)) error {
	src := files.Local{Root: srcDir}
	var list []files.Info
	inPackage := map[string]bool{}
	for info, err := range src.List(ctx, "") {
		if err != nil {
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
		if !inPackage[info.Key] && files.Module(info.Key) != "backups" {
			stale = append(stale, info.Key)
		}
	}
	for _, key := range stale {
		if err := dst.Delete(ctx, key); err != nil {
			return fmt.Errorf("删除 %s 失败：%w", key, err)
		}
	}
	for i, info := range list {
		rc, got, err := src.Get(ctx, info.Key)
		if err != nil {
			return err
		}
		err = dst.Put(ctx, info.Key, rc, got.Size)
		rc.Close()
		if err != nil {
			return fmt.Errorf("恢复 %s 失败：%w", info.Key, err)
		}
		progress(int64(i+1), int64(len(list)))
	}
	return nil
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
