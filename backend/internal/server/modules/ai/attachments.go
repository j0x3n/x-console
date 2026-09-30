package ai

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/llm"
)

// B39: files the user adds to an assistant message. Images go to the model
// as images; text and code files go in as text.

const (
	maxImageAttachment = 10 << 20
	maxTextAttachment  = 512 << 10
	maxAttachments     = 10
)

var errUnsupportedAttachment = httpx.NewError(http.StatusUnsupportedMediaType, "unsupported_type", "只支持图片（PNG、JPEG、WebP、GIF）和文本、代码文件")

func (m *Module) attachmentStore() files.Store { return m.d.Files.For("ai") }
func attachmentKey(id int64) string            { return "attachments/" + strconv.FormatInt(id, 10) }

func attachmentName(value string) string {
	value = filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	if chars := []rune(value); len(chars) > 120 {
		value = string(chars[:120])
	}
	if value == "" || value == "." || value == "/" {
		return "file"
	}
	return value
}

// attachmentKind decides by content, not by the name the browser sent.
func attachmentKind(data []byte) (kind, mime string, err error) {
	switch sniffed := http.DetectContentType(data[:min(len(data), 512)]); sniffed {
	case "image/png", "image/jpeg", "image/webp", "image/gif":
		if len(data) > maxImageAttachment {
			return "", "", httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "图片不能超过 10 MB")
		}
		return "image", sniffed, nil
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data[:min(len(data), 8<<10)]), 0) {
		return "", "", errUnsupportedAttachment
	}
	if len(data) > maxTextAttachment {
		return "", "", httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文本文件不能超过 512 KB")
	}
	return "text", "text/plain; charset=utf-8", nil
}

func (m *Module) UploadAiAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxImageAttachment+(1<<20))
	file, head, err := r.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "附件不能超过 10 MB"))
		} else {
			httpx.Fail(w, r, httpx.Invalid("需要上传文件"))
		}
		return
	}
	defer file.Close()
	defer r.MultipartForm.RemoveAll()
	data, err := io.ReadAll(io.LimitReader(file, maxImageAttachment+1))
	if m.fail(w, r, err) {
		return
	}
	if len(data) == 0 {
		httpx.Fail(w, r, httpx.Invalid("文件是空的"))
		return
	}
	kind, mime, err := attachmentKind(data)
	if m.fail(w, r, err) {
		return
	}
	out := api.AiAttachment{Name: attachmentName(head.Filename), Mime: mime, Size: int64(len(data)), Kind: api.AiAttachmentKind(kind)}
	err = m.d.DB.QueryRowContext(ctx, "INSERT INTO ai_attachments(name,mime,size,kind,created_at) VALUES(?,?,?,?,?) RETURNING id", out.Name, mime, out.Size, kind, m.now().UTC()).Scan(&out.Id)
	if m.fail(w, r, err) {
		return
	}
	if err = m.attachmentStore().Put(ctx, attachmentKey(out.Id), strings.NewReader(string(data)), out.Size); err != nil {
		_, _ = m.d.DB.ExecContext(ctx, "DELETE FROM ai_attachments WHERE id=?", out.Id)
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, out)
}

func (m *Module) DownloadAiAttachment(w http.ResponseWriter, r *http.Request, id int64) {
	var name, mime string
	err := m.d.DB.QueryRowContext(r.Context(), "SELECT name,mime FROM ai_attachments WHERE id=?", id).Scan(&name, &mime)
	if m.fail(w, r, notFound(err)) {
		return
	}
	reader, _, err := m.attachmentStore().Get(r.Context(), attachmentKey(id))
	if errors.Is(err, files.ErrNotFound) {
		err = httpx.ErrNotFound
	}
	if m.fail(w, r, err) {
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+url.PathEscape(name))
	_, _ = io.Copy(w, reader)
}

type attachmentRow struct {
	id         int64
	name, mime string
	kind       string
}

// claimAttachments checks the ids and hands them to the conversation.
// Only attachments not sent yet (or already in this conversation) count.
func (m *Module) claimAttachments(ctx context.Context, conversationID int64, ids []int64) ([]attachmentRow, error) {
	if len(ids) > maxAttachments {
		return nil, httpx.Invalid("一条消息最多 10 个附件")
	}
	out := make([]attachmentRow, 0, len(ids))
	for _, id := range ids {
		var row attachmentRow
		var owner sql.NullInt64
		err := m.d.DB.QueryRowContext(ctx, "SELECT id,name,mime,kind,conversation_id FROM ai_attachments WHERE id=?", id).Scan(&row.id, &row.name, &row.mime, &row.kind, &owner)
		if errors.Is(err, sql.ErrNoRows) || err == nil && owner.Valid && owner.Int64 != conversationID {
			return nil, httpx.Invalid("附件不存在或已经用在别的对话里")
		}
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	for _, row := range out {
		if _, err := m.d.DB.ExecContext(ctx, "UPDATE ai_attachments SET conversation_id=? WHERE id=?", conversationID, row.id); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// attachmentBlocks are saved in the user message; history turns them back
// into images and text for the model.
func attachmentBlocks(rows []attachmentRow) []map[string]any {
	out := []map[string]any{}
	for _, row := range rows {
		kind := "file"
		if row.kind == "image" {
			kind = "image"
		}
		out = append(out, map[string]any{"type": kind, "attachmentId": row.id, "name": row.name, "mime": row.mime})
	}
	return out
}

// addAttachment puts one saved block into the message for the model.
func (m *Module) addAttachment(ctx context.Context, msg *llm.Message, block map[string]any) {
	id, _ := block["attachmentId"].(float64)
	name := stringValue(block["name"])
	reader, _, err := m.attachmentStore().Get(ctx, attachmentKey(int64(id)))
	if err != nil {
		msg.Content += "（附件 " + name + " 已经找不到了）\n"
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxImageAttachment+1))
	if err != nil {
		return
	}
	if block["type"] == "image" {
		msg.Images = append(msg.Images, llm.Image{MIME: stringValue(block["mime"]), Data: data})
		return
	}
	msg.Content += "附件 " + name + "：\n```\n" + string(data) + "\n```\n"
}

// deleteAttachments removes the stored files after their rows are gone.
func (m *Module) deleteAttachmentFiles(ctx context.Context, ids []int64) {
	for _, id := range ids {
		if err := m.attachmentStore().Delete(ctx, attachmentKey(id)); err != nil {
			m.d.Log.Warn("ai attachment file not deleted", "attachment", id, "err", err)
		}
	}
}

func (m *Module) attachmentIDs(ctx context.Context, query string, args ...any) ([]int64, error) {
	rows, err := m.d.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// cleanupAttachments drops uploads that were never sent.
func (m *Module) cleanupAttachments(ctx context.Context) error {
	cutoff := m.now().UTC().Add(-24 * time.Hour)
	ids, err := m.attachmentIDs(ctx, "SELECT id FROM ai_attachments WHERE conversation_id IS NULL AND created_at<?", cutoff)
	if err != nil {
		return err
	}
	if _, err := m.d.DB.ExecContext(ctx, "DELETE FROM ai_attachments WHERE conversation_id IS NULL AND created_at<?", cutoff); err != nil {
		return err
	}
	m.deleteAttachmentFiles(ctx, ids)
	return nil
}

// modelSeesImages is false only when the model's spec says it has no image
// input; an unknown spec is given the benefit of the doubt.
func (m *Module) modelSeesImages(ctx context.Context) (bool, error) {
	selected, err := m.modelSettings(ctx)
	if err != nil || selected.Agent == nil {
		return true, err
	}
	models, err := m.listModels(ctx, &selected.Agent.ProviderId)
	if err != nil {
		return true, err
	}
	for _, model := range models {
		if model.Id == selected.Agent.Model && model.ImageInput != nil {
			return *model.ImageInput, nil
		}
	}
	return true, nil
}
