package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func applyPreparedDatabase(dataDir string) (bool, error) {
	dir := filepath.Join(dataDir, restoreFolder)
	if !pending(dir) {
		return false, nil
	}
	staged := filepath.Join(dir, dbName)
	if _, err := os.Stat(staged); errors.Is(err, os.ErrNotExist) {
		return true, nil
	} else if err != nil {
		return false, err
	}
	name, err := os.ReadFile(filepath.Join(dir, "pre-backup"))
	if err != nil {
		return false, fmt.Errorf("恢复前的现场备份尚未完成：%w", err)
	}
	if !backupID.MatchString(string(name)) || strings.ContainsAny(string(name), `/\`) {
		return false, fmt.Errorf("恢复前的现场备份记录已损坏")
	}
	info, err := (files.Local{Root: filepath.Join(dataDir, "backups")}).Stat(context.Background(), string(name))
	if err != nil {
		return false, fmt.Errorf("恢复前的现场备份无法读取：%w", err)
	}
	if info.Size <= 0 {
		return false, fmt.Errorf("恢复前的现场备份为空")
	}
	if err := validateStagedDB(context.Background(), staged); err != nil {
		return false, err
	}
	if err := validateStageFiles(filepath.Join(dir, "files")); err != nil {
		return false, err
	}
	if err := validateStagedSnapshot(context.Background(), dir); err != nil {
		return false, err
	}
	target := filepath.Join(dataDir, dbName)
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(target + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	if err := os.Rename(staged, target); err != nil {
		return false, fmt.Errorf("换上恢复的数据库失败：%w", err)
	}
	return true, nil
}
