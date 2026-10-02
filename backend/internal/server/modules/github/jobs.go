package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/api"
)

var errJobsUnsupported = httpx.NewError(501, "steps_unavailable", "这个服务器不提供步骤详情")

type jobsEntry struct {
	At   time.Time
	Jobs []api.GitHubJob
}
type jobsCache struct {
	mu      sync.Mutex
	entries map[string]jobsEntry
}
type remoteStep struct {
	Number      int        `json:"number"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  *string    `json:"conclusion"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
type remoteJob struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	Conclusion  *string      `json:"conclusion"`
	HTMLURL     string       `json:"html_url"`
	URL         string       `json:"url"`
	StartedAt   *time.Time   `json:"started_at"`
	CompletedAt *time.Time   `json:"completed_at"`
	Steps       []remoteStep `json:"steps"`
}

func normalizedStatus(status string, conclusion *string) (string, string) {
	result := ""
	if conclusion != nil {
		result = *conclusion
	}
	switch status {
	case "success", "failure", "cancelled", "skipped", "timed_out":
		return "completed", status
	case "waiting", "blocked":
		return "queued", ""
	case "running":
		return "in_progress", ""
	}
	return status, result
}
func (m *Module) fetchJobs(ctx context.Context, c *restClient, k repoKey, id int64) ([]api.GitHubJob, error) {
	m.jobs.mu.Lock()
	defer m.jobs.mu.Unlock()
	key := fmt.Sprintf("%d:%s:%d:%s:%s", k.ConnectionID, k.Repo, id, c.base, c.token)
	if e, ok := m.jobs.entries[key]; ok && m.now().Sub(e.At) < 5*time.Second {
		return e.Jobs, nil
	}
	out := []api.GitHubJob{}
	for page := 1; page <= 20; page++ {
		var raw json.RawMessage
		more, err := c.getPage(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/jobs", k.Repo, id), pageQuery(c, 100, page), &raw)
		if err != nil {
			if c.forge == "forgejo" && (statusOf(err) == 404 || statusOf(err) == 501) {
				return nil, errJobsUnsupported
			}
			return nil, err
		}
		var batch []remoteJob
		var wrapped struct {
			Jobs []remoteJob `json:"jobs"`
		}
		if len(raw) > 0 && raw[0] == '[' {
			err = json.Unmarshal(raw, &batch)
		} else {
			err = json.Unmarshal(raw, &wrapped)
			batch = wrapped.Jobs
		}
		if err != nil {
			return nil, err
		}
		for _, j := range batch {
			status, conclusion := normalizedStatus(j.Status, j.Conclusion)
			v := api.GitHubJob{Id: j.ID, Name: j.Name, Status: status, Conclusion: conclusion, Url: j.HTMLURL, StartedAt: j.StartedAt, CompletedAt: j.CompletedAt, Steps: []api.GitHubStep{}}
			if v.Url == "" {
				v.Url = j.URL
			}
			for _, s := range j.Steps {
				status, conclusion := normalizedStatus(s.Status, s.Conclusion)
				v.Steps = append(v.Steps, api.GitHubStep{Number: s.Number, Name: s.Name, Status: status, Conclusion: conclusion, StartedAt: s.StartedAt, CompletedAt: s.CompletedAt})
			}
			sort.SliceStable(v.Steps, func(i, j int) bool { return v.Steps[i].Number < v.Steps[j].Number })
			out = append(out, v)
		}
		if !more && (c.forge != "forgejo" || len(batch) < 100) {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	if len(m.jobs.entries) > 2000 {
		m.jobs.entries = map[string]jobsEntry{}
	}
	m.jobs.entries[key] = jobsEntry{At: m.now(), Jobs: out}
	return out, nil
}
func jobProgress(jobs []api.GitHubJob) (done, total int, current string) {
	for _, j := range jobs {
		for _, s := range j.Steps {
			total++
			if s.Status == "completed" {
				done++
			}
			if current == "" && s.Status == "in_progress" {
				current = s.Name
			}
		}
	}
	return
}
func (m *Module) ListGitHubRunJobs(w http.ResponseWriter, r *http.Request, id int64, p api.ListGitHubRunJobsParams) {
	k, err := m.resolveRepo(r.Context(), p.ConnectionId, p.Repo)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var run cachedRun
	if err = m.cachedOne(r.Context(), k, "run", strconv.FormatInt(id, 10), &run); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if run.Synthetic {
		httpx.Fail(w, r, errJobsUnsupported)
		return
	}
	cfg, err := m.accountConfig(r.Context(), k.ConnectionID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.fetchJobs(r.Context(), m.client(cfg), k, id)
	if err != nil {
		if !errors.Is(err, errJobsUnsupported) && statusOf(err) > 0 {
			err = httpx.NewError(502, "git_unavailable", err.Error())
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, 200, out)
}
func (m *Module) fetchRuns(ctx context.Context, c *restClient, k repoKey, branch string, commits []ghCommit) ([]cachedRun, error) {
	var raw json.RawMessage
	err := c.get(ctx, "/repos/"+k.Repo+"/actions/runs", pageQuery(c, runsPerRepo, 1), &raw)
	if err != nil {
		if c.forge == "forgejo" && (statusOf(err) == 404 || statusOf(err) == 501) {
			return m.fallbackRun(ctx, c, k, branch, commits)
		}
		if c.forge != "forgejo" && statusOf(err) == 404 {
			return []cachedRun{}, nil
		}
		return nil, err
	}
	var wrapped ghRuns
	var batch []ghRun
	if len(raw) > 0 && raw[0] == '[' {
		err = json.Unmarshal(raw, &batch)
	} else {
		err = json.Unmarshal(raw, &wrapped)
		batch = wrapped.WorkflowRuns
	}
	if err != nil {
		return nil, err
	}
	out := []cachedRun{}
	for _, r := range batch {
		status, conclusion := normalizedStatus(r.Status, r.Conclusion)
		url := r.HTMLURL
		if url == "" {
			url = r.URL
		}
		out = append(out, cachedRun{GitHubRun: api.GitHubRun{ConnectionId: ptr(k.ConnectionID), Forge: ptr(api.Forge(c.forge)), Id: r.ID, Repo: k.Repo, Name: r.Name, Branch: r.HeadBranch, Event: r.Event, Status: status, Conclusion: conclusion, Url: url, HeadSha: ptr(r.HeadSHA), DefaultBranch: branch != "" && branch == r.HeadBranch, CreatedAt: utc(r.CreatedAt), UpdatedAt: utc(r.UpdatedAt)}, WorkflowID: r.WorkflowID, Attempt: max(1, r.RunAttempt)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].Id > out[j].Id
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}
func (m *Module) fallbackRun(ctx context.Context, c *restClient, k repoKey, branch string, commits []ghCommit) ([]cachedRun, error) {
	if len(commits) == 0 {
		return []cachedRun{}, nil
	}
	head := commits[0]
	var st struct {
		State    string `json:"state"`
		Total    int    `json:"total_count"`
		Statuses []struct {
			TargetURL string    `json:"target_url"`
			CreatedAt time.Time `json:"created_at"`
			UpdatedAt time.Time `json:"updated_at"`
		} `json:"statuses"`
	}
	if err := c.get(ctx, "/repos/"+k.Repo+"/commits/"+head.SHA+"/status", nil, &st); err != nil {
		return nil, err
	}
	if st.Total == 0 {
		return []cachedRun{}, nil
	}
	var id int64
	err := m.d.DB.QueryRowContext(ctx, `INSERT INTO github_synthetic_runs(connection_id,repo,sha) VALUES(?,?,?) ON CONFLICT(connection_id,repo,sha) DO UPDATE SET sha=excluded.sha RETURNING id`, k.ConnectionID, k.Repo, head.SHA).Scan(&id)
	if err != nil {
		return nil, err
	}
	status, conclusion := "completed", "success"
	switch st.State {
	case "pending":
		status, conclusion = "in_progress", ""
	case "failure", "error":
		conclusion = "failure"
	}
	created, updated := head.Commit.Committer.Date, head.Commit.Committer.Date
	url := head.HTMLURL
	for _, s := range st.Statuses {
		if s.CreatedAt.After(created) {
			created = s.CreatedAt
		}
		if s.UpdatedAt.After(updated) {
			updated = s.UpdatedAt
		}
		if s.TargetURL != "" {
			url = s.TargetURL
		}
	}
	return []cachedRun{{GitHubRun: api.GitHubRun{ConnectionId: ptr(k.ConnectionID), Forge: ptr(api.Forgejo), Id: -id, Repo: k.Repo, Name: "提交状态", Branch: branch, Event: "push", Status: status, Conclusion: conclusion, Url: url, HeadSha: ptr(head.SHA), DefaultBranch: true, CreatedAt: utc(created), UpdatedAt: utc(updated)}, WorkflowID: 0, Attempt: 1, Synthetic: true}}, nil
}
