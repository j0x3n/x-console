package inbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/inbox/db"
)

// create runs the action that makes the object and returns its reference
// (a number, or a key such as XC-12 for a card).
func (m *Module) create(ctx context.Context, in api.InboxFileInput, fallbackProject int64) (string, error) {
	title, body := strings.TrimSpace(deref(in.Title)), strings.TrimSpace(deref(in.Body))
	var name string
	var args map[string]any
	switch in.Kind {
	case api.Reminder:
		at := strings.TrimSpace(deref(in.At))
		if title == "" {
			return "", httpx.Invalid("提醒要有标题")
		}
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			return "", httpx.Invalid("提醒要有时间，格式是 RFC 3339")
		}
		name, args = "reminders.create", map[string]any{"title": title, "at": t.Format(time.RFC3339)}
		if body != "" {
			args["body"] = body
		}
	case api.Card:
		project := int64(0)
		if in.ProjectId != nil {
			project = *in.ProjectId
		}
		if project == 0 {
			project = fallbackProject
		}
		if title == "" {
			return "", httpx.Invalid("卡片要有标题")
		}
		if project <= 0 {
			return "", httpx.Invalid("要选一个项目")
		}
		name, args = "projects.create_issue", map[string]any{"projectId": project, "title": title}
		if body != "" {
			args["description"] = body
		}
	case api.Note:
		if title == "" && body == "" {
			return "", httpx.Invalid("笔记要有标题或内容")
		}
		name, args = "notes.create", map[string]any{"title": title, "body": body}
	case api.Readlater:
		u, ok := onlyURL(deref(in.Url))
		if !ok {
			return "", httpx.Invalid("稍后读要有 http 或 https 开头的网址")
		}
		name, args = "readlater.add", map[string]any{"url": u}
		if title != "" && title != u {
			args["note"] = title
		}
	default:
		return "", httpx.Invalid("先选一个归到哪里")
	}
	raw, _ := json.Marshal(args)
	out, err := m.d.Actions.Run(ctx, name, raw)
	if err != nil {
		return "", err
	}
	return refOf(out), nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// refOf reads the id or key out of what an action returned.
func refOf(out any) string {
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	var v map[string]any
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	for _, k := range []string{"key", "id"} {
		switch x := v[k].(type) {
		case string:
			if x != "" {
				return x
			}
		case float64:
			return fmt.Sprintf("%d", int64(x))
		}
	}
	return ""
}

// file creates the object and closes the item.
func (m *Module) file(ctx context.Context, id int64, in api.InboxFileInput) (db.InboxItem, error) {
	cur, err := m.item(ctx, id)
	if err != nil {
		return cur, err
	}
	if cur.Status != "open" {
		return cur, httpx.NewError(http.StatusConflict, "conflict", "这条已经处理过了")
	}
	ref, err := m.create(ctx, in, m.options(ctx).CardProjectId)
	if err != nil {
		return cur, err
	}
	now := m.now().UTC()
	row, err := m.q.CloseInboxItem(ctx, db.CloseInboxItemParams{Status: "filed", FiledKind: string(in.Kind), FiledRef: ref, ClosedAt: &now, UpdatedAt: now, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		// Filed twice at the same time: the object exists, the item is closed already.
		return cur, httpx.NewError(http.StatusConflict, "conflict", "这条已经处理过了")
	}
	m.d.Audit.Record(ctx, "inbox.file", ref, map[string]any{"id": id, "kind": in.Kind}, err)
	if err == nil {
		m.publish()
	}
	return row, err
}

func (m *Module) FileInboxItem(w http.ResponseWriter, r *http.Request, id api.InboxItemId) {
	var body api.InboxFileInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	row, err := m.file(r.Context(), id, body)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toItem(row))
}
