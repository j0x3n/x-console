package core

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// 两步验证可选（B12）。前端先做，这几个接口后端还没实现，先回 501。
var errNotReady = httpx.NewError(http.StatusNotImplemented, "not_ready", "功能还没上线")

func (h *Handlers) SkipSetupTotp(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errNotReady)
}

func (h *Handlers) EnrollTotp(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errNotReady)
}

func (h *Handlers) ConfirmTotp(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errNotReady)
}

func (h *Handlers) DisableTotp(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errNotReady)
}

func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, errNotReady)
}
