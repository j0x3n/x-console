package notes

import (
	"net/http"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func (m *Module) ListNotes(w http.ResponseWriter, r *http.Request, params api.ListNotesParams) {
	f := listFilter{Q: deref(params.Q), Tag: deref(params.Tag), Pinned: params.Pinned,
		Archived: deref(params.Archived), Limit: int(httpx.Limit(params.Limit))}
	if params.Cursor != nil && *params.Cursor != "" {
		off, err := httpx.DecodeIDCursor(params.Cursor)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		f.Offset = int(off)
	}
	items, next, err := m.listNotes(r.Context(), f)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := map[string]any{"items": items}
	if next > 0 {
		out["nextCursor"] = httpx.EncodeIDCursor(int64(next))
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateNote(w http.ResponseWriter, r *http.Request) {
	var body api.CreateNote
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.createNote(r.Context(), deref(body.Title), deref(body.Body), deref(body.Tags), deref(body.Pinned))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) ListNoteTags(w http.ResponseWriter, r *http.Request) {
	out, err := m.tagCounts(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) SetNoteTagColor(w http.ResponseWriter, r *http.Request) {
	var body api.TagColorInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := m.setTagColor(r.Context(), body.Tag, body.Color); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) GetNote(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	out, err := m.getNote(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UpdateNote(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	var body api.UpdateNote
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.updateNote(r.Context(), id, notePatch{Title: body.Title, Body: body.Body, Pinned: body.Pinned,
		Archived: body.Archived, Tags: body.Tags})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) DeleteNote(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	if err := m.deleteNote(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (m *Module) NoteToIssue(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	var body api.NoteToIssueJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.noteToIssue(r.Context(), id, body.ProjectId)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) NoteToReminder(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	var body api.NoteToReminderJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	reminderID, err := m.noteToReminder(r.Context(), id, body.At, deref(body.Rrule))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]int64{"reminderId": reminderID})
}
