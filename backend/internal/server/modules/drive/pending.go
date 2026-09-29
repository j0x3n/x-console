package drive

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B31（docs/specs/B31.md 的“后端（待做，给开发者）”）

// PublicPaths 让分享页的公开接口不用登录。实现分享时把它挪到 share.go。
func (m *Module) PublicPaths() []string { return []string{"/public/shares"} }

func (m *Module) ArchiveDriveItems(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CopyDriveItems(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) MoveDriveItems(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ExtractDriveItem(w http.ResponseWriter, r *http.Request, itemId api.ItemId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) FollowDriveItem(w http.ResponseWriter, r *http.Request, itemId api.ItemId, params api.FollowDriveItemParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListDriveVersions(w http.ResponseWriter, r *http.Request, itemId api.ItemId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetDriveVersionContent(w http.ResponseWriter, r *http.Request, itemId api.ItemId, versionId api.VersionId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) RestoreDriveVersion(w http.ResponseWriter, r *http.Request, itemId api.ItemId, versionId api.VersionId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListDriveShares(w http.ResponseWriter, r *http.Request, params api.ListDriveSharesParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CreateDriveShare(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DeleteDriveShare(w http.ResponseWriter, r *http.Request, shareId int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListDriveTasks(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CancelDriveTask(w http.ResponseWriter, r *http.Request, taskId string) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetDriveVersionSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutDriveVersionSettings(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DownloadDriveZip(w http.ResponseWriter, r *http.Request, params api.DownloadDriveZipParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetPublicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.GetPublicShareParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UnlockPublicShare(w http.ResponseWriter, r *http.Request, token api.ShareToken) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListPublicShareItems(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.ListPublicShareItemsParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetPublicShareContent(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.GetPublicShareContentParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DownloadPublicShareZip(w http.ResponseWriter, r *http.Request, token api.ShareToken, params api.DownloadPublicShareZipParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
