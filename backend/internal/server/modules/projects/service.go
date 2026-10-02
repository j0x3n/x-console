package projects

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// Business logic shared by the HTTP handlers, the contracts and the actions.
// Methods publish their events after the transaction commits.

// ---- projects ----

// claimFiles runs after the record is saved. A failure is logged, not
// returned: returning it would make the client retry and save twice.
func (m *Module) claimFiles(ctx context.Context, kind string, id int64, markdown string) error {
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		if err := files.Claim(ctx, kind, id, markdown); err != nil {
			m.d.Log.Error("claim images failed", "kind", kind, "id", id, "err", err)
		}
	}
	return nil
}

func (m *Module) deleteOwnedFiles(ctx context.Context, kind string, id int64) error {
	if files, ok := module.Lookup[contracts.Files](m.d.Registry, contracts.FilesKey); ok {
		return files.DeleteOwned(ctx, kind, id)
	}
	return nil
}

func (m *Module) listProjects(ctx context.Context, archived bool) ([]api.Project, error) {
	rows, err := m.q.ListProjects(ctx, archived)
	if err != nil {
		return nil, err
	}
	out := make([]api.Project, len(rows))
	for i, r := range rows {
		out[i] = toProject(r.Project, r.IssueCount, r.OpenCount)
	}
	return out, nil
}

func (m *Module) getProject(ctx context.Context, id int64) (api.Project, error) {
	r, err := m.q.GetProject(ctx, id)
	if err != nil {
		return api.Project{}, notFound(err)
	}
	return toProject(r.Project, r.IssueCount, r.OpenCount), nil
}

// projectIDByKey finds a project id by its key, for example "XC".
func (m *Module) projectIDByKey(ctx context.Context, key string) (int64, error) {
	id, err := m.q.GetProjectIDByKey(ctx, strings.ToUpper(strings.TrimSpace(key)))
	return id, notFound(err)
}

func (m *Module) createProject(ctx context.Context, in api.CreateProject) (out api.Project, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "project.create", in.Key, map[string]any{"name": in.Name}, err)
	}()
	in.Key = strings.ToUpper(strings.TrimSpace(in.Key))
	in.Name = strings.TrimSpace(in.Name)
	if !projectKeyRe.MatchString(in.Key) {
		return out, httpx.Invalid("项目 key 需要 2 到 5 个大写字母")
	}
	if in.Name == "" {
		return out, httpx.Invalid("项目名称不能为空")
	}
	if n, err := m.q.ProjectKeyExists(ctx, in.Key); err != nil {
		return out, err
	} else if n > 0 {
		return out, httpx.NewError(409, "conflict", "项目 key 已经被用了")
	}
	now := m.now()
	id, err := m.q.CreateProject(ctx, db.CreateProjectParams{
		Key: in.Key, Name: in.Name, Description: deref(in.Description), Color: deref(in.Color), Icon: deref(in.Icon),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return out, err
	}
	// B46: every project starts with one board with a list per status.
	if err = m.tx(ctx, func(q *db.Queries) error {
		_, err := m.createBoardTx(ctx, q, id, "看板", "", "statuses")
		return err
	}); err != nil {
		return out, err
	}
	out, err = m.getProject(ctx, id)
	if err != nil {
		return out, err
	}
	if err = m.claimFiles(ctx, "project", id, deref(in.Description)); err != nil {
		return out, err
	}
	m.d.Bus.Publish("project.created", out)
	return out, nil
}

func (m *Module) updateProject(ctx context.Context, id int64, in api.UpdateProject) (out api.Project, err error) {
	defer func() { m.d.Audit.Record(ctx, "project.update", out.Key, map[string]any{"id": id}, err) }()
	r, err := m.q.GetProject(ctx, id)
	if err != nil {
		return out, notFound(err)
	}
	p := r.Project
	wasArchived := p.ArchivedAt != nil
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return out, httpx.Invalid("项目名称不能为空")
		}
		p.Name = name
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.Color != nil {
		p.Color = *in.Color
	}
	if in.Icon != nil {
		p.Icon = *in.Icon
	}
	if in.LayoutLocked != nil {
		p.LayoutLocked = 0
		if *in.LayoutLocked {
			p.LayoutLocked = 1
		}
	}
	now := m.now()
	if in.Archived != nil {
		if *in.Archived && p.ArchivedAt == nil {
			p.ArchivedAt = &now
		} else if !*in.Archived {
			p.ArchivedAt = nil
		}
	}
	if err := m.q.UpdateProject(ctx, db.UpdateProjectParams{
		Name: p.Name, Description: p.Description, Color: p.Color, Icon: p.Icon, ArchivedAt: p.ArchivedAt,
		LayoutLocked: p.LayoutLocked, UpdatedAt: now, ID: id,
	}); err != nil {
		return out, err
	}
	out, err = m.getProject(ctx, id)
	if err != nil {
		return out, err
	}
	if err = m.claimFiles(ctx, "project", id, p.Description); err != nil {
		return out, err
	}
	if !wasArchived && out.ArchivedAt != nil {
		m.d.Bus.Publish("project.archived", out)
	} else {
		m.d.Bus.Publish("project.updated", out)
	}
	return out, nil
}

// ---- issues ----

// issueInput creates an issue. Status defaults to todo.
type issueInput struct {
	Title          string
	Description    string
	Status         string
	Priority       int
	DueDate        *string
	DueAt          *time.Time
	DueRemind      string
	CategoryID     *int64
	MilestoneID    *int64
	LabelIDs       []int64
	ExternalSource string
	ExternalID     string
	// UpdatedAt zero means now. Sync passes the remote time.
	UpdatedAt time.Time
	// BoardID and ListID place the card (B46); nil picks a default.
	BoardID *int64
	ListID  *int64
}

// issuePatch changes some fields of an issue. Nil means unchanged.
type issuePatch struct {
	Title          *string
	Description    *string
	Status         *string
	Priority       *int
	DueDate        *string
	ClearDueDate   bool
	DueAt          *time.Time
	ClearDueAt     bool
	DueRemind      *string
	CategoryID     *int64
	ClearCategory  bool
	MilestoneID    *int64
	ClearMilestone bool
	LabelIDs       *[]int64
	// Color is B85's card color; "" clears it.
	Color *string
	// UpdatedAt zero means now. Sync passes the remote time.
	UpdatedAt time.Time
}

// statusChange is the payload of issue.status_changed.
type statusChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

func validPriority(p int) error {
	if p < 0 || p > 4 {
		return httpx.Invalid("优先级只能是 0 到 4")
	}
	return nil
}

// checkRefs makes sure the milestone and labels can be used in the project.
func checkRefs(ctx context.Context, q *db.Queries, projectID int64, milestoneID *int64, labelIDs []int64) error {
	if milestoneID != nil {
		ms, err := q.GetMilestone(ctx, *milestoneID)
		if err != nil || ms.ProjectID != projectID {
			return httpx.Invalid("里程碑不属于这个项目")
		}
	}
	for _, id := range labelIDs {
		l, err := q.GetLabel(ctx, id)
		if err != nil || (l.ProjectID != nil && *l.ProjectID != projectID) {
			return httpx.Invalid("标签不属于这个项目")
		}
	}
	return nil
}

func checkCategory(ctx context.Context, q *db.Queries, projectID int64, categoryID *int64) error {
	if categoryID == nil {
		return nil
	}
	owner, err := q.GetCategoryProject(ctx, *categoryID)
	if err != nil || owner != projectID {
		return httpx.NewError(400, "invalid_category", "分类不属于这个项目")
	}
	return nil
}

func setLabels(ctx context.Context, q *db.Queries, issueID int64, labelIDs []int64) error {
	if err := q.ClearIssueLabels(ctx, issueID); err != nil {
		return err
	}
	for _, id := range labelIDs {
		if err := q.AddIssueLabel(ctx, db.AddIssueLabelParams{IssueID: issueID, LabelID: id}); err != nil {
			return err
		}
	}
	return nil
}

// completedAt keeps the first completion time while an issue stays closed.
func completedAt(prev *time.Time, from, to string, at time.Time) *time.Time {
	if !closedStatus(to) {
		return nil
	}
	if closedStatus(from) && prev != nil {
		return prev
	}
	return &at
}

func (m *Module) findIssue(ctx context.Context, q *db.Queries, key string) (issueRow, error) {
	pk, n, err := parseIssueKey(key)
	if err != nil {
		return issueRow{}, err
	}
	r, err := q.GetIssueByKey(ctx, db.GetIssueByKeyParams{Key: pk, Number: n})
	if err != nil {
		return issueRow{}, notFound(err)
	}
	return issueRow{r.Issue, r.ProjectKey}, nil
}

func (m *Module) issueByID(ctx context.Context, id int64) (api.Issue, error) {
	r, err := m.q.GetIssue(ctx, id)
	if err != nil {
		return api.Issue{}, notFound(err)
	}
	list, err := toIssues(ctx, m.q, []issueRow{{r.Issue, r.ProjectKey}})
	if err != nil {
		return api.Issue{}, err
	}
	return list[0], nil
}

func (m *Module) getIssue(ctx context.Context, key string) (api.Issue, error) {
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return api.Issue{}, err
	}
	list, err := toIssues(ctx, m.q, []issueRow{r})
	if err != nil {
		return api.Issue{}, err
	}
	return list[0], nil
}

// insertIssue creates an issue without publishing events.
func (m *Module) insertIssue(ctx context.Context, projectID int64, in issueInput) (api.Issue, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return api.Issue{}, httpx.Invalid("标题不能为空")
	}
	if in.Status == "" {
		in.Status = "todo"
	}
	if !validStatus(in.Status) {
		return api.Issue{}, httpx.Invalid("状态不正确")
	}
	if err := validPriority(in.Priority); err != nil {
		return api.Issue{}, err
	}
	if in.DueDate != nil {
		if err := validDate(*in.DueDate); err != nil {
			return api.Issue{}, err
		}
	}
	if in.DueAt == nil && in.DueDate != nil {
		var err error
		in.DueAt, err = m.dueAtForDate(*in.DueDate)
		if err != nil {
			return api.Issue{}, err
		}
	}
	if in.DueAt != nil {
		in.DueDate = m.dateOf(in.DueAt)
	}
	if in.DueRemind == "" {
		in.DueRemind = "at_due"
	}
	if !validDueRemind(in.DueRemind) {
		return api.Issue{}, httpx.Invalid("到期提醒设置无效")
	}
	now := m.now()
	updated := in.UpdatedAt.UTC()
	if in.UpdatedAt.IsZero() {
		updated = now
	}
	created := now
	if updated.Before(created) {
		created = updated
	}
	var id int64
	err := m.tx(ctx, func(q *db.Queries) error {
		p, err := q.GetProject(ctx, projectID)
		if err != nil {
			return notFound(err)
		}
		if p.Project.ArchivedAt != nil {
			return httpx.Invalid("项目已归档")
		}
		if err := checkRefs(ctx, q, projectID, in.MilestoneID, in.LabelIDs); err != nil {
			return err
		}
		if err := checkCategory(ctx, q, projectID, in.CategoryID); err != nil {
			return err
		}
		list, err := m.placeFor(ctx, q, projectID, in.BoardID, in.ListID, in.Status)
		if err != nil {
			return err
		}
		if list.Status != nil && in.ListID != nil {
			in.Status = *list.Status
		}
		next, err := q.TakeIssueNumber(ctx, projectID)
		if err != nil {
			return err
		}
		sort, err := topOfList(ctx, q, list.ID, 0)
		if err != nil {
			return err
		}
		var completed *time.Time
		if closedStatus(in.Status) {
			completed = &updated
		}
		id, err = q.InsertIssue(ctx, db.InsertIssueParams{
			ProjectID: projectID, Number: next - 1, Title: in.Title, Description: in.Description, Status: in.Status,
			Priority: int64(in.Priority), DueDate: in.DueDate, MilestoneID: in.MilestoneID, SortOrder: sort,
			ExternalSource: in.ExternalSource, ExternalID: in.ExternalID,
			CreatedAt: created, UpdatedAt: updated, CompletedAt: completed,
			CategoryID: in.CategoryID, DueAt: dueString(in.DueAt), DueRemind: in.DueRemind,
			BoardID: &list.BoardID, ListID: &list.ID,
		})
		if err != nil {
			return err
		}
		if err := activity(ctx, q, id, "created", map[string]any{"listId": list.ID, "toList": list.Name}, now); err != nil {
			return err
		}
		return setLabels(ctx, q, id, in.LabelIDs)
	})
	if err != nil {
		return api.Issue{}, err
	}
	return m.issueByID(ctx, id)
}

func (m *Module) createIssue(ctx context.Context, projectID int64, in issueInput) (out api.Issue, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "issue.create", out.Key, map[string]any{"projectId": projectID, "title": in.Title}, err)
	}()
	out, err = m.insertIssue(ctx, projectID, in)
	if err != nil {
		return out, err
	}
	if err = m.claimFiles(ctx, "issue", out.Id, in.Description); err != nil {
		return out, err
	}
	m.d.Bus.Publish("issue.created", out)
	return out, nil
}

// patchIssue applies a patch without publishing events. It returns the
// status before the change.
func (m *Module) patchIssue(ctx context.Context, key string, p issuePatch) (api.Issue, string, error) {
	var id int64
	var from string
	err := m.tx(ctx, func(q *db.Queries) error {
		row, err := m.findIssue(ctx, q, key)
		if err != nil {
			return err
		}
		i := row.Issue
		id, from = i.ID, i.Status
		if p.Title != nil {
			title := strings.TrimSpace(*p.Title)
			if title == "" {
				return httpx.Invalid("标题不能为空")
			}
			i.Title = title
		}
		if p.Description != nil {
			i.Description = *p.Description
		}
		if p.Priority != nil {
			if err := validPriority(*p.Priority); err != nil {
				return err
			}
			i.Priority = int64(*p.Priority)
		}
		if p.ClearDueAt || p.DueAt != nil {
			i.DueAt = dueString(p.DueAt)
			i.DueDate = m.dateOf(p.DueAt)
			i.DueNotifiedAt = nil
		} else if p.ClearDueDate {
			i.DueDate, i.DueAt, i.DueNotifiedAt = nil, nil, nil
		} else if p.DueDate != nil {
			if err := validDate(*p.DueDate); err != nil {
				return err
			}
			i.DueDate = p.DueDate
			when, err := m.dueAtForDate(*p.DueDate)
			if err != nil {
				return err
			}
			i.DueAt, i.DueNotifiedAt = dueString(when), nil
		}
		if p.DueRemind != nil {
			if !validDueRemind(*p.DueRemind) {
				return httpx.Invalid("到期提醒设置无效")
			}
			i.DueRemind, i.DueNotifiedAt = *p.DueRemind, nil
		}
		if p.ClearCategory {
			i.CategoryID = nil
		} else if p.CategoryID != nil {
			if err := checkCategory(ctx, q, i.ProjectID, p.CategoryID); err != nil {
				return err
			}
			i.CategoryID = p.CategoryID
		}
		if p.ClearMilestone {
			i.MilestoneID = nil
		} else if p.MilestoneID != nil {
			if err := checkRefs(ctx, q, i.ProjectID, p.MilestoneID, nil); err != nil {
				return err
			}
			i.MilestoneID = p.MilestoneID
		}
		if p.LabelIDs != nil {
			if err := checkRefs(ctx, q, i.ProjectID, nil, *p.LabelIDs); err != nil {
				return err
			}
		}
		if p.Color != nil && !api.CardColor(*p.Color).Valid() {
			return httpx.Invalid("卡片颜色不正确")
		}
		updated := p.UpdatedAt.UTC()
		if p.UpdatedAt.IsZero() {
			updated = m.now()
		}
		statusChanged := false
		if p.Status != nil && *p.Status != i.Status {
			if !validStatus(*p.Status) {
				return httpx.Invalid("状态不正确")
			}
			i.CompletedAt = completedAt(i.CompletedAt, i.Status, *p.Status, updated)
			i.Status = *p.Status
			statusChanged = true
		}
		if err := q.UpdateIssue(ctx, db.UpdateIssueParams{
			Title: i.Title, Description: i.Description, Status: i.Status, Priority: i.Priority, DueDate: i.DueDate,
			MilestoneID: i.MilestoneID, SortOrder: i.SortOrder, UpdatedAt: updated, CompletedAt: i.CompletedAt, ID: i.ID,
			CategoryID: i.CategoryID, DueAt: i.DueAt, DueRemind: i.DueRemind, DueNotifiedAt: i.DueNotifiedAt,
		}); err != nil {
			return err
		}
		if p.Color != nil && *p.Color != i.Color {
			if err := q.SetIssueColor(ctx, db.SetIssueColorParams{Color: *p.Color, UpdatedAt: updated, ID: i.ID}); err != nil {
				return err
			}
			if err := activity(ctx, q, i.ID, "color", map[string]any{"color": *p.Color}, updated); err != nil {
				return err
			}
		}
		if statusChanged {
			// B46: the card follows its status into the matching list.
			if err := m.followStatus(ctx, q, i.ID, i.Status); err != nil {
				return err
			}
		}
		if p.LabelIDs != nil {
			return setLabels(ctx, q, i.ID, *p.LabelIDs)
		}
		return nil
	})
	if err != nil {
		return api.Issue{}, "", err
	}
	out, err := m.issueByID(ctx, id)
	return out, from, err
}

// publishUpdate sends issue.updated and, if the status changed, issue.status_changed.
func (m *Module) publishUpdate(issue api.Issue, from string) {
	m.d.Bus.Publish("issue.updated", issue)
	if from != string(issue.Status) {
		m.d.Bus.Publish("issue.status_changed", statusChange{Key: issue.Key, From: from, To: string(issue.Status)})
	}
}

func (m *Module) updateIssue(ctx context.Context, key string, p issuePatch) (out api.Issue, err error) {
	defer func() { m.d.Audit.Record(ctx, "issue.update", key, nil, err) }()
	out, from, err := m.patchIssue(ctx, key, p)
	if err != nil {
		return out, err
	}
	if err = m.claimFiles(ctx, "issue", out.Id, out.Description); err != nil {
		return out, err
	}
	m.publishUpdate(out, from)
	return out, nil
}

func (m *Module) deleteIssue(ctx context.Context, key string) (err error) {
	defer func() { m.d.Audit.Record(ctx, "issue.delete", key, nil, err) }()
	issue, err := m.getIssue(ctx, key)
	if err != nil {
		return err
	}
	comments, err := m.listComments(ctx, key)
	if err != nil {
		return err
	}
	if err := m.q.DeleteIssue(ctx, issue.Id); err != nil {
		return err
	}
	if err := m.deleteOwnedFiles(ctx, "issue", issue.Id); err != nil {
		return err
	}
	for _, comment := range comments {
		if err := m.deleteOwnedFiles(ctx, "comment", comment.Id); err != nil {
			return err
		}
	}
	m.d.Bus.Publish("issue.deleted", issue)
	return nil
}

// ---- comments and links ----

// issueEvent is the payload of issue_comment.* and issue_link.* events.
type issueEvent struct {
	IssueKey string `json:"issueKey"`
	Data     any    `json:"data"`
}

func (m *Module) listComments(ctx context.Context, key string) ([]api.Comment, error) {
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return nil, err
	}
	rows, err := m.q.ListComments(ctx, r.Issue.ID)
	if err != nil {
		return nil, err
	}
	out := make([]api.Comment, len(rows))
	for i, c := range rows {
		out[i] = toComment(c)
	}
	return out, nil
}

func (m *Module) createComment(ctx context.Context, key, body string) (out api.Comment, err error) {
	defer func() { m.d.Audit.Record(ctx, "issue_comment.create", key, nil, err) }()
	if strings.TrimSpace(body) == "" {
		return out, httpx.Invalid("评论不能为空")
	}
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return out, err
	}
	c, err := m.q.CreateComment(ctx, db.CreateCommentParams{IssueID: r.Issue.ID, Body: body, CreatedAt: m.now()})
	if err != nil {
		return out, err
	}
	out = toComment(c)
	if err = m.claimFiles(ctx, "comment", out.Id, body); err != nil {
		return out, err
	}
	m.d.Bus.Publish("issue_comment.created", issueEvent{IssueKey: issueKey(r.ProjectKey, r.Issue.Number), Data: out})
	return out, nil
}

func (m *Module) deleteComment(ctx context.Context, key string, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "issue_comment.delete", key, map[string]any{"id": id}, err) }()
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return err
	}
	n, err := m.q.DeleteComment(ctx, db.DeleteCommentParams{ID: id, IssueID: r.Issue.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	if err := m.deleteOwnedFiles(ctx, "comment", id); err != nil {
		return err
	}
	m.d.Bus.Publish("issue_comment.deleted", issueEvent{IssueKey: issueKey(r.ProjectKey, r.Issue.Number), Data: map[string]int64{"id": id}})
	return nil
}

func (m *Module) listLinks(ctx context.Context, key string) ([]api.IssueLink, error) {
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return nil, err
	}
	rows, err := m.q.ListLinks(ctx, r.Issue.ID)
	if err != nil {
		return nil, err
	}
	out := make([]api.IssueLink, len(rows))
	for i, l := range rows {
		out[i] = toLink(l)
	}
	return out, nil
}

// createLink attaches a link. The same kind and url is stored only once.
func (m *Module) createLink(ctx context.Context, key string, in api.CreateIssueLink) (out api.IssueLink, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "issue_link.create", key, map[string]any{"kind": in.Kind, "url": in.Url}, err)
	}()
	if !in.Kind.Valid() {
		return out, httpx.Invalid("关联类型不正确")
	}
	in.Url = strings.TrimSpace(in.Url)
	if in.Url == "" {
		return out, httpx.Invalid("链接不能为空")
	}
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return out, err
	}
	existing, err := m.q.FindLink(ctx, db.FindLinkParams{IssueID: r.Issue.ID, Kind: string(in.Kind), Url: in.Url})
	if err == nil {
		return toLink(existing), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	title := strings.TrimSpace(deref(in.Title))
	if title == "" {
		title = in.Url
	}
	l, err := m.q.CreateLink(ctx, db.CreateLinkParams{
		IssueID: r.Issue.ID, Kind: string(in.Kind), Title: title, Url: in.Url, Ref: deref(in.Ref), CreatedAt: m.now(),
	})
	if err != nil {
		return out, err
	}
	out = toLink(l)
	m.d.Bus.Publish("issue_link.created", issueEvent{IssueKey: issueKey(r.ProjectKey, r.Issue.Number), Data: out})
	return out, nil
}

func (m *Module) deleteLink(ctx context.Context, key string, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "issue_link.delete", key, map[string]any{"id": id}, err) }()
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return err
	}
	n, err := m.q.DeleteLink(ctx, db.DeleteLinkParams{ID: id, IssueID: r.Issue.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return httpx.ErrNotFound
	}
	m.d.Bus.Publish("issue_link.deleted", issueEvent{IssueKey: issueKey(r.ProjectKey, r.Issue.Number), Data: map[string]int64{"id": id}})
	return nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
