package linear

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// gqlClient calls the Linear GraphQL API.
type gqlClient struct {
	url string
	key string
	hc  *http.Client
}

// gqlError is an error answer from Linear.
type gqlError struct {
	Status  int
	Code    string
	Message string
}

func (e *gqlError) Error() string {
	switch {
	case e.Status == http.StatusUnauthorized || e.Code == "AUTHENTICATION_ERROR":
		return "API key 无效或已过期"
	case e.Status == http.StatusTooManyRequests || e.Code == "RATELIMITED":
		return "Linear 请求太频繁，请稍后再试"
	case e.Message != "":
		return "Linear 返回错误：" + e.Message
	}
	return fmt.Sprintf("Linear 返回 %d", e.Status)
}

func (c *gqlClient) do(ctx context.Context, op, query string, vars map[string]any, out any) error {
	raw, err := json.Marshal(map[string]any{"operationName": op, "query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "x-console")
	// Personal API keys go in the header as is; OAuth tokens need Bearer.
	if strings.HasPrefix(c.key, "lin_oauth_") {
		req.Header.Set("Authorization", "Bearer "+c.key)
	} else {
		req.Header.Set("Authorization", c.key)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("连不上 Linear：%w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message    string `json:"message"`
			Extensions struct {
				Code string `json:"code"`
			} `json:"extensions"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(body, &env)
	if len(env.Errors) > 0 {
		return &gqlError{Status: resp.StatusCode, Code: env.Errors[0].Extensions.Code, Message: env.Errors[0].Message}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &gqlError{Status: resp.StatusCode}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("Linear 返回的数据看不懂：%w", err)
	}
	return nil
}

// ---- shapes ----

type lnTeam struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type lnState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"` // triage, backlog, unstarted, started, completed, canceled
	Position float64 `json:"position"`
}

type lnIssue struct {
	ID          string    `json:"id"`
	Identifier  string    `json:"identifier"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	Priority    float64   `json:"priority"`
	DueDate     *string   `json:"dueDate"`
	UpdatedAt   time.Time `json:"updatedAt"`
	URL         string    `json:"url"`
	Team        struct {
		ID string `json:"id"`
	} `json:"team"`
	State lnState `json:"state"`
}

const issueFields = `fragment IssueFields on Issue {
  id identifier title description priority dueDate updatedAt url
  team { id }
  state { id name type position }
}`

const qViewer = `query Viewer { viewer { id name email } }`

const qTeams = `query Teams { teams(first: 100) { nodes { id key name } } }`

const qTeamStates = `query TeamStates($teamId: String!) {
  team(id: $teamId) { states(first: 100) { nodes { id name type position } } }
}`

const qIssues = `query Issues($teamId: ID!, $since: DateTimeOrDuration!, $after: String) {
  issues(first: 100, after: $after, orderBy: updatedAt,
         filter: { team: { id: { eq: $teamId } }, updatedAt: { gt: $since } }) {
    nodes { ...IssueFields }
    pageInfo { hasNextPage endCursor }
  }
}
` + issueFields

const qIssue = `query Issue($id: String!) { issue(id: $id) { ...IssueFields } }
` + issueFields

const mUpdateIssue = `mutation UpdateIssue($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) { success issue { ...IssueFields } }
}
` + issueFields

func (c *gqlClient) viewer(ctx context.Context) (string, error) {
	var out struct {
		Viewer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"viewer"`
	}
	if err := c.do(ctx, "Viewer", qViewer, nil, &out); err != nil {
		return "", err
	}
	if out.Viewer.Name != "" {
		return out.Viewer.Name, nil
	}
	return out.Viewer.Email, nil
}

func (c *gqlClient) teams(ctx context.Context) ([]lnTeam, error) {
	var out struct {
		Teams struct {
			Nodes []lnTeam `json:"nodes"`
		} `json:"teams"`
	}
	if err := c.do(ctx, "Teams", qTeams, nil, &out); err != nil {
		return nil, err
	}
	return out.Teams.Nodes, nil
}

func (c *gqlClient) teamStates(ctx context.Context, teamID string) ([]lnState, error) {
	var out struct {
		Team *struct {
			States struct {
				Nodes []lnState `json:"nodes"`
			} `json:"states"`
		} `json:"team"`
	}
	if err := c.do(ctx, "TeamStates", qTeamStates, map[string]any{"teamId": teamID}, &out); err != nil {
		return nil, err
	}
	if out.Team == nil {
		return nil, &gqlError{Message: "找不到团队 " + teamID}
	}
	return out.Team.States.Nodes, nil
}

// issuesSince lists issues of a team updated after since, oldest first.
func (c *gqlClient) issuesSince(ctx context.Context, teamID string, since time.Time) ([]lnIssue, error) {
	var all []lnIssue
	var after *string
	for page := 0; page < 50; page++ {
		var out struct {
			Issues struct {
				Nodes    []lnIssue `json:"nodes"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"issues"`
		}
		vars := map[string]any{"teamId": teamID, "since": since.UTC().Format(time.RFC3339Nano), "after": after}
		if err := c.do(ctx, "Issues", qIssues, vars, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Issues.Nodes...)
		if !out.Issues.PageInfo.HasNextPage || out.Issues.PageInfo.EndCursor == "" {
			break
		}
		cursor := out.Issues.PageInfo.EndCursor
		after = &cursor
	}
	return all, nil
}

func (c *gqlClient) issue(ctx context.Context, id string) (*lnIssue, error) {
	var out struct {
		Issue *lnIssue `json:"issue"`
	}
	if err := c.do(ctx, "Issue", qIssue, map[string]any{"id": id}, &out); err != nil {
		return nil, err
	}
	return out.Issue, nil
}

func (c *gqlClient) updateIssue(ctx context.Context, id string, input map[string]any) (lnIssue, error) {
	var out struct {
		IssueUpdate struct {
			Success bool     `json:"success"`
			Issue   *lnIssue `json:"issue"`
		} `json:"issueUpdate"`
	}
	if err := c.do(ctx, "UpdateIssue", mUpdateIssue, map[string]any{"id": id, "input": input}, &out); err != nil {
		return lnIssue{}, err
	}
	if !out.IssueUpdate.Success || out.IssueUpdate.Issue == nil {
		return lnIssue{}, &gqlError{Message: "更新没有成功"}
	}
	return *out.IssueUpdate.Issue, nil
}
