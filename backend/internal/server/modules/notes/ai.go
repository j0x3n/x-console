package notes

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// minAutoAIRunes is how much text a note needs before a title and tags are
// made for it automatically (B40: "有内容就生成", but not for one word).
const minAutoAIRunes = 10

const noteAISchema = `{"type":"object","properties":{"title":{"type":"string"},"tags":{"type":"array","items":{"type":"string"},"maxItems":3}},"required":["title","tags"],"additionalProperties":false}`

type aiTimer struct{ cancel func() }

func (m *Module) noteAISettings(ctx context.Context) (api.NoteAiSettings, error) {
	out := api.NoteAiSettings{AutoTitle: true, AutoTags: true, TagMode: api.NoteAiSettingsTagModeSuggest}
	for _, item := range []struct {
		key string
		out *bool
	}{{"notes.ai_auto_title", &out.AutoTitle}, {"notes.ai_auto_tags", &out.AutoTags}} {
		err := m.d.Settings.Get(ctx, item.key, item.out)
		if err != nil && !errors.Is(err, settings.ErrNotSet) {
			return out, err
		}
	}
	err := m.d.Settings.Get(ctx, "notes.ai_tag_mode", &out.TagMode)
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return out, err
	}
	if client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey); ok {
		out.Available = client.Available(ctx)
	}
	return out, nil
}

func (m *Module) GetNoteAiSettings(w http.ResponseWriter, r *http.Request) {
	out, err := m.noteAISettings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) PutNoteAiSettings(w http.ResponseWriter, r *http.Request) {
	var body api.NoteAiSettingsInput
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.TagMode != nil && !body.TagMode.Valid() {
		httpx.Fail(w, r, httpx.Invalid("标签模式不正确"))
		return
	}
	if body.AutoTitle != nil {
		if err := m.d.Settings.Set(r.Context(), "notes.ai_auto_title", *body.AutoTitle); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if body.AutoTags != nil {
		if err := m.d.Settings.Set(r.Context(), "notes.ai_auto_tags", *body.AutoTags); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if body.TagMode != nil {
		if err := m.d.Settings.Set(r.Context(), "notes.ai_tag_mode", *body.TagMode); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	m.d.Audit.Record(r.Context(), "note.ai_settings.update", "", nil, nil)
	m.GetNoteAiSettings(w, r)
}

func (m *Module) DismissNoteSuggestedTags(w http.ResponseWriter, r *http.Request, id api.NoteId) {
	if _, err := m.noteRow(r.Context(), id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.d.DB.ExecContext(r.Context(), `UPDATE notes SET suggested_tags=NULL WHERE id=?`, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("note.updated", map[string]any{"id": id})
	httpx.NoContent(w)
}

func (m *Module) scheduleNoteAI(id int64, hidden bool) {
	m.aiMu.Lock()
	defer m.aiMu.Unlock()
	if timer := m.aiTimers[id]; timer != nil {
		timer.cancel()
		delete(m.aiTimers, id)
	}
	if hidden || m.aiCtx.Err() != nil {
		return
	}
	timer := &aiTimer{}
	m.aiTimers[id] = timer
	// B40: a few seconds after the save, not 10. Saves come in bursts while
	// typing; the timer restarts on each one.
	timer.cancel = m.aiDelay(3*time.Second, func() {
		m.aiMu.Lock()
		if m.aiTimers[id] != timer {
			m.aiMu.Unlock()
			return
		}
		delete(m.aiTimers, id)
		ctx := m.aiCtx
		m.aiMu.Unlock()
		if err := m.processNoteAI(ctx, id); err != nil && ctx.Err() == nil {
			m.d.Log.Warn("note AI failed", "note", id, "err", err)
		}
	})
}

// fingerprintSize caps the stored fingerprint. It keeps the smallest hashes
// of the note's 3-rune pieces (a bottom-k sketch): a 100k-character note
// stores 128 numbers instead of all of them.
const fingerprintSize = 128

func noteFingerprint(body string) string {
	runes := []rune(strings.ToLower(strings.TrimSpace(body)))
	set := map[uint64]bool{}
	for index := 0; index+3 <= len(runes); index++ {
		if unicode.IsSpace(runes[index]) {
			continue
		}
		hash := sha256.Sum256([]byte(string(runes[index : index+3])))
		set[binary.BigEndian.Uint64(hash[:8])] = true
	}
	values := make([]uint64, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	raw, _ := json.Marshal(values[:min(len(values), fingerprintSize)])
	return string(raw)
}

// changedEnough reports whether more than 30% of the text changed. Both
// sketches are compared on the smallest hashes of their union, which is a
// fair sample of both notes. Older fingerprints held every hash in order,
// so cutting them to the same size gives the same kind of sketch.
func changedEnough(previous, current string) bool {
	if previous == "" {
		return true
	}
	var oldValues, newValues []uint64
	if json.Unmarshal([]byte(previous), &oldValues) != nil || json.Unmarshal([]byte(current), &newValues) != nil {
		return true
	}
	if len(oldValues) == 0 || len(newValues) == 0 {
		return len(oldValues) != len(newValues)
	}
	sort.Slice(oldValues, func(i, j int) bool { return oldValues[i] < oldValues[j] })
	oldValues = oldValues[:min(len(oldValues), fingerprintSize)]
	old, now := map[uint64]bool{}, map[uint64]bool{}
	union := make([]uint64, 0, len(oldValues)+len(newValues))
	for _, value := range oldValues {
		old[value] = true
		union = append(union, value)
	}
	for _, value := range newValues {
		now[value] = true
		if !old[value] {
			union = append(union, value)
		}
	}
	sort.Slice(union, func(i, j int) bool { return union[i] < union[j] })
	union = union[:min(len(union), fingerprintSize)]
	common, inOld, inNew := 0, 0, 0
	for _, value := range union {
		if old[value] {
			inOld++
		}
		if now[value] {
			inNew++
		}
		if old[value] && now[value] {
			common++
		}
	}
	denominator := max(inOld, inNew)
	return float64(denominator-common)/float64(denominator) > 0.30
}

// maxNoteAIInput caps what one call sends to the model. The title and tags
// come from the start of a note; a long note must not cost a long prompt.
const maxNoteAIInput = 4000

func (m *Module) processNoteAI(ctx context.Context, id int64) error {
	client, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey)
	if !ok || !client.Available(ctx) {
		return nil
	}
	settings, err := m.noteAISettings(ctx)
	if err != nil {
		return err
	}
	if !settings.AutoTags && !settings.AutoTitle {
		return nil
	}
	var title, body string
	var hidden int64
	var checked sql.NullString
	err = m.d.DB.QueryRowContext(ctx, `SELECT title,body,hidden,ai_checked_hash FROM notes WHERE id=?`, id).Scan(&title, &body, &hidden, &checked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if hidden != 0 || utf8.RuneCountInString(strings.Join(strings.Fields(body), "")) < minAutoAIRunes {
		return nil
	}
	var tagCount int
	if err := m.d.DB.QueryRowContext(ctx, `SELECT count(*) FROM note_tags WHERE note_id=?`, id).Scan(&tagCount); err != nil {
		return err
	}
	needTitle := settings.AutoTitle && strings.TrimSpace(title) == ""
	needTags := settings.AutoTags && tagCount == 0
	if !needTitle && !needTags {
		return nil
	}
	fingerprint := noteFingerprint(body)
	if checked.Valid && !changedEnough(checked.String, fingerprint) {
		return nil
	}
	tags, err := m.tagCounts(ctx, false)
	if err != nil {
		return err
	}
	if len(tags) > 200 {
		tags = tags[:200]
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Tag)
	}
	var answer struct {
		Title string   `json:"title"`
		Tags  []string `json:"tags"`
	}
	system := "给笔记生成 20 字以内的中文标题和最多 3 个标签。尽量从已有标签中选，只有很确定时才新建。只返回 JSON。已有标签：" + strings.Join(names, "、")
	input := body
	if runes := []rune(input); len(runes) > maxNoteAIInput {
		input = string(runes[:maxNoteAIInput])
	}
	if err := client.CompleteJSON(contracts.WithAIUsage(ctx, "notes", ""), "fast", system, input, json.RawMessage(noteAISchema), &answer); err != nil {
		return err
	}
	if utf8.RuneCountInString(answer.Title) > 20 {
		answer.Title = string([]rune(answer.Title)[:20])
	}
	answer.Tags, err = cleanTags(answer.Tags)
	if err != nil {
		return err
	}
	if len(answer.Tags) > 3 {
		answer.Tags = answer.Tags[:3]
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentTitle, currentBody string
	if err := tx.QueryRowContext(ctx, `SELECT title,body,hidden FROM notes WHERE id=?`, id).Scan(&currentTitle, &currentBody, &hidden); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if hidden != 0 || currentBody != body {
		return nil
	}
	if needTitle && strings.TrimSpace(currentTitle) == "" && strings.TrimSpace(answer.Title) != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE notes SET title=?,updated_at=? WHERE id=?`, strings.TrimSpace(answer.Title), m.now(), id); err != nil {
			return err
		}
	}
	if needTags {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM note_tags WHERE note_id=?`, id).Scan(&tagCount); err != nil {
			return err
		}
		if tagCount == 0 && len(answer.Tags) > 0 {
			if settings.TagMode == api.NoteAiSettingsTagModeApply {
				for _, tag := range answer.Tags {
					if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO note_tags(note_id,tag) VALUES(?,?)`, id, tag); err != nil {
						return err
					}
				}
			} else {
				raw, _ := json.Marshal(answer.Tags)
				if _, err := tx.ExecContext(ctx, `UPDATE notes SET suggested_tags=? WHERE id=?`, string(raw), id); err != nil {
					return err
				}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notes SET ai_checked_hash=? WHERE id=?`, fingerprint, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.d.Bus.Publish("note.updated", map[string]any{"id": id})
	return nil
}
