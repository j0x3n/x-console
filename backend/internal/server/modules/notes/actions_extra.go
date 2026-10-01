package notes

import (
	"context"
	"encoding/json"
	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
)

func (m *Module) registerExtraActions() {
	m.d.Actions.Register(actions.Action{Name: "notes.get", Title: "查看笔记", Description: "Read a note by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), Effect: actions.Read, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		return m.getNote(auth.WithoutVault(ctx), in.ID)
	}})
	m.d.Actions.Register(actions.Action{Name: "notes.update", Title: "修改笔记", Description: "Update the title, body or tags of a note.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},"title":{"type":"string"},"body":{"type":"string"},"tags":{"type":"array","items":{"type":"string"}},"kind":{"type":"string","enum":["note","memo"]}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID          int64 `json:"id"`
			Title, Body *string
			Tags        *[]string
			Kind        *api.NoteKind
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		return m.updateNote(auth.WithoutVault(ctx), in.ID, notePatch{Title: in.Title, Body: in.Body, Tags: in.Tags, Kind: in.Kind})
	}})
	m.d.Actions.Register(actions.Action{Name: "notes.delete", Title: "删除笔记", Description: "Delete a note by id.", Input: actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"]}`), Effect: actions.Write, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in struct {
			ID int64 `json:"id"`
		}
		if err := decodeInput(raw, &in); err != nil {
			return nil, err
		}
		err := m.deleteNote(auth.WithoutVault(ctx), in.ID)
		return map[string]any{"deleted": err == nil}, err
	}})
}
