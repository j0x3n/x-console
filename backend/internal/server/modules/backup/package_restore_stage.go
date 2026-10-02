package backup

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func stagePackage(ctx context.Context, src io.Reader, dir, current string, note restoreNote) (manifest, error) {
	if pending(dir) {
		return manifest{}, errBusy
	}
	if err := os.RemoveAll(dir); err != nil {
		return manifest{}, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "files"), 0o700); err != nil {
		return manifest{}, err
	}
	return stagePackageInto(ctx, src, dir, current, note)
}

func stagePackageInto(ctx context.Context, src io.Reader, dir, current string, note restoreNote) (manifest, error) {
	ar, err := openArchive(src)
	if err != nil {
		return manifest{}, err
	}
	defer ar.Close()
	man, err := ar.readManifest()
	if err != nil {
		return man, err
	}
	migration, parseErr := strconv.ParseInt(man.Migration, 10, 64)
	checksum, hashErr := hex.DecodeString(man.DBSha256)
	if parseErr != nil || migration < 0 || hashErr != nil || len(checksum) != sha256.Size || man.Files < 0 || man.Bytes < 0 {
		return man, errNotBackup
	}
	if newerMigration(man.Migration, current) {
		return man, errNewer
	}
	local := files.Local{Root: dir}
	seen := map[string]bool{}
	snapshot := repo.Snapshot{Files: []repo.File{}}
	var total int64
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
		if hdr.Typeflag == tar.TypeDir {
			key := strings.TrimSuffix(hdr.Name, "/")
			if key != "files" && (!strings.HasPrefix(key, filesPrefix) || repo.ValidatePath(strings.TrimPrefix(key, filesPrefix)) != nil) {
				return man, errNotBackup
			}
			continue
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size < 0 || total > maxUpload-hdr.Size {
			return man, fmt.Errorf("备份条目类型或解包大小不支持")
		}
		total += hdr.Size
		key := hdr.Name
		switch {
		case key == dbName:
			if haveDB || hdr.Size == 0 {
				return man, errNotBackup
			}
			haveDB = true
		case strings.HasPrefix(key, filesPrefix):
			file := strings.TrimPrefix(key, filesPrefix)
			if repo.ValidatePath(file) != nil || restorePathConflict(seen, file) {
				return man, fmt.Errorf("备份里有重复或不支持的文件路径")
			}
			seen[strings.ToLower(file)] = true
		default:
			return man, errNotBackup
		}
		h := sha256.New()
		if err := local.Put(ctx, key, io.TeeReader(ar.tr, h), hdr.Size); err != nil {
			return man, fmt.Errorf("写入恢复目录失败：%w", err)
		}
		f := repo.File{Size: hdr.Size, SHA256: hex.EncodeToString(h.Sum(nil)), ModTime: hdr.ModTime}
		if key == dbName {
			if f.SHA256 != strings.ToLower(man.DBSha256) {
				return man, fmt.Errorf("备份里的数据库校验失败")
			}
			snapshot.Database = f
		} else {
			f.Path = strings.TrimPrefix(key, filesPrefix)
			snapshot.Files = append(snapshot.Files, f)
		}
	}
	if !haveDB {
		return man, errNotBackup
	}
	if _, err := io.Copy(io.Discard, ar.gz); err != nil {
		return man, fmt.Errorf("备份压缩校验失败：%w", err)
	}
	if err := validateStagedDB(ctx, filepath.Join(dir, dbName)); err != nil {
		return man, err
	}
	if err := writeRestoreNote(dir, note); err != nil {
		return man, err
	}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return man, err
	}
	if err := writeStageData(ctx, dir, "snapshot.json", raw); err != nil {
		return man, err
	}
	return man, writeStageData(ctx, dir, readyName, nil)
}

func restorePathConflict(seen map[string]bool, key string) bool {
	key = strings.ToLower(key)
	if seen[key] {
		return true
	}
	for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
		if seen[parent] {
			return true
		}
	}
	for old := range seen {
		if strings.HasPrefix(old, key+"/") {
			return true
		}
	}
	return false
}

func writeStageData(ctx context.Context, dir, key string, raw []byte) error {
	return (files.Local{Root: dir}).Put(ctx, key, strings.NewReader(string(raw)), int64(len(raw)))
}
