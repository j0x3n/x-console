package notes

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/db"
)

// Business logic shared by the HTTP handlers, the contract and the actions.

func toNote(n db.Note, tags []string) api.Note {
	if tags == nil {
		tags = []string{}
	}
	return api.Note{Id: n.ID, Title: n.Title, Body: n.Body, Pinned: n.Pinned != 0, Tags: tags,
		ArchivedAt: n.ArchivedAt, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

func toSummary(n db.Note, tags []string) api.NoteSummary {
	if tags == nil {
		tags = []string{}
	}
	return api.NoteSummary{Id: n.ID, Title: n.Title, Excerpt: truncate(plainText(n.Body), 160), Pinned: n.Pinned != 0,
		Tags: tags, ArchivedAt: n.ArchivedAt, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (m *Module) getNote(ctx context.Context, id int64) (api.Note, error) {
	n, err := m.q.GetNote(ctx, id)
	if err != nil {
		return api.Note{}, notFound(err)
	}
	tags, err := m.q.ListNoteTags(ctx, id)
	if err != nil {
		return api.Note{}, err
	}
	return toNote(n, tags), nil
}

func setTags(ctx context.Context, q *db.Queries, id int64, tags []string) error {
	if err := q.ClearNoteTags(ctx, id); err != nil {
		return err
	}
	for _, t := range tags {
		if err := q.AddNoteTag(ctx, db.AddNoteTagParams{NoteID: id, Tag: t}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) createNote(ctx context.Context, title, body string, tags []string, pinned bool) (out api.Note, err error) {
	defer func() { m.d.Audit.Record(ctx, "note.create", strconv.FormatInt(out.Id, 10), nil, err) }()
	tags, err = cleanTags(tags)
	if err != nil {
		return out, err
	}
	now := m.now()
	var id int64
	err = m.tx(ctx, func(q *db.Queries) error {
		n, err := q.CreateNote(ctx, db.CreateNoteParams{Title: strings.TrimSpace(title), Body: body,
			Pinned: boolInt(pinned), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		id = n.ID
		return setTags(ctx, q, id, tags)
	})
	if err != nil {
		return out, err
	}
	out, err = m.getNote(ctx, id)
	if err != nil {
		return out, err
	}
	m.d.Bus.Publish("note.created", out)
	return out, nil
}

// notePatch changes some fields. Nil means unchanged.
type notePatch struct {
	Title    *string
	Body     *string
	Pinned   *bool
	Archived *bool
	Tags     *[]string
}

func (m *Module) updateNote(ctx context.Context, id int64, p notePatch) (out api.Note, err error) {
	defer func() { m.d.Audit.Record(ctx, "note.update", strconv.FormatInt(id, 10), nil, err) }()
	var tags []string
	if p.Tags != nil {
		if tags, err = cleanTags(*p.Tags); err != nil {
			return out, err
		}
	}
	err = m.tx(ctx, func(q *db.Queries) error {
		n, err := q.GetNote(ctx, id)
		if err != nil {
			return notFound(err)
		}
		now := m.now()
		if p.Title != nil {
			n.Title = strings.TrimSpace(*p.Title)
		}
		if p.Body != nil {
			n.Body = *p.Body
		}
		if p.Pinned != nil {
			n.Pinned = boolInt(*p.Pinned)
		}
		if p.Archived != nil {
			if *p.Archived && n.ArchivedAt == nil {
				n.ArchivedAt = &now
			} else if !*p.Archived {
				n.ArchivedAt = nil
			}
		}
		if err := q.UpdateNote(ctx, db.UpdateNoteParams{Title: n.Title, Body: n.Body, Pinned: n.Pinned,
			ArchivedAt: n.ArchivedAt, UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		if p.Tags != nil {
			return setTags(ctx, q, id, tags)
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out, err = m.getNote(ctx, id)
	if err != nil {
		return out, err
	}
	m.d.Bus.Publish("note.updated", out)
	return out, nil
}

// appendNote adds text at the end of a note, separated by a blank line.
func (m *Module) appendNote(ctx context.Context, id int64, text string) (api.Note, error) {
	if strings.TrimSpace(text) == "" {
		return api.Note{}, httpx.Invalid("追加的内容不能为空")
	}
	n, err := m.getNote(ctx, id)
	if err != nil {
		return api.Note{}, err
	}
	body := strings.TrimRight(n.Body, "\n")
	if body != "" {
		body += "\n\n"
	}
	body += text
	return m.updateNote(ctx, id, notePatch{Body: &body})
}

func (m *Module) deleteNote(ctx context.Context, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "note.delete", strconv.FormatInt(id, 10), nil, err) }()
	n, err := m.q.DeleteNote(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	m.d.Bus.Publish("note.deleted", map[string]int64{"id": id})
	return nil
}

func (m *Module) tagCounts(ctx context.Context) ([]api.TagCount, error) {
	rows, err := m.q.TagCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.TagCount, len(rows))
	for i, r := range rows {
		out[i] = api.TagCount{Tag: r.Tag, Count: int(r.Count)}
	}
	return out, nil
}

// toIssueResult is the answer of noteToIssue.
type toIssueResult struct {
	IssueKey string   `json:"issueKey"`
	IssueURL string   `json:"issueUrl"`
	Note     api.Note `json:"note"`
}

// noteToIssue creates an issue from a note, links the note on the issue and
// writes the issue link at the end of the note.
func (m *Module) noteToIssue(ctx context.Context, id, projectID int64) (out toIssueResult, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "note.to_issue", strconv.FormatInt(id, 10), map[string]any{"issue": out.IssueKey}, err)
	}()
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return out, httpx.NewError(501, "feature_unavailable", "项目模块未启用")
	}
	n, err := m.getNote(ctx, id)
	if err != nil {
		return out, err
	}
	title := displayTitle(n.Title, n.Body)
	ref, err := issues.Create(ctx, contracts.CreateIssue{ProjectID: projectID, Title: title, Description: n.Body})
	if err != nil {
		return out, err
	}
	if err := issues.AttachLink(ctx, ref.Key, contracts.IssueLink{Kind: "note", Title: title, URL: notePath(id),
		Ref: strconv.FormatInt(id, 10)}); err != nil {
		return out, err
	}
	out.IssueKey, out.IssueURL = ref.Key, issuePath(ref.Key)
	out.Note, err = m.appendNote(ctx, id, fmt.Sprintf("关联 Issue：[%s %s](%s)", ref.Key, ref.Title, out.IssueURL))
	return out, err
}

// noteToReminder creates a reminder that links back to the note.
func (m *Module) noteToReminder(ctx context.Context, id int64, at time.Time, rrule string) (reminderID int64, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "note.to_reminder", strconv.FormatInt(id, 10), map[string]any{"reminderId": reminderID}, err)
	}()
	reminders, ok := module.Lookup[contracts.Reminders](m.d.Registry, contracts.RemindersKey)
	if !ok {
		return 0, httpx.NewError(501, "feature_unavailable", "提醒模块未启用")
	}
	if at.IsZero() {
		return 0, httpx.Invalid("需要提醒时间")
	}
	n, err := m.getNote(ctx, id)
	if err != nil {
		return 0, err
	}
	return reminders.Create(ctx, contracts.CreateReminder{
		Title: displayTitle(n.Title, n.Body), Body: truncate(plainText(n.Body), 200), At: at, RRule: rrule,
		Link: notePath(id),
	})
}
