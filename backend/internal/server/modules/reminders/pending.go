package reminders

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B37（docs/specs/B37.md 的“后端（待做，给开发者）”）

// B37：提醒页汇总其他模块的到期事项（docs/specs/B37.md）。
func (m *Module) ListExternalReminders(w http.ResponseWriter, r *http.Request, params api.ListExternalRemindersParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
