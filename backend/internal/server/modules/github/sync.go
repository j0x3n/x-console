package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// lastSync is stored under keyLastSync.
type lastSync struct {
	At    time.Time `json:"at"`
	Error string    `json:"error,omitempty"`
}

const runsPerRepo = 20

func (m *Module) setSyncing(v bool) {
	m.stateMu.Lock()
	m.syncing = v
	m.stateMu.Unlock()
}

func (m *Module) isSyncing() bool {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.syncing
}

// sync refreshes every watched repository. Errors of single repositories
// do not stop the others; they are joined into the stored result.
func (m *Module) sync(ctx context.Context) error {
	m.syncMu.Lock()
	defer m.syncMu.Unlock()
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return err
	}
	m.setSyncing(true)
	defer m.setSyncing(false)

	c := m.client(cfg)
	var errs []string
	var me ghUser
	if err := c.get(ctx, "/user", nil, &me); err != nil {
		errs = append(errs, err.Error())
	} else if me.Login != cfg.Login {
		cfg.Login = me.Login
		if err := m.d.Settings.Set(ctx, keyLogin, me.Login); err != nil {
			return err
		}
	}
	if len(errs) == 0 {
		if err := m.dropUnwatched(ctx, cfg.Repos); err != nil {
			return err
		}
		for _, repo := range cfg.Repos {
			if err := m.syncRepo(ctx, c, repo, cfg.Login); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				m.log.Warn("github sync failed", "repo", repo, "err", err)
				errs = append(errs, repo+"："+err.Error())
				if isRateLimited(err) {
					break
				}
			}
		}
	}
	result := lastSync{At: m.now(), Error: strings.Join(errs, "；")}
	if err := m.d.Settings.Set(ctx, keyLastSync, result); err != nil {
		return err
	}
	m.d.Bus.Publish("github.synced", map[string]any{"at": result.At, "error": result.Error})
	if result.Error != "" {
		return errors.New(result.Error)
	}
	return nil
}

// dropUnwatched removes cached rows of repositories no longer watched.
func (m *Module) dropUnwatched(ctx context.Context, repos []string) error {
	keep := keepStrings(repos)
	if err := m.q.DeletePullsNotIn(ctx, keep); err != nil {
		return err
	}
	if err := m.q.DeleteRunsNotIn(ctx, keep); err != nil {
		return err
	}
	return m.q.DeleteIssuesNotIn(ctx, keep)
}

// syncRepo refreshes one repository. Pull requests are required; runs and
// issues are best effort (the token may lack the Actions or Issues
// permission), but their errors are still reported.
func (m *Module) syncRepo(ctx context.Context, c *restClient, repo, login string) error {
	var info ghRepo
	if err := c.get(ctx, "/repos/"+repo, nil, &info); err != nil {
		return err
	}
	if err := m.syncPulls(ctx, c, repo); err != nil {
		return err
	}
	var errs []error
	if err := m.syncRuns(ctx, c, repo, info.DefaultBranch); err != nil {
		if isRateLimited(err) {
			return err
		}
		errs = append(errs, fmt.Errorf("读取 CI 运行失败：%w", err))
	}
	if login != "" {
		if err := m.syncIssues(ctx, c, repo, login); err != nil {
			if isRateLimited(err) {
				return err
			}
			errs = append(errs, fmt.Errorf("读取 Issue 失败：%w", err))
		}
	}
	return errors.Join(errs...)
}

func (m *Module) syncPulls(ctx context.Context, c *restClient, repo string) error {
	pulls, err := m.openPulls(ctx, c, repo)
	if err != nil {
		return err
	}
	now := m.now()
	numbers := make([]int64, 0, len(pulls))
	for _, p := range pulls {
		review, err := m.reviewState(ctx, c, repo, p)
		if err != nil {
			return err
		}
		checks, err := m.checkState(ctx, c, repo, p.Head.SHA)
		if err != nil {
			return err
		}
		if err := m.q.UpsertPull(ctx, db.UpsertPullParams{
			Repo: repo, Number: int64(p.Number), Title: p.Title, Author: p.User.Login, Url: p.HTMLURL,
			HeadRef: p.Head.Ref, HeadSha: p.Head.SHA, BaseRef: p.Base.Ref, Draft: p.Draft,
			ReviewState: review, CheckState: checks, CreatedAt: p.CreatedAt.UTC(), UpdatedAt: p.UpdatedAt.UTC(), SyncedAt: now,
		}); err != nil {
			return err
		}
		numbers = append(numbers, int64(p.Number))
		ref := fmt.Sprintf("%s#%d", repo, p.Number)
		if err := m.ciTransition(ctx, "pr:"+ref, checks, notify.Notification{
			Kind: "github.ci_failed", Title: "PR 检查失败：" + ref, Body: p.Title, Link: "/github",
			Priority: notify.PriorityHigh, Source: "github",
			Data: map[string]any{"repo": repo, "number": p.Number, "url": p.HTMLURL},
		}); err != nil {
			return err
		}
		m.linkPull(ctx, repo, p.Number, p.Title, p.Head.Ref, p.HTMLURL)
	}
	closed, err := m.q.DeletePullsExcept(ctx, db.DeletePullsExceptParams{Repo: repo, Keep: keepInts(numbers)})
	if err != nil {
		return err
	}
	for _, n := range closed {
		if err := m.q.DeleteCIState(ctx, fmt.Sprintf("pr:%s#%d", repo, n)); err != nil {
			return err
		}
	}
	return nil
}

// maxPullPages caps the open pull requests read per repository (2000).
const maxPullPages = 20

// openPulls reads every open pull request, page by page.
func (m *Module) openPulls(ctx context.Context, c *restClient, repo string) ([]ghPull, error) {
	var all []ghPull
	for page := 1; page <= maxPullPages; page++ {
		var batch []ghPull
		q := url.Values{"state": {"open"}, "per_page": {"100"}, "page": {strconv.Itoa(page)}}
		more, err := c.getPage(ctx, "/repos/"+repo+"/pulls", q, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if !more {
			break
		}
	}
	return all, nil
}

// reviewState sums up the latest review of every reviewer.
func (m *Module) reviewState(ctx context.Context, c *restClient, repo string, p ghPull) (string, error) {
	var reviews []ghReview
	path := fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, p.Number)
	if err := c.get(ctx, path, url.Values{"per_page": {"100"}}, &reviews); err != nil {
		return "", err
	}
	return summarizeReviews(reviews, len(p.RequestedReviewers)+len(p.RequestedTeams) > 0), nil
}

func summarizeReviews(reviews []ghReview, requested bool) string {
	latest := map[string]string{}
	commented := false
	for _, r := range reviews { // oldest first
		switch r.State {
		case "APPROVED", "CHANGES_REQUESTED", "DISMISSED":
			latest[r.User.Login] = r.State
		case "COMMENTED":
			commented = true
		}
	}
	approved := false
	for _, s := range latest {
		if s == "CHANGES_REQUESTED" {
			return "changes_requested"
		}
		if s == "APPROVED" {
			approved = true
		}
	}
	switch {
	case approved:
		return "approved"
	case requested:
		return "pending"
	case commented:
		return "commented"
	}
	return "none"
}

// checkState combines commit statuses and check runs of a commit.
func (m *Module) checkState(ctx context.Context, c *restClient, repo, sha string) (string, error) {
	if sha == "" {
		return "none", nil
	}
	var st ghCombinedStatus
	var runs ghCheckRuns
	// Missing permission for one of them just leaves it out.
	if err := c.get(ctx, "/repos/"+repo+"/commits/"+sha+"/status", nil, &st); err != nil && !ignorable(err) {
		return "", err
	}
	if err := c.get(ctx, "/repos/"+repo+"/commits/"+sha+"/check-runs", url.Values{"per_page": {"100"}}, &runs); err != nil && !ignorable(err) {
		return "", err
	}
	var states []string
	if st.TotalCount > 0 {
		states = append(states, st.State)
	}
	for _, r := range runs.CheckRuns {
		if r.Status != "completed" {
			states = append(states, "pending")
			continue
		}
		switch r.Conclusion {
		case "success", "neutral", "skipped":
			states = append(states, "success")
		case "failure", "timed_out", "action_required", "startup_failure":
			states = append(states, "failure")
		case "cancelled":
			// Usually replaced by a newer run; says nothing either way.
		default:
			states = append(states, "pending")
		}
	}
	return combineChecks(states), nil
}

func combineChecks(states []string) string {
	if len(states) == 0 {
		return "none"
	}
	out := "success"
	for _, s := range states {
		switch s {
		case "failure", "error":
			return "failure"
		case "pending":
			out = "pending"
		}
	}
	return out
}

func ignorable(err error) bool {
	s := statusOf(err)
	return s == 403 || s == 404
}

func (m *Module) syncRuns(ctx context.Context, c *restClient, repo, defaultBranch string) error {
	var runs ghRuns
	if err := c.get(ctx, "/repos/"+repo+"/actions/runs", url.Values{"per_page": {strconv.Itoa(runsPerRepo)}}, &runs); err != nil {
		if statusOf(err) == 404 {
			// Actions turned off for the repository.
			return m.q.DeleteRunsByRepo(ctx, repo)
		}
		return err
	}
	ids := make([]int64, 0, len(runs.WorkflowRuns))
	settled := map[int64]bool{}
	for _, r := range runs.WorkflowRuns { // newest first
		conclusion := ""
		if r.Conclusion != nil {
			conclusion = *r.Conclusion
		}
		onDefault := defaultBranch != "" && r.HeadBranch == defaultBranch
		if err := m.q.UpsertRun(ctx, db.UpsertRunParams{
			ID: r.ID, Repo: repo, WorkflowID: r.WorkflowID, Name: r.Name, Branch: r.HeadBranch, Event: r.Event,
			Status: r.Status, Conclusion: conclusion, Url: r.HTMLURL, HeadSha: r.HeadSHA, DefaultBranch: onDefault,
			CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
		}); err != nil {
			return err
		}
		ids = append(ids, r.ID)
		// Only the newest finished run of each workflow on the default
		// branch decides whether the branch is green.
		if !onDefault || r.Status != "completed" || settled[r.WorkflowID] {
			continue
		}
		state := runState(conclusion)
		if state == "" {
			continue
		}
		settled[r.WorkflowID] = true
		if err := m.ciTransition(ctx, fmt.Sprintf("run:%s:%d", repo, r.WorkflowID), state, notify.Notification{
			Kind: "github.ci_failed", Title: fmt.Sprintf("%s 的 %s 失败了", repo, r.Name),
			Body: "分支 " + r.HeadBranch, Link: "/github?tab=runs", Priority: notify.PriorityHigh, Source: "github",
			Data: map[string]any{"repo": repo, "runId": r.ID, "url": r.HTMLURL},
		}); err != nil {
			return err
		}
	}
	return m.q.DeleteRunsExcept(ctx, db.DeleteRunsExceptParams{Repo: repo, Keep: keepInts(ids)})
}

// runState maps a run conclusion to success or failure. Cancelled and
// skipped runs say nothing about the branch.
func runState(conclusion string) string {
	switch conclusion {
	case "success":
		return "success"
	case "failure", "timed_out", "startup_failure":
		return "failure"
	}
	return ""
}

// ciTransition stores the latest settled result under key and sends n when
// it turns from success to failure. Pending results are ignored, so
// success → pending → failure still notifies once.
func (m *Module) ciTransition(ctx context.Context, key, state string, n notify.Notification) error {
	if state != "success" && state != "failure" {
		return nil
	}
	prev, err := m.q.GetCIState(ctx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if prev == state {
		return nil
	}
	if err := m.q.SetCIState(ctx, db.SetCIStateParams{Key: key, State: state, UpdatedAt: m.now()}); err != nil {
		return err
	}
	if prev == "success" && state == "failure" {
		if _, err := m.d.Notify.Send(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) syncIssues(ctx context.Context, c *restClient, repo, login string) error {
	type entry struct {
		issue    ghIssue
		assigned bool
		created  bool
	}
	byNumber := map[int]*entry{}
	var order []int
	for _, filter := range []string{"assignee", "creator"} {
		var list []ghIssue
		q := url.Values{"state": {"open"}, "per_page": {"50"}, filter: {login}}
		if err := c.get(ctx, "/repos/"+repo+"/issues", q, &list); err != nil {
			return err
		}
		for _, is := range list {
			if len(is.PullRequest) > 0 && string(is.PullRequest) != "null" {
				continue
			}
			e, ok := byNumber[is.Number]
			if !ok {
				e = &entry{issue: is}
				byNumber[is.Number] = e
				order = append(order, is.Number)
			}
			if filter == "assignee" {
				e.assigned = true
			} else {
				e.created = true
			}
		}
	}
	numbers := make([]int64, 0, len(order))
	for _, n := range order {
		e := byNumber[n]
		relation := "assigned"
		switch {
		case e.assigned && e.created:
			relation = "both"
		case e.created:
			relation = "created"
		}
		assignees := make([]string, 0, len(e.issue.Assignees))
		for _, a := range e.issue.Assignees {
			assignees = append(assignees, a.Login)
		}
		labels := make([]string, 0, len(e.issue.Labels))
		for _, l := range e.issue.Labels {
			labels = append(labels, l.Name)
		}
		aj, _ := json.Marshal(assignees)
		lj, _ := json.Marshal(labels)
		if err := m.q.UpsertIssue(ctx, db.UpsertIssueParams{
			Repo: repo, Number: int64(n), Title: e.issue.Title, Url: e.issue.HTMLURL, Author: e.issue.User.Login,
			Assignees: string(aj), Labels: string(lj), Relation: relation,
			CreatedAt: e.issue.CreatedAt.UTC(), UpdatedAt: e.issue.UpdatedAt.UTC(),
		}); err != nil {
			return err
		}
		numbers = append(numbers, int64(n))
	}
	return m.q.DeleteIssuesExcept(ctx, db.DeleteIssuesExceptParams{Repo: repo, Keep: keepInts(numbers)})
}

// ---- links ----

var (
	issueKeyRe   = regexp.MustCompile(`(?i)\b([a-z]{2,5})-([1-9][0-9]{0,8})\b`)
	codingTaskRe = regexp.MustCompile(`^xc/([0-9]+)(?:-|$)`)
)

// issueKeysIn finds issue keys such as XC-12 in a title and a branch name.
func issueKeysIn(texts ...string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range texts {
		for _, m := range issueKeyRe.FindAllStringSubmatch(t, -1) {
			key := strings.ToUpper(m[1]) + "-" + m[2]
			if !seen[key] {
				seen[key] = true
				out = append(out, key)
			}
		}
	}
	return out
}

// codingTaskID reads the task id from a branch like xc/42-fix-login.
func codingTaskID(branch string) int64 {
	m := codingTaskRe.FindStringSubmatch(branch)
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// linkPull attaches a pull request to the local issues named in its title
// or branch (once per issue), and records the coding task of an xc/ branch.
// Problems are logged; they never fail the sync.
func (m *Module) linkPull(ctx context.Context, repo string, number int, title, branch, htmlURL string) {
	now := m.now()
	if id := codingTaskID(branch); id > 0 {
		if err := m.q.InsertLink(ctx, db.InsertLinkParams{Repo: repo, Number: int64(number), Kind: "coding_task",
			Ref: strconv.FormatInt(id, 10), CreatedAt: now}); err != nil {
			m.log.Warn("github link coding task", "err", err)
		}
	}
	keys := issueKeysIn(title, branch)
	if len(keys) == 0 {
		return
	}
	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
	if !ok {
		return
	}
	ref := fmt.Sprintf("%s#%d", repo, number)
	for _, key := range keys {
		n, err := m.q.LinkExists(ctx, db.LinkExistsParams{Repo: repo, Number: int64(number), Kind: "issue", Ref: key})
		if err != nil || n > 0 {
			continue
		}
		if _, err := issues.Get(ctx, key); err != nil {
			continue // not a local issue, for example "UTF-8"
		}
		link := contracts.IssueLink{Kind: "pull_request", Title: ref + " " + title, URL: htmlURL, Ref: ref}
		if err := issues.AttachLink(ctx, key, link); err != nil {
			m.log.Warn("github attach link", "issue", key, "pr", ref, "err", err)
			continue
		}
		if err := m.q.InsertLink(ctx, db.InsertLinkParams{Repo: repo, Number: int64(number), Kind: "issue", Ref: key, CreatedAt: now}); err != nil {
			m.log.Warn("github link issue", "err", err)
		}
	}
}
