package core

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// 接口契约里已经有、后端还没做的接口。前端看到 501 会显示“还没上线”。
// 实现某个接口时，把它从这个文件删掉，写到正式的文件里。

// GetPreferences 待 B22 后端实现：见 docs/specs/B22.md。
func (h *Handlers) GetPreferences(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

// PutPreferences 待 B22 后端实现。
func (h *Handlers) PutPreferences(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
