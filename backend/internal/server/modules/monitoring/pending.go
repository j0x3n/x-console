package monitoring

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B23、B28（docs/specs/B23.md、B28.md）

func (m *Module) PruneImages(w http.ResponseWriter, r *http.Request, hostId api.HostId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) RemoveImage(w http.ResponseWriter, r *http.Request, hostId api.HostId, imageId string) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListSubscriptionCategories(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) CreateSubscriptionCategory(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DeleteSubscriptionCategory(w http.ResponseWriter, r *http.Request, categoryId int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UpdateSubscriptionCategory(w http.ResponseWriter, r *http.Request, categoryId int64) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
