package notes

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// B243: visitors who open a share link with allow_edit can change the title
// and the body of the note.

const (
	maxEditorNameLen     = 20
	maxSharedTitleLen    = 200 // the notes have no title limit of their own
	shareEditNotifyEvery = 10 * time.Minute
)

var errNoteEditDisabled = httpx.NewError(403, "note_edit_disabled", "这个链接不能编辑")

// attachmentThumb is the thumb parameter of an attachment address, the only
// parameter rewriteNoteAttachments keeps.
func attachmentThumb(match string) string {
	_, query, ok := strings.Cut(match, "?")
	if !ok {
		return ""
	}
	values, err := url.ParseQuery(query)
	if err != nil {
		return ""
	}
	return values.Get("thumb")
}

// restoreNoteAttachments is the reverse of rewriteNoteAttachments for one
// token: public file addresses of this token go back to the note's own
// attachment addresses. An address that the original body already has (same
// file, same thumb) is written back exactly as it was there, so saving an
// unchanged text gives the same text. Addresses of other tokens stay.
func restoreNoteAttachments(body, token, original string) string {
	known := map[string]string{}
	for _, match := range sharedAttachment.FindAllStringSubmatch(original, -1) {
		key := match[1] + "\x00" + attachmentThumb(match[0])
		if _, exists := known[key]; !exists {
			known[key] = match[0]
		}
	}
	public := regexp.MustCompile(`/api/v1/public/notes/` + regexp.QuoteMeta(token) + `/files/([0-9]+)(?:\?[^\s\)\]>"']*)?`)
	return public.ReplaceAllStringFunc(body, func(match string) string {
		id := public.FindStringSubmatch(match)[1]
		thumb := attachmentThumb(match)
		if previous, ok := known[id+"\x00"+thumb]; ok {
			return previous
		}
		path := "/api/v1/notes/attachments/" + id
		if thumb != "" {
			path += "?" + url.Values{"thumb": {thumb}}.Encode()
		}
		return path
	})
}

// cleanEditorName trims the name a visitor typed and removes line breaks and
// other control characters: it goes into the audit log and a notification.
func cleanEditorName(name string) (string, error) {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name))
	if utf8.RuneCountInString(name) > maxEditorNameLen {
		return "", httpx.Invalid("称呼最多 20 个字")
	}
	return name, nil
}

func (m *Module) EditPublicNote(w http.ResponseWriter, r *http.Request, token api.NoteShareToken, params api.EditPublicNoteParams) {
	s, n, ok := m.publicNote(w, r, token, params.T)
	if !ok {
		return
	}
	if !s.AllowEdit {
		httpx.Fail(w, r, errNoteEditDisabled)
		return
	}
	var in api.PublicNoteEdit
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.editPublicNote(r, s, n.Body, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, api.PublicNote{Title: out.Title, Body: rewriteNoteAttachments(out.Body, token, deref(params.T)),
		Color: out.Color, UpdatedAt: out.UpdatedAt, CanEdit: new(true)})
}

func (m *Module) editPublicNote(r *http.Request, s noteShareRow, before string, in api.PublicNoteEdit) (out api.Note, err error) {
	ctx := r.Context()
	name, nameErr := cleanEditorName(deref(in.EditorName))
	detail := map[string]any{"editor": name, "ip": m.noteClientIP(r), "before": utf8.RuneCountInString(before)}
	defer func() {
		m.d.Audit.Record(ctx, "note.share.edit", strconv.FormatInt(s.NoteID, 10), detail, err)
	}()
	if nameErr != nil {
		return out, nameErr
	}
	title := strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(title) > maxSharedTitleLen {
		return out, httpx.Invalid("标题太长，最多 200 个字")
	}
	body := restoreNoteAttachments(in.Body, s.Token, before)
	detail["after"] = utf8.RuneCountInString(body)
	out, err = m.updateNote(ctx, s.NoteID, notePatch{Title: &title, Body: &body, BaseUpdatedAt: &in.BaseUpdatedAt, Source: "share"})
	if err == httpx.ErrNotFound {
		err = errNoteShare
	}
	if err != nil {
		return out, err
	}
	m.notifyShareEdit(ctx, out, name)
	return out, nil
}

// notifyShareEdit tells the owner about a visitor's edit, at most once per
// note in shareEditNotifyEvery. The memory is lost on restart.
func (m *Module) notifyShareEdit(ctx context.Context, note api.Note, editor string) {
	now := m.shareClock()
	m.shareMu.Lock()
	if at, told := m.shareEditTold[note.Id]; told && at.Add(shareEditNotifyEvery).After(now) {
		m.shareMu.Unlock()
		return
	}
	m.shareEditTold[note.Id] = now
	m.shareMu.Unlock()
	if editor == "" {
		editor = "访客"
	}
	_, err := m.d.Notify.Send(context.WithoutCancel(ctx), notify.Notification{
		Kind: "note.shared_edit", Source: "notes", Link: notePath(note.Id),
		Title: "笔记被访客修改", Body: editor + "改了笔记《" + displayTitle(note.Title, note.Body) + "》",
		Data: map[string]any{"noteId": note.Id},
	})
	if err != nil {
		m.shareMu.Lock()
		delete(m.shareEditTold, note.Id)
		m.shareMu.Unlock()
		m.d.Log.Warn("note share edit notification", "id", note.Id, "err", err)
	}
}
