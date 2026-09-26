package projects

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// issuesService implements contracts.Issues.
type issuesService struct{ m *Module }

var _ contracts.Issues = (*issuesService)(nil)

func (s *issuesService) Get(ctx context.Context, key string) (contracts.IssueRef, error) {
	r, err := s.m.findIssue(ctx, s.m.q, key)
	if err != nil {
		return contracts.IssueRef{}, err
	}
	return s.m.toRef(r.Issue, r.ProjectKey), nil
}

func (s *issuesService) Create(ctx context.Context, in contracts.CreateIssue) (contracts.IssueRef, error) {
	issue, err := s.m.createIssue(ctx, in.ProjectID, issueInput{
		Title: in.Title, Description: in.Description, Priority: in.Priority, DueDate: s.m.dateOf(in.DueDate),
	})
	if err != nil {
		return contracts.IssueRef{}, err
	}
	return s.Get(ctx, issue.Key)
}

func (s *issuesService) SetStatus(ctx context.Context, key, status string) error {
	_, err := s.m.updateIssue(ctx, key, issuePatch{Status: &status})
	return err
}

func (s *issuesService) AttachLink(ctx context.Context, key string, link contracts.IssueLink) error {
	title, ref := link.Title, link.Ref
	_, err := s.m.createLink(ctx, key, api.CreateIssueLink{
		Kind: api.IssueLinkKind(link.Kind), Title: &title, Url: link.URL, Ref: &ref,
	})
	return err
}

func (s *issuesService) ListDue(ctx context.Context, until time.Time) ([]contracts.IssueRef, error) {
	day := s.m.dateOf(&until)
	rows, err := s.m.q.ListDue(ctx, day)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.IssueRef, len(rows))
	for i, r := range rows {
		out[i] = s.m.toRef(r.Issue, r.ProjectKey)
	}
	return out, nil
}

// dateOf turns a time into the user's local YYYY-MM-DD.
func (m *Module) dateOf(t *time.Time) *string {
	if t == nil || t.IsZero() {
		return nil
	}
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.Local
	}
	s := t.In(loc).Format(dateLayout)
	return &s
}

// syncService implements contracts.IssueSync. Changes it makes publish
// "issue.synced" (data: Issue) instead of issue.created/updated, so a sync
// that listens to issue.* never pushes its own change back.
type syncService struct{ m *Module }

var _ contracts.IssueSync = (*syncService)(nil)

func (s *syncService) FindByExternal(ctx context.Context, source, externalID string) (contracts.SyncIssue, bool, error) {
	r, err := s.m.q.GetIssueByExternal(ctx, db.GetIssueByExternalParams{ExternalSource: source, ExternalID: externalID})
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.SyncIssue{}, false, nil
	}
	if err != nil {
		return contracts.SyncIssue{}, false, err
	}
	return s.m.toSync(r.Issue, r.ProjectKey), true, nil
}

func (s *syncService) Upsert(ctx context.Context, in contracts.SyncIssue) (out contracts.SyncIssue, err error) {
	if in.ExternalSource == "" || in.ExternalID == "" {
		return out, httpx.Invalid("缺少外部来源或外部编号")
	}
	defer func() {
		s.m.d.Audit.Record(ctx, "issue.sync", out.Key, map[string]any{"source": in.ExternalSource, "externalId": in.ExternalID}, err)
	}()
	if in.Status == "" {
		in.Status = "todo"
	}
	if in.UpdatedAt.IsZero() {
		in.UpdatedAt = s.m.now()
	}
	existing, found, err := s.FindByExternal(ctx, in.ExternalSource, in.ExternalID)
	if err != nil {
		return out, err
	}
	var issue api.Issue
	if found {
		p := issuePatch{
			Title: &in.Title, Description: &in.Description, Status: &in.Status, Priority: &in.Priority,
			DueDate: s.m.dateOf(in.DueDate), ClearDueDate: in.DueDate == nil, UpdatedAt: in.UpdatedAt,
		}
		if issue, _, err = s.m.patchIssue(ctx, existing.Key, p); err != nil {
			return out, err
		}
	} else {
		issue, err = s.m.insertIssue(ctx, in.ProjectID, issueInput{
			Title: in.Title, Description: in.Description, Status: in.Status, Priority: in.Priority,
			DueDate: s.m.dateOf(in.DueDate), ExternalSource: in.ExternalSource, ExternalID: in.ExternalID,
			UpdatedAt: in.UpdatedAt,
		})
		if err != nil {
			return out, err
		}
	}
	s.m.d.Bus.Publish("issue.synced", issue)
	r, err := s.m.findIssue(ctx, s.m.q, issue.Key)
	if err != nil {
		return out, err
	}
	return s.m.toSync(r.Issue, r.ProjectKey), nil
}

func (s *syncService) Link(ctx context.Context, key, source, externalID string) (err error) {
	defer func() {
		s.m.d.Audit.Record(ctx, "issue.sync_link", key, map[string]any{"source": source, "externalId": externalID}, err)
	}()
	source, externalID = strings.TrimSpace(source), strings.TrimSpace(externalID)
	if source == "" || externalID == "" {
		return httpx.Invalid("缺少外部来源或外部编号")
	}
	r, err := s.m.findIssue(ctx, s.m.q, key)
	if err != nil {
		return err
	}
	other, found, err := s.FindByExternal(ctx, source, externalID)
	if err != nil {
		return err
	}
	if found && other.Key != issueKey(r.ProjectKey, r.Issue.Number) {
		return httpx.NewError(409, "conflict", "这个外部编号已经关联了别的 Issue")
	}
	_, err = s.m.q.SetIssueExternal(ctx, db.SetIssueExternalParams{ExternalSource: source, ExternalID: externalID, ID: r.Issue.ID})
	return err
}

func (s *syncService) ChangedSince(ctx context.Context, projectIDs []int64, since time.Time) ([]contracts.SyncIssue, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	rows, err := s.m.q.ChangedSince(ctx, db.ChangedSinceParams{ProjectIds: projectIDs, Since: since.UTC()})
	if err != nil {
		return nil, err
	}
	out := make([]contracts.SyncIssue, len(rows))
	for i, r := range rows {
		out[i] = s.m.toSync(r.Issue, r.ProjectKey)
	}
	return out, nil
}
