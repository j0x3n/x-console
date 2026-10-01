package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/core"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

// siteFiles lists the files that go into a package. Backups kept in the same
// bucket are not part of the site.
func (m *Module) siteFiles(ctx context.Context) ([]files.Info, int64, error) {
	var list []files.Info
	var total int64
	for info, err := range m.d.Files.Store().List(ctx, "") {
		if err != nil {
			return nil, 0, fmt.Errorf("读取文件列表失败：%w", err)
		}
		if files.Module(info.Key) == "backups" {
			continue
		}
		list = append(list, info)
		total += info.Size
	}
	return list, total, nil
}

// versionText is the program version made safe for a file name.
func versionText() string {
	v := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		}
		return '-'
	}, core.Version)
	if v == "" {
		return "dev"
	}
	return v
}

// uniqueName picks x-console-<date>-<time>-<version>.tar.gz, with a counter
// when a package of the same minute exists.
func (m *Module) uniqueName(ctx context.Context, dest files.Store, prefix string, at time.Time) string {
	base := fmt.Sprintf("x-console-%s-%s", at.In(m.d.Config.Location).Format("20060102-1504"), versionText())
	name := base + ".tar.gz"
	for i := 2; ; i++ {
		if _, err := dest.Stat(ctx, prefix+name); err != nil {
			return name
		}
		name = fmt.Sprintf("%s-%d.tar.gz", base, i)
	}
}

// create writes one package into dest and returns its name and size.
// prefix is the key prefix in dest ("" for the local folder, "backups/" in S3).
// spool packs into a temporary file first and uploads it afterwards, which an
// S3 needs because it must be told the size; the local folder is streamed to.
func (m *Module) create(ctx context.Context, kind string, dest files.Store, prefix string, spool bool, j *job) (string, int64, error) {
	tmpDir := m.d.Config.TmpDir()
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return "", 0, err
	}
	dbPath := filepath.Join(tmpDir, fmt.Sprintf("backup-%d.db", time.Now().UnixNano()))
	defer os.Remove(dbPath)

	j.step("正在备份数据库")
	sum, dbSize, err := snapshotDB(ctx, m.d.DB, dbPath)
	if err != nil {
		return "", 0, err
	}
	migration, err := latestMigration(ctx, m.d.DB)
	if err != nil {
		return "", 0, err
	}
	j.step("正在统计文件")
	list, listed, err := m.siteFiles(ctx)
	if err != nil {
		return "", 0, err
	}
	created := m.now().UTC()
	man := manifest{Format: formatVersion, Version: versionText(), CreatedAt: created, Files: int64(len(list)),
		Bytes: listed, DBSha256: sum, Migration: migration}
	name := m.uniqueName(ctx, dest, prefix, created)
	total := dbSize + listed
	j.set(func(v *api.BackupJob) { v.TotalBytes, v.DoneBytes = &total, ptr(dbSize) })

	in := packInput{Manifest: man, DBPath: dbPath, Store: m.d.Files.Store(), Files: list,
		Progress: func(done, bytes int64) {
			j.set(func(v *api.BackupJob) {
				v.DoneBytes = ptr(dbSize + bytes)
				v.Step = ptr(fmt.Sprintf("正在打包文件（%d/%d）", done, len(list)))
			})
		}}
	j.step("正在打包文件")
	if err := putArchive(ctx, dest, prefix+name, spool, tmpDir, in); err != nil {
		_ = dest.Delete(context.WithoutCancel(ctx), prefix+name)
		return "", 0, fmt.Errorf("写备份包失败：%w", err)
	}
	info, err := dest.Stat(ctx, prefix+name)
	if err != nil {
		return "", 0, err
	}
	if err := writeSidecar(ctx, dest, prefix+name, sidecar{manifest: man, Kind: kind}); err != nil {
		_ = dest.Delete(context.WithoutCancel(ctx), prefix+name)
		return "", 0, err
	}
	return name, info.Size, nil
}

func writeSidecar(ctx context.Context, s files.Store, key string, sc sidecar) error {
	raw, err := json.Marshal(sc)
	if err != nil {
		return err
	}
	return s.Put(ctx, key+".json", strings.NewReader(string(raw)), int64(len(raw)))
}

// readSidecar returns the sidecar of a package, or false when there is none.
func readSidecar(ctx context.Context, s files.Store, key string) (sidecar, bool) {
	var sc sidecar
	rc, _, err := s.Get(ctx, key+".json")
	if err != nil {
		return sc, false
	}
	defer rc.Close()
	if err := json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&sc); err != nil {
		return sc, false
	}
	return sc, true
}

// entry is one package found in a folder or bucket.
type entry struct {
	store    files.Store
	key      string
	location api.BackupLocation
	info     files.Info
	meta     sidecar
	hasMeta  bool
}

func (e entry) kind() string {
	if e.hasMeta && e.meta.Kind != "" {
		return e.meta.Kind
	}
	return kindManual
}

func (e entry) createdAt() time.Time {
	if e.hasMeta && !e.meta.CreatedAt.IsZero() {
		return e.meta.CreatedAt
	}
	return e.info.ModTime
}

func (e entry) toAPI() api.Backup {
	name := path.Base(e.key)
	out := api.Backup{Id: name, Name: name, CreatedAt: e.createdAt(), SizeBytes: e.info.Size,
		Location: e.location, Kind: api.BackupKind(e.kind())}
	if e.hasMeta {
		out.Version, out.Files = &e.meta.Version, &e.meta.Files
	}
	return out
}

// scan lists the packages in one store below prefix.
func scan(ctx context.Context, s files.Store, prefix string, loc api.BackupLocation) ([]entry, error) {
	var out []entry
	for info, err := range s.List(ctx, strings.TrimSuffix(prefix, "/")) {
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(info.Key, ".tar.gz") || !backupID.MatchString(path.Base(info.Key)) {
			continue
		}
		e := entry{store: s, key: info.Key, location: loc, info: info}
		e.meta, e.hasMeta = readSidecar(ctx, s, info.Key)
		out = append(out, e)
	}
	return out, nil
}

// entries lists every package: this machine first, then the S3 folder when
// the automatic backup has one. An S3 that does not answer is left out.
func (m *Module) entries(ctx context.Context) ([]entry, error) {
	out, err := scan(ctx, m.local, "", api.BackupLocationLocal)
	if err != nil {
		return nil, err
	}
	if remote, err := m.remote(ctx); err == nil {
		if more, err := scan(ctx, remote.store, remoteFolder, remote.loc); err == nil {
			out = append(out, more...)
		} else {
			m.log().Warn("backup: list remote backups", "location", remote.loc, "error", err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].createdAt().After(out[j].createdAt()) })
	return out, nil
}

// find returns the package with the given file name.
func (m *Module) find(ctx context.Context, id string) (entry, error) {
	if !backupID.MatchString(id) {
		return entry{}, errBadID
	}
	if info, err := m.local.Stat(ctx, id); err == nil {
		e := entry{store: m.local, key: id, location: api.BackupLocationLocal, info: info}
		e.meta, e.hasMeta = readSidecar(ctx, m.local, id)
		return e, nil
	}
	if remote, err := m.remote(ctx); err == nil {
		if info, err := remote.store.Stat(ctx, remoteFolder+id); err == nil {
			e := entry{store: remote.store, key: remoteFolder + id, location: remote.loc, info: info}
			e.meta, e.hasMeta = readSidecar(ctx, remote.store, remoteFolder+id)
			return e, nil
		}
	}
	return entry{}, errMissing
}

// remove deletes a package and its sidecar.
func (e entry) remove(ctx context.Context) error {
	return errors.Join(e.store.Delete(ctx, e.key), e.store.Delete(ctx, e.key+".json"))
}

// pruneLocal keeps the newest packages of each kind that only lives on this
// machine. Uploaded packages are never removed.
func (m *Module) pruneLocal(ctx context.Context) {
	all, err := scan(ctx, m.local, "", api.BackupLocationLocal)
	if err != nil {
		m.log().Warn("backup: prune local", "error", err)
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].createdAt().After(all[j].createdAt()) })
	seen := map[string]int{}
	for _, e := range all {
		kind := e.kind()
		if kind != kindManual && kind != kindPreRestore {
			continue
		}
		seen[kind]++
		if seen[kind] > keepLocal {
			if err := e.remove(ctx); err != nil {
				m.log().Warn("backup: remove old backup", "name", e.key, "error", err)
			}
		}
	}
}

// putArchive writes the package to dest under key.
func putArchive(ctx context.Context, dest files.Store, key string, spool bool, tmpDir string, in packInput) error {
	if !spool {
		pr, pw := io.Pipe()
		go func() { pw.CloseWithError(writeArchive(ctx, pw, in)) }()
		err := dest.Put(ctx, key, pr, -1)
		pr.CloseWithError(err) // lets the writer stop if Put gave up early
		return err
	}
	f, err := os.CreateTemp(tmpDir, "backup-*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := writeArchive(ctx, f, in); err != nil {
		return err
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return dest.Put(ctx, key, f, size)
}
