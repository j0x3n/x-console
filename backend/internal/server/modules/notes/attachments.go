package notes

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

// 附件接口（B11）。前端先做，后端还没实现，先回 501。

var errNotReady = httpx.NewError(http.StatusNotImplemented, "not_ready", "功能还没上线")

func (m *Module) ListNoteAttachments(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	httpx.Fail(w, r, errNotReady)
}

func (m *Module) UploadNoteAttachment(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	httpx.Fail(w, r, errNotReady)
}

func (m *Module) DownloadNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	httpx.Fail(w, r, errNotReady)
}

func (m *Module) DeleteNoteAttachment(w http.ResponseWriter, r *http.Request, attachmentID api.AttachmentId) {
	httpx.Fail(w, r, errNotReady)
}
