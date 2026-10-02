package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func validateStagedDB(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&result); err != nil {
		return fmt.Errorf("备份数据库无法读取：%w", err)
	}
	if result != "ok" {
		return fmt.Errorf("备份数据库完整性检查失败")
	}
	return nil
}

func validateStagedSnapshot(ctx context.Context, dir string) error {
	raw, err := os.ReadFile(filepath.Join(dir, "snapshot.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var snapshot repo.Snapshot
	if json.Unmarshal(raw, &snapshot) != nil {
		return repo.ErrCorrupt
	}
	local := files.Local{Root: filepath.Join(dir, "files")}
	seen := map[string]bool{}
	for _, file := range snapshot.Files {
		if err := repo.ValidatePath(file.Path); err != nil {
			return err
		}
		rc, info, err := local.Get(ctx, file.Path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, rc)
		rc.Close()
		if err != nil {
			return err
		}
		if info.Size != file.Size || hex.EncodeToString(h.Sum(nil)) != file.SHA256 {
			return repo.ErrCorrupt
		}
		seen[file.Path] = true
	}
	for info, err := range local.List(ctx, "") {
		if err != nil {
			return err
		}
		if !seen[info.Key] {
			return repo.ErrCorrupt
		}
	}
	if _, err := os.Stat(filepath.Join(dir, dbName)); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		f, err := os.Open(filepath.Join(dir, dbName))
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		if n != snapshot.Database.Size || hex.EncodeToString(h.Sum(nil)) != snapshot.Database.SHA256 {
			return repo.ErrCorrupt
		}
	}
	return nil
}

func validateStageFiles(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("恢复目录不能包含符号链接")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("恢复目录只能包含普通文件")
		}
		key, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		return repo.ValidatePath(filepath.ToSlash(key))
	})
}
