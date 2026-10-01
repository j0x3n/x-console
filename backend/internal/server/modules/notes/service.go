package notes

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
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
	out := api.Note{Id: n.ID, Title: n.Title, Body: n.Body, Pinned: n.Pinned != 0, Hidden: hiddenField(n.Hidden), Tags: tags,
		ArchivedAt: n.ArchivedAt, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
	if n.SuggestedTags != nil {
		var suggestions []string
		if json.Unmarshal([]byte(*n.SuggestedTags), &suggestions) == nil && len(suggestions) > 0 {
			out.SuggestedTags = &suggestions
		}
	}
	return out
}

func toSummary(n db.Note, tags []string) api.NoteSummary {
	if tags == nil {
		tags = []string{}
	}
	return api.NoteSummary{Id: n.ID, Title: n.Title, Excerpt: truncate(plainText(n.Body), 160), Pinned: n.Pinned != 0,
		Hidden: hiddenField(n.Hidden), Tags: tags, ArchivedAt: n.ArchivedAt, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func hiddenField(hidden int64) *bool {
	if hidden == 0 {
		return nil
	}
	v := true
	return &v
}

func requireVault(ctx context.Context) error {
	if !auth.VaultUnlocked(ctx) {
		return httpx.NewError(403, "vault_locked", "先解锁隐藏内容")
	}
	return nil
}

func (m *Module) noteRow(ctx context.Context, id int64) (db.Note, error) {
	n, err := m.q.GetNote(ctx, id)
	if err != nil {
		return n, notFound(err)
	}
	if n.Hidden != 0 && !auth.VaultUnlocked(ctx) {
		return n, httpx.ErrNotFound
	}
	return n, nil
}

func (m *Module) getNote(ctx context.Context, id int64) (api.Note, error) {
	n, err := m.noteRow(ctx, id)
	if err != nil {
		return api.Note{}, err
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

// createNote adds a note. quick is the quick-note box (B67): the title and
// tags are made right away, see scheduleNoteAI.
func (m *Module) createNote(ctx context.Context, title, body string, tags []string, pinned, hidden, quick bool) (out api.Note, err error) {
	defer func() { m.d.Audit.Record(ctx, "note.create", strconv.FormatInt(out.Id, 10), nil, err) }()
	if hidden {
		if err = requireVault(ctx); err != nil {
			return out, err
		}
	}
	tags, err = cleanTags(tags)
	if err != nil {
		return out, err
	}
	now := m.now()
	var id int64
	err = m.tx(ctx, func(q *db.Queries) error {
		n, err := q.CreateNote(ctx, db.CreateNoteParams{Title: strings.TrimSpace(title), Body: body,
			Pinned: boolInt(pinned), Hidden: boolInt(hidden), CreatedAt: now, UpdatedAt: now})
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
	if hidden {
		m.d.Audit.Record(ctx, "note.hide", strconv.FormatInt(id, 10), nil, nil)
	}
	m.publishNote("note.created", out)
	m.scheduleNoteAI(id, hidden, quick)
	return out, nil
}

// notePatch changes some fields. Nil means unchanged.
type notePatch struct {
	Title    *string
	Body     *string
	Pinned   *bool
	Hidden   *bool
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
	if p.Hidden != nil {
		if err = requireVault(ctx); err != nil {
			return out, err
		}
	}
	var changedHidden *bool
	err = m.tx(ctx, func(q *db.Queries) error {
		n, err := q.GetNote(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if n.Hidden != 0 && !auth.VaultUnlocked(ctx) {
			return httpx.ErrNotFound
		}
		wasHidden := n.Hidden != 0
		if p.Hidden != nil {
			n.Hidden = boolInt(*p.Hidden)
		}
		now := m.now()
		if !now.After(n.UpdatedAt) {
			now = n.UpdatedAt.Add(time.Millisecond)
		}
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
			ArchivedAt: n.ArchivedAt, Hidden: n.Hidden, UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		if p.Hidden != nil && wasHidden != *p.Hidden {
			v := *p.Hidden
			changedHidden = &v
		}
		if p.Tags != nil {
			if err := setTags(ctx, q, id, tags); err != nil {
				return err
			}
			if n.SuggestedTags != nil {
				var suggestions []string
				if json.Unmarshal([]byte(*n.SuggestedTags), &suggestions) == nil {
					selected := map[string]bool{}
					for _, tag := range tags {
						selected[tag] = true
					}
					remaining := []string{}
					for _, tag := range suggestions {
						if !selected[tag] {
							remaining = append(remaining, tag)
						}
					}
					raw, _ := json.Marshal(remaining)
					if err := q.SetSuggestedTags(ctx, db.SetSuggestedTagsParams{SuggestedTags: new(string(raw)), ID: id}); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	if changedHidden != nil {
		action := "note.hide"
		if !*changedHidden {
			action = "note.restore"
		}
		m.d.Audit.Record(ctx, action, strconv.FormatInt(id, 10), nil, nil)
	}
	out, err = m.getNote(ctx, id)
	if err != nil {
		return out, err
	}
	if changedHidden != nil {
		m.d.Bus.Publish("note.updated", map[string]any{"id": id, "hidden": true})
	} else {
		m.publishNote("note.updated", out)
	}
	if p.Body != nil || p.Title != nil || p.Tags != nil || p.Hidden != nil {
		m.scheduleNoteAI(id, out.Hidden != nil && *out.Hidden, false)
	}
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
	note, err := m.noteRow(ctx, id)
	if err != nil {
		return err
	}
	files, err := m.noteAttachmentIDs(ctx, id)
	if err != nil {
		return err
	}
	n, err := m.q.DeleteNote(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	m.deleteAttachmentFiles(ctx, files) // after the rows, see DeleteNoteAttachment
	if note.Hidden != 0 {
		m.d.Bus.Publish("note.deleted", map[string]any{"id": id, "hidden": true})
	} else {
		m.d.Bus.Publish("note.deleted", map[string]int64{"id": id})
	}
	m.scheduleNoteAI(id, true, false)
	return nil
}

func (m *Module) publishNote(topic string, note api.Note) {
	if note.Hidden != nil && *note.Hidden {
		m.d.Bus.Publish(topic, map[string]any{"id": note.Id, "hidden": true})
		return
	}
	m.d.Bus.Publish(topic, note)
}

func (m *Module) noteChanged(ctx context.Context, id int64) {
	n, err := m.q.GetNote(ctx, id)
	if err == nil && n.Hidden != 0 {
		m.d.Bus.Publish("note.updated", map[string]any{"id": id, "hidden": true})
		return
	}
	m.d.Bus.Publish("note.updated", map[string]int64{"id": id})
}

func (m *Module) tagCounts(ctx context.Context, hidden bool) ([]api.TagCount, error) {
	rows, err := m.q.TagCounts(ctx, boolInt(hidden))
	if err != nil {
		return nil, err
	}
	out := make([]api.TagCount, len(rows))
	for i, r := range rows {
		out[i] = api.TagCount{Tag: r.Tag, Count: int(r.Count)}
		if r.Color != "" {
			out[i].Color = &r.Color
		}
	}
	return out, nil
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// setTagColor saves a tag's color. An empty color goes back to the default.
func (m *Module) setTagColor(ctx context.Context, tag, color string) error {
	tags, err := cleanTags([]string{tag})
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return httpx.Invalid("标签不能为空")
	}
	if color == "" {
		return m.q.ClearTagColor(ctx, tags[0])
	}
	if !hexColor.MatchString(color) {
		return httpx.Invalid("颜色要写成 #cc7752 这样")
	}
	return m.q.SetTagColor(ctx, db.SetTagColorParams{Tag: tags[0], Color: strings.ToLower(color)})
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
	n, err := m.noteRow(ctx, id)
	if err != nil {
		return out, err
	}
	if n.Hidden != 0 {
		return out, httpx.ErrNotFound
	}
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return out, httpx.NewError(501, "feature_unavailable", "项目模块未启用")
	}
	note := toNote(n, nil)
	title := displayTitle(note.Title, note.Body)
	ref, err := issues.Create(ctx, contracts.CreateIssue{ProjectID: projectID, Title: title, Description: note.Body})
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
	n, err := m.noteRow(ctx, id)
	if err != nil {
		return 0, err
	}
	if n.Hidden != 0 && !auth.VaultUnlocked(ctx) {
		return 0, httpx.ErrNotFound
	}
	reminders, ok := module.Lookup[contracts.Reminders](m.d.Registry, contracts.RemindersKey)
	if !ok {
		return 0, httpx.NewError(501, "feature_unavailable", "提醒模块未启用")
	}
	if at.IsZero() {
		return 0, httpx.Invalid("需要提醒时间")
	}
	return reminders.Create(ctx, contracts.CreateReminder{
		Title: displayTitle(n.Title, n.Body), Body: truncate(plainText(n.Body), 200), At: at, RRule: rrule,
		Link: notePath(id),
	})
}
