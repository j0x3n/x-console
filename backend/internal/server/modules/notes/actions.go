package notes

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

func (m *Module) registerActions() {
	m.d.Actions.Register(actions.Action{
		Name:  "notes.search",
		Title: "搜索笔记",
		Description: "Search notes by text (any language; words of three or more characters use the full-text index) " +
			"and optionally a tag. Without q it lists recent notes, pinned first. Returns id, title, excerpt and a snippet.",
		Input: actions.Schema(`{"type":"object","properties":{
			"q":{"type":"string"},
			"tag":{"type":"string"},
			"limit":{"type":"integer","minimum":1,"maximum":50}
		},"additionalProperties":false}`),
		Effect: actions.Read,
		Run:    m.actionSearch,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "notes.create",
		Title:       "新建笔记",
		Description: "Create a Markdown note. Title may be empty. Returns the note with its id.",
		Input: actions.Schema(`{"type":"object","properties":{
			"title":{"type":"string"},
			"body":{"type":"string"},
			"tags":{"type":"array","items":{"type":"string"}}
		},"required":["body"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionCreate,
	})
	m.d.Actions.Register(actions.Action{
		Name:        "notes.append",
		Title:       "追加到笔记",
		Description: "Append Markdown text to the end of an existing note, separated by a blank line. Returns the note.",
		Input: actions.Schema(`{"type":"object","properties":{
			"id":{"type":"integer"},
			"text":{"type":"string"}
		},"required":["id","text"],"additionalProperties":false}`),
		Effect: actions.Write,
		Run:    m.actionAppend,
	})
}

func decodeInput(input json.RawMessage, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return httpx.Invalid("参数不正确: " + err.Error())
	}
	return nil
}

func (m *Module) actionSearch(ctx context.Context, input json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		Q     string `json:"q"`
		Tag   string `json:"tag"`
		Limit int    `json:"limit"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 20
	}
	items, _, err := m.listNotes(auth.WithoutVault(ctx), listFilter{Q: in.Q, Tag: in.Tag, Limit: in.Limit})
	if err != nil {
		return nil, err
	}
	// Plain brackets read better than private-use markers for a model.
	for i := range items {
		if items[i].Snippet != nil {
			s := strings.NewReplacer(markOpen, "[", markClose, "]").Replace(*items[i].Snippet)
			items[i].Snippet = &s
		}
	}
	return items, nil
}

func (m *Module) actionCreate(ctx context.Context, input json.RawMessage) (any, error) {
	var in struct {
		Title string   `json:"title"`
		Body  string   `json:"body"`
		Tags  []string `json:"tags"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Body) == "" {
		return nil, httpx.Invalid("笔记内容不能为空")
	}
	return m.createNote(auth.WithoutVault(ctx), in.Title, in.Body, in.Tags, false, false)
}

func (m *Module) actionAppend(ctx context.Context, input json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ID   int64  `json:"id"`
		Text string `json:"text"`
	}
	if err := decodeInput(input, &in); err != nil {
		return nil, err
	}
	n, err := m.noteRow(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if n.Hidden != 0 {
		return nil, httpx.ErrNotFound
	}
	return m.appendNote(auth.WithoutVault(ctx), in.ID, in.Text)
}
