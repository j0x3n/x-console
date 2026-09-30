package drive

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

func (m *Module) registerActions() {
	register := func(name, title, description, schema string, effect actions.Effect, run func(context.Context, json.RawMessage) (any, error)) {
		m.d.Actions.Register(actions.Action{Name: name, Title: title, Description: description, Input: actions.Schema(schema), Effect: effect, Run: run})
	}
	register("drive.list", "列出云盘文件", "List visible items in a folder. parentId defaults to root.", `{"type":"object","properties":{"parentId":{"type":"integer"}},"additionalProperties":false}`, actions.Read, m.actionList)
	register("drive.search", "搜索云盘文件", "Search visible file and folder names.", `{"type":"object","properties":{"q":{"type":"string"}},"required":["q"],"additionalProperties":false}`, actions.Read, m.actionSearch)
	register("drive.read_text", "读取云盘文本", "Read at most the first 100 KB of a visible text file.", `{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`, actions.Read, m.actionReadText)
	register("drive.create_folder", "新建云盘文件夹", "Create a visible folder.", `{"type":"object","properties":{"parentId":{"type":"integer"},"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`, actions.Write, m.actionCreateFolder)
	register("drive.write_text", "写入云盘文本", "Create or replace a visible UTF-8 text file, up to 1 MB.", `{"type":"object","properties":{"parentId":{"type":"integer"},"name":{"type":"string"},"text":{"type":"string"}},"required":["name","text"],"additionalProperties":false}`, actions.Write, m.actionWriteText)
	register("drive.move", "移动云盘条目", "Move a visible item to a visible folder; parentId 0 means root.", `{"type":"object","properties":{"id":{"type":"integer"},"parentId":{"type":"integer"}},"required":["id","parentId"],"additionalProperties":false}`, actions.Write, m.actionMove)
	register("drive.rename", "重命名云盘条目", "Rename a visible item.", `{"type":"object","properties":{"id":{"type":"integer"},"name":{"type":"string"}},"required":["id","name"],"additionalProperties":false}`, actions.Write, m.actionRename)
	register("drive.delete", "删除云盘条目", "Move a visible item to trash.", `{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`, actions.Write, m.actionDelete)
}
func actionInput(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return httpx.Invalid("参数不正确")
	}
	return nil
}
func (m *Module) actionList(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ParentID int64 `json:"parentId"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	req := httptest.NewRequest(http.MethodGet, "/drive/items", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	var parent *int64
	if in.ParentID > 0 {
		parent = &in.ParentID
	}
	m.ListDriveItems(rec, req, api.ListDriveItemsParams{Parent: parent})
	return actionResult(rec)
}
func (m *Module) actionSearch(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		Q string `json:"q"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	req := httptest.NewRequest(http.MethodGet, "/drive/items", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	m.ListDriveItems(rec, req, api.ListDriveItemsParams{Q: &in.Q})
	return actionResult(rec)
}
func (m *Module) actionReadText(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ID int64 `json:"id"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	item, err := m.visibleRow(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if item.IsDir != 0 || item.TrashedAt != nil || !(strings.HasPrefix(item.Mime, "text/") || strings.Contains(item.Mime, "json") || strings.Contains(item.Mime, "xml")) {
		return nil, httpx.Invalid("只支持文本文件")
	}
	f, _, err := m.store.Get(ctx, blobKey(item.Sha256))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, 100*1024))
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": item.ID, "name": item.Name, "text": string(body), "truncated": item.Size > int64(len(body))}, nil
}
func (m *Module) actionRequest(ctx context.Context, method, url string, body any, handler func(http.ResponseWriter, *http.Request)) (any, error) {
	ctx = auth.WithoutVault(ctx)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(raw)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return actionResult(rec)
}
func actionResult(rec *httptest.ResponseRecorder) (any, error) {
	result := rec.Result()
	defer result.Body.Close()
	if result.StatusCode >= 300 {
		var body struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		_ = json.NewDecoder(result.Body).Decode(&body)
		return nil, httpx.NewError(result.StatusCode, body.Code, body.Message)
	}
	if result.StatusCode == 204 {
		return map[string]bool{"ok": true}, nil
	}
	var out any
	err := json.NewDecoder(result.Body).Decode(&out)
	return out, err
}
func (m *Module) actionCreateFolder(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		ParentID int64  `json:"parentId"`
		Name     string `json:"name"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return m.actionRequest(ctx, http.MethodPost, "/drive/folders", map[string]any{"parentId": in.ParentID, "name": in.Name}, m.CreateDriveFolder)
}
func (m *Module) actionMove(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		ID       int64 `json:"id"`
		ParentID int64 `json:"parentId"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return m.actionRequest(ctx, http.MethodPatch, "/drive/items", map[string]any{"parentId": in.ParentID}, func(w http.ResponseWriter, r *http.Request) { m.UpdateDriveItem(w, r, in.ID) })
}
func (m *Module) actionRename(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return m.actionRequest(ctx, http.MethodPatch, "/drive/items", map[string]any{"name": in.Name}, func(w http.ResponseWriter, r *http.Request) { m.UpdateDriveItem(w, r, in.ID) })
}
func (m *Module) actionDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return m.actionRequest(ctx, http.MethodDelete, "/drive/items", nil, func(w http.ResponseWriter, r *http.Request) {
		m.DeleteDriveItem(w, r, in.ID, api.DeleteDriveItemParams{})
	})
}
func (m *Module) actionWriteText(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ParentID int64  `json:"parentId"`
		Name     string `json:"name"`
		Text     string `json:"text"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	if !validName(in.Name) || len(in.Text) > 1<<20 || !utf8.ValidString(in.Text) {
		return nil, httpx.Invalid("文件名不正确或内容超过 1 MB")
	}
	parent, err := m.parent(ctx, &in.ParentID, false)
	if err != nil {
		return nil, err
	}
	hash, err := m.storeBlob(ctx, []byte(in.Text))
	if err != nil {
		return nil, err
	}
	var id int64
	var oldHash string
	err = m.write(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT id FROM drive_items WHERE parent_id IS ? AND name=? AND hidden=0 AND trashed_at IS NULL", parent, in.Name).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id = 0
		} else if err != nil {
			return err
		}
		now := time.Now().UTC()
		if id == 0 {
			return tx.QueryRowContext(ctx, "INSERT INTO drive_items(parent_id,name,is_dir,size,mime,sha256,hidden,created_at,updated_at) VALUES(?,?,0,?,?,?,?,?,?) RETURNING id", parent, in.Name, len(in.Text), "text/plain; charset=utf-8", hash, 0, now, now).Scan(&id)
		}
		current, err := db.New(tx).GetItem(ctx, id)
		if err != nil {
			return err
		}
		if current.IsDir != 0 {
			return httpx.Invalid("同名条目是文件夹")
		}
		oldHash = current.Sha256
		if err := m.recordVersion(ctx, tx, current, hash); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE drive_items SET size=?,mime=?,sha256=?,updated_at=?,s3_synced_at=NULL WHERE id=?", len(in.Text), "text/plain; charset=utf-8", hash, now, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if oldHash != hash {
		m.dropBlob(ctx, oldHash)
	}
	item, err := m.row(ctx, id)
	if err != nil {
		return nil, err
	}
	m.event("drive_item.updated", item)
	m.audit(ctx, "drive.write_text", id, nil)
	m.triggerSync()
	return m.dto(ctx, item), nil
}
