package drive

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B31（docs/specs/B31.md 的“后端（待做，给开发者）”）

func (m *Module) FollowDriveItem(w http.ResponseWriter, r *http.Request, itemId api.ItemId, params api.FollowDriveItemParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
