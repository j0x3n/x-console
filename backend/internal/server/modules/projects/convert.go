package projects

import (
	"context"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

func toProject(p db.Project, issueCount, openCount int64) api.Project {
	locked := p.LayoutLocked != 0
	return api.Project{
		Id: p.ID, Key: p.Key, Name: p.Name, Description: p.Description, Color: p.Color, Icon: p.Icon,
		ArchivedAt: p.ArchivedAt, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
		IssueCount: int(issueCount), OpenCount: int(openCount), LayoutLocked: &locked,
	}
}

func toLabel(l db.Label) api.Label {
	return api.Label{Id: l.ID, ProjectId: l.ProjectID, Name: l.Name, Color: l.Color}
}

func toMilestone(ms db.Milestone) api.Milestone {
	return api.Milestone{Id: ms.ID, ProjectId: ms.ProjectID, Name: ms.Name, DueDate: toDate(ms.DueDate), CreatedAt: ms.CreatedAt}
}

func toComment(c db.IssueComment) api.Comment {
	out := api.Comment{Id: c.ID, IssueId: c.IssueID, Body: c.Body, CreatedAt: c.CreatedAt}
	if c.Author != "" {
		author := c.Author
		out.Author = &author
	}
	return out
}

func toLink(l db.IssueLink) api.IssueLink {
	return api.IssueLink{Id: l.ID, IssueId: l.IssueID, Kind: api.IssueLinkKind(l.Kind), Title: l.Title, Url: l.Url,
		Ref: l.Ref, CreatedAt: l.CreatedAt}
}

// toDate converts a stored YYYY-MM-DD into the API date type.
func toDate(s *string) *openapi_types.Date {
	if s == nil {
		return nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil
	}
	return &openapi_types.Date{Time: t}
}

// fromDate converts an API date into the stored YYYY-MM-DD.
func fromDate(d *openapi_types.Date) *string {
	if d == nil {
		return nil
	}
	s := d.Format(dateLayout)
	return &s
}

func toIssue(i db.Issue, projectKey string, labels []api.Label) api.Issue {
	if labels == nil {
		labels = []api.Label{}
	}
	remind := api.DueRemind(i.DueRemind)
	color := api.CardColor(i.Color)
	return api.Issue{
		Id: i.ID, Key: issueKey(projectKey, i.Number), ProjectId: i.ProjectID, ProjectKey: projectKey, Number: i.Number,
		Title: i.Title, Description: i.Description, Status: api.IssueStatus(i.Status), Priority: int(i.Priority),
		DueDate: toDate(i.DueDate), MilestoneId: i.MilestoneID, SortOrder: i.SortOrder, Labels: labels,
		CategoryId: i.CategoryID, DueAt: parseDue(i.DueAt), DueRemind: &remind,
		ExternalSource: i.ExternalSource, ExternalId: i.ExternalID,
		CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, CompletedAt: i.CompletedAt,
		BoardId: i.BoardID, ListId: i.ListID, ArchivedAt: i.ArchivedAt,
		Color: &color,
	}
}

// labelsFor loads the labels of many issues at once.
func labelsFor(ctx context.Context, q *db.Queries, ids []int64) (map[int64][]api.Label, error) {
	out := map[int64][]api.Label{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListLabelsForIssues(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.IssueID] = append(out[r.IssueID], toLabel(r.Label))
	}
	return out, nil
}

// issueRow is an issue with its project key, as every issue query returns it.
type issueRow struct {
	Issue      db.Issue
	ProjectKey string
}

// toIssues converts rows and attaches their labels.
func toIssues(ctx context.Context, q *db.Queries, rows []issueRow) ([]api.Issue, error) {
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.Issue.ID
	}
	labels, err := labelsFor(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	progress := map[int64][2]int{}
	if len(ids) > 0 {
		rows, err := q.ChecklistProgressForIssues(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			done := 0
			if row.Done != nil {
				done = int(*row.Done)
			}
			progress[row.IssueID] = [2]int{done, int(row.Total)}
		}
	}
	members := map[int64][]api.IssueMember{}
	comments := map[int64]int{}
	if len(ids) > 0 {
		rows, err := q.MembersForIssues(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			members[row.IssueID] = append(members[row.IssueID], api.IssueMember{Kind: api.IssueMemberKind(row.MemberKind), Id: row.MemberID})
		}
		counts, err := q.CommentCountsForIssues(ctx, ids)
		if err != nil {
			return nil, err
		}
		for _, c := range counts {
			comments[c.IssueID] = int(c.N)
		}
	}
	out := make([]api.Issue, len(rows))
	for i, r := range rows {
		out[i] = toIssue(r.Issue, r.ProjectKey, labels[r.Issue.ID])
		counts := progress[r.Issue.ID]
		out[i].ChecklistDone = &counts[0]
		out[i].ChecklistTotal = &counts[1]
		ms := members[r.Issue.ID]
		if ms == nil {
			ms = []api.IssueMember{}
		}
		n := comments[r.Issue.ID]
		out[i].Members, out[i].CommentCount = &ms, &n
	}
	return out, nil
}

// localDate turns a stored YYYY-MM-DD into midnight in loc.
func localDate(s *string, loc *time.Location) *time.Time {
	if s == nil {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	t, err := time.ParseInLocation(dateLayout, *s, loc)
	if err != nil {
		return nil
	}
	return &t
}

func (m *Module) toRef(i db.Issue, projectKey string) contracts.IssueRef {
	return contracts.IssueRef{
		ID: i.ID, Key: issueKey(projectKey, i.Number), ProjectID: i.ProjectID, Title: i.Title,
		Description: i.Description, Status: i.Status, Priority: int(i.Priority),
		DueDate: localDate(i.DueDate, m.d.Config.Location),
	}
}

func (m *Module) toSync(i db.Issue, projectKey string) contracts.SyncIssue {
	return contracts.SyncIssue{
		ProjectID: i.ProjectID, Key: issueKey(projectKey, i.Number), Title: i.Title, Description: i.Description,
		Status: i.Status, Priority: int(i.Priority), DueDate: localDate(i.DueDate, m.d.Config.Location),
		ExternalSource: i.ExternalSource, ExternalID: i.ExternalID, UpdatedAt: i.UpdatedAt,
	}
}
