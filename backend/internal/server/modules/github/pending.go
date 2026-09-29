package github

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会退回手动填写仓库。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。全部实现后删掉这个文件。
// 对应任务：B35（docs/specs/B35.md 的“后端（待做，给开发者）”）

func (m *Module) ListGitHubAvailableRepos(w http.ResponseWriter, r *http.Request, params api.ListGitHubAvailableReposParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
