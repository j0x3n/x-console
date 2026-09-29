package hosts

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B29、B33（docs/specs/B27.md、B29.md、B33.md）

func (m *Module) GetSyslog(w http.ResponseWriter, r *http.Request, hostId api.HostId, params api.GetSyslogParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) FollowSyslog(w http.ResponseWriter, r *http.Request, hostId api.HostId, params api.FollowSyslogParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ListSyslogUnits(w http.ResponseWriter, r *http.Request, hostId api.HostId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) ReadFileRange(w http.ResponseWriter, r *http.Request, hostId api.HostId, params api.ReadFileRangeParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) FollowHostFile(w http.ResponseWriter, r *http.Request, hostId api.HostId, params api.FollowHostFileParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
