package core

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B30（docs/specs/B22.md、B30.md）

func (h *Handlers) DownloadAgent(w http.ResponseWriter, r *http.Request, os api.DownloadAgentParamsOs, arch api.DownloadAgentParamsArch, params api.DownloadAgentParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (h *Handlers) GetAgentInstallPowerShell(w http.ResponseWriter, r *http.Request, params api.GetAgentInstallPowerShellParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (h *Handlers) GetAgentInstallScript(w http.ResponseWriter, r *http.Request, params api.GetAgentInstallScriptParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (h *Handlers) DownloadAgentSetup(w http.ResponseWriter, r *http.Request, params api.DownloadAgentSetupParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (h *Handlers) GetAgentUninstallScript(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
