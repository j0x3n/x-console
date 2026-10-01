package projects

import (
	"context"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// workService implements contracts.IssueWork for B47 agents.
type workService struct{ m *Module }

var _ contracts.IssueWork = (*workService)(nil)

func (s *workService) Brief(ctx context.Context, key string) (contracts.IssueBrief, error) {
	r, err := s.m.findIssue(ctx, s.m.q, key)
	if err != nil {
		return contracts.IssueBrief{}, err
	}
	out := contracts.IssueBrief{Key: issueKey(r.ProjectKey, r.Issue.Number), Title: r.Issue.Title,
		Description: r.Issue.Description, Status: r.Issue.Status}
	if out.OpenItems, err = s.m.q.OpenChecklistItems(ctx, r.Issue.ID); err != nil {
		return out, err
	}
	comments, err := s.m.q.RecentComments(ctx, r.Issue.ID)
	if err != nil {
		return out, err
	}
	for i := len(comments) - 1; i >= 0; i-- {
		out.Comments = append(out.Comments, comments[i].Body)
	}
	return out, nil
}

func (s *workService) Comment(ctx context.Context, key, author, body string) (err error) {
	m := s.m
	defer func() { m.d.Audit.Record(ctx, "issue_comment.create", key, map[string]any{"author": author}, err) }()
	if strings.TrimSpace(body) == "" {
		return httpx.Invalid("评论不能为空")
	}
	r, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return err
	}
	c, err := m.q.CreateCommentBy(ctx, db.CreateCommentByParams{IssueID: r.Issue.ID, Body: body, CreatedAt: m.now(), Author: author})
	if err != nil {
		return err
	}
	m.d.Bus.Publish("issue_comment.created", issueEvent{IssueKey: issueKey(r.ProjectKey, r.Issue.Number), Data: toComment(c)})
	return nil
}

func (s *workService) AddMember(ctx context.Context, key, kind, id string) error {
	m := s.m
	if kind != "agent" && kind != "me" {
		return httpx.Invalid("成员类型不正确")
	}
	if kind == "me" {
		id = ""
	}
	var issueID int64
	err := m.tx(ctx, func(q *db.Queries) error {
		r, err := m.findIssue(ctx, q, key)
		if err != nil {
			return err
		}
		issueID = r.Issue.ID
		return q.AddIssueMember(ctx, db.AddIssueMemberParams{IssueID: issueID, MemberKind: kind, MemberID: id})
	})
	if err != nil {
		return err
	}
	if out, err := m.issueByID(ctx, issueID); err == nil {
		m.d.Bus.Publish("issue.updated", out)
	}
	return nil
}
