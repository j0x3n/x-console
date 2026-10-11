package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/db"
)

func toItem(r db.InboxItem) api.InboxItem {
	out := api.InboxItem{Id: r.ID, Text: r.Text, Source: api.InboxItemSource(r.Source), Status: api.InboxItemStatus(r.Status),
		SuggestState: api.InboxItemSuggestState(r.SuggestState), CreatedAt: r.CreatedAt, ClosedAt: r.ClosedAt}
	if r.SuggestState == "ready" && r.Suggestion != "" {
		var s api.InboxSuggestion
		if json.Unmarshal([]byte(r.Suggestion), &s) == nil {
			out.Suggestion = &s
		}
	}
	if r.FiledKind != "" {
		k := api.InboxKind(r.FiledKind)
		out.FiledKind = &k
		ref := r.FiledRef
		out.FiledRef = &ref
	}
	return out
}

func (m *Module) publish() { m.d.Bus.Publish("inbox.changed", map[string]any{}) }

func (m *Module) item(ctx context.Context, id int64) (db.InboxItem, error) {
	row, err := m.q.GetInboxItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

func cleanText(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", httpx.Invalid("内容不能是空的")
	}
	if utf8.RuneCountInString(s) > maxText {
		return "", httpx.Invalid("一条最多 5000 个字")
	}
	return s, nil
}

// suggestAvailable says whether a new item should get an AI suggestion.
func (m *Module) suggestAvailable(ctx context.Context) bool {
	if !m.options(ctx).Suggest {
		return false
	}
	llm, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	return ok && llm.Available(ctx)
}

// capture stores a new item and starts the suggestion. announce says the
// suggestion should be sent as a notification with buttons (Telegram).
func (m *Module) capture(ctx context.Context, text, source string, announce bool) (db.InboxItem, error) {
	text, err := cleanText(text)
	if err != nil {
		return db.InboxItem{}, err
	}
	n, err := m.q.CountOpenInboxItems(ctx)
	if err != nil {
		return db.InboxItem{}, err
	}
	if n >= maxOpen {
		return db.InboxItem{}, httpx.Invalid("收集箱里有 500 条没处理的了，先清一清")
	}
	_, onlyLink := onlyURL(text)
	state := "off"
	if onlyLink || m.suggestAvailable(ctx) {
		state = "pending"
	}
	now := m.now().UTC()
	row, err := m.q.InsertInboxItem(ctx, db.InsertInboxItemParams{Text: text, Source: source, SuggestState: state, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return db.InboxItem{}, err
	}
	m.d.Audit.Record(ctx, "inbox.capture", "", map[string]any{"id": row.ID, "source": source}, nil)
	m.publish()
	if state == "pending" {
		m.suggestAsync(row.ID, announce)
	}
	return row, nil
}

func (m *Module) ListInbox(w http.ResponseWriter, r *http.Request, p api.ListInboxParams) {
	ctx := r.Context()
	var rows []db.InboxItem
	var err error
	if p.Status != nil && *p.Status == "closed" {
		rows, err = m.q.ListClosedInboxItems(ctx)
	} else {
		rows, err = m.q.ListOpenInboxItems(ctx)
	}
	if fail(w, r, err) {
		return
	}
	open, err := m.q.CountOpenInboxItems(ctx)
	if fail(w, r, err) {
		return
	}
	items := make([]api.InboxItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, toItem(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "openCount": open})
}

func (m *Module) CreateInboxItem(w http.ResponseWriter, r *http.Request) {
	var body api.CreateInboxItemJSONBody
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	row, err := m.capture(r.Context(), body.Text, "web", false)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, toItem(row))
}

func (m *Module) UpdateInboxItem(w http.ResponseWriter, r *http.Request, id api.InboxItemId) {
	ctx := r.Context()
	var body struct {
		Text string `json:"text"`
	}
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	text, err := cleanText(body.Text)
	if fail(w, r, err) {
		return
	}
	cur, err := m.item(ctx, id)
	if fail(w, r, err) {
		return
	}
	if cur.Status != "open" {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "conflict", "这条已经处理过了"))
		return
	}
	_, onlyLink := onlyURL(text)
	state := "off"
	if onlyLink || m.suggestAvailable(ctx) {
		state = "pending"
	}
	row, err := m.q.UpdateInboxText(ctx, db.UpdateInboxTextParams{Text: text, SuggestState: state, UpdatedAt: m.now().UTC(), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	m.publish()
	if state == "pending" {
		m.suggestAsync(id, false)
	}
	httpx.JSON(w, http.StatusOK, toItem(row))
}

func (m *Module) DeleteInboxItem(w http.ResponseWriter, r *http.Request, id api.InboxItemId) {
	n, err := m.q.DeleteInboxItem(r.Context(), id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "inbox.delete", "", map[string]any{"id": id}, err)
	if fail(w, r, err) {
		return
	}
	m.publish()
	httpx.NoContent(w)
}

func (m *Module) DismissInboxItem(w http.ResponseWriter, r *http.Request, id api.InboxItemId) {
	row, err := m.dismiss(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toItem(row))
}

func (m *Module) dismiss(ctx context.Context, id int64) (db.InboxItem, error) {
	if _, err := m.item(ctx, id); err != nil {
		return db.InboxItem{}, err
	}
	now := m.now().UTC()
	row, err := m.q.CloseInboxItem(ctx, db.CloseInboxItemParams{Status: "dismissed", ClosedAt: &now, UpdatedAt: now, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.NewError(http.StatusConflict, "conflict", "这条已经处理过了")
	}
	if err == nil {
		m.d.Audit.Record(ctx, "inbox.dismiss", "", map[string]any{"id": id}, nil)
		m.publish()
	}
	return row, err
}

// CreateInboxVoice takes a recording, turns it into text and stores that.
func (m *Module) CreateInboxVoice(w http.ResponseWriter, r *http.Request, p api.CreateInboxVoiceParams) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxVoiceSize)
	audio, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "录音太大了，最多 10 MB"))
			return
		}
		httpx.Fail(w, r, httpx.Invalid("读不到录音"))
		return
	}
	lang := ""
	if p.Language != nil {
		lang = strings.ToLower(strings.TrimSpace(*p.Language))
		if lang != "" && !languageRe.MatchString(lang) {
			httpx.Fail(w, r, httpx.Invalid("语言要写 ISO-639-1 代码，比如 zh 或 en"))
			return
		}
	}
	text, err := m.transcribe(ctx, audio, r.Header.Get("Content-Type"), lang)
	if fail(w, r, err) {
		return
	}
	row, err := m.capture(ctx, text, "voice", false)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusCreated, toItem(row))
}

// audioExt picks a file extension for a recording from its content type; the
// provider reads the format from it.
func audioExt(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch mime {
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/mp4", "audio/x-m4a", "audio/m4a", "video/mp4":
		return ".m4a"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/flac":
		return ".flac"
	}
	return ".ogg"
}

// transcribe turns a recording into text with the configured provider.
func (m *Module) transcribe(ctx context.Context, audio []byte, mime, language string) (string, error) {
	tr, ok := module.Lookup[contracts.Transcriber](m.d.Registry, contracts.TranscriberKey)
	if !ok || !tr.Available(ctx) {
		return "", httpx.NewError(http.StatusConflict, "ai_unavailable", "还没有配置 AI 供应商，语音转不了文字")
	}
	opts := m.options(ctx)
	if language == "" {
		language = opts.Language
	}
	text, err := tr.Transcribe(ctx, contracts.Transcribe{Audio: audio, Filename: "voice" + audioExt(mime), Mime: mime, Model: opts.TranscribeModel, Language: language})
	if err != nil {
		return "", httpx.NewError(http.StatusBadGateway, "transcribe_failed", err.Error())
	}
	if strings.TrimSpace(text) == "" {
		return "", httpx.Invalid("没听出内容")
	}
	return text, nil
}
