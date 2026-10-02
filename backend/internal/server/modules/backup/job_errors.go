package backup

import (
	"errors"
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func publicBackupError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repo.ErrMissing):
		return notReady(repo.ErrMissing.Error())
	case errors.Is(err, repo.ErrLocked), errors.Is(err, repo.ErrLostLock):
		return httpx.NewError(http.StatusConflict, "conflict", "备份仓库已有任务或锁已失效")
	case errors.Is(err, repo.ErrKey):
		return httpx.Invalid(repo.ErrKey.Error())
	case errors.Is(err, repo.ErrCorrupt):
		return httpx.Invalid(repo.ErrCorrupt.Error())
	case errors.Is(err, files.ErrGDriveAuth):
		return notReady(files.ErrGDriveAuth.Error())
	case errors.Is(err, errNewer):
		return errNewer
	case errors.Is(err, files.ErrNotFound):
		return httpx.NewError(http.StatusBadRequest, "backup_missing", "备份内容或现场文件已缺失，请检查存储后重试")
	default:
		return httpx.NewError(http.StatusInternalServerError, "backup_failed", "备份任务失败，请查看服务日志并检查存储")
	}
}
