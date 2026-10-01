package notes

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

// These endpoints are in the contract but not built yet. See docs/specs/B72.md and B73.md.

// PublicPaths lets the note share page reach /public/notes without a login.
// Every handler under it checks its own token (B72).
func (m *Module) PublicPaths() []string { return []string{"/public/notes/"} }

func (m *Module) GetNoteCounts(w http.ResponseWriter, r *http.Request) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetNoteShare(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) PutNoteShare(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) DeleteNoteShare(w http.ResponseWriter, r *http.Request, noteID api.NoteId) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetPublicNote(w http.ResponseWriter, r *http.Request, token api.NoteShareToken, params api.GetPublicNoteParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) UnlockPublicNote(w http.ResponseWriter, r *http.Request, token api.NoteShareToken) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}

func (m *Module) GetPublicNoteFile(w http.ResponseWriter, r *http.Request, token api.NoteShareToken, attachmentID api.AttachmentId, params api.GetPublicNoteFileParams) {
	httpx.Fail(w, r, httpx.ErrNotLive)
}
