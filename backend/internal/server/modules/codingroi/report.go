package codingroi

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/codingroi/api"
)

const dateLayout = "2006-01-02"

// query is a parsed report request.
type query struct {
	Period  string // week or month
	GroupBy string // repo, tool or agent
	From    time.Time
	To      time.Time // the day after the last day
	Now     time.Time
}

// parseQuery checks the parameters. date is any day inside the period; empty
// means today. Weeks start on Monday.
func parseQuery(loc *time.Location, period, date, groupBy string, now time.Time) (query, error) {
	if period == "" {
		period = "week"
	}
	if period != "week" && period != "month" {
		return query{}, httpx.Invalid("范围只能是 week 或 month")
	}
	if groupBy == "" {
		groupBy = "repo"
	}
	if groupBy != "repo" && groupBy != "tool" && groupBy != "agent" {
		return query{}, httpx.Invalid("分组只能是 repo、tool 或 agent")
	}
	n := now.In(loc)
	day := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	if date != "" {
		t, err := time.ParseInLocation(dateLayout, date, loc)
		if err != nil {
			return query{}, httpx.Invalid("日期要写成 YYYY-MM-DD")
		}
		day = t
	}
	q := query{Period: period, GroupBy: groupBy, Now: now}
	if period == "week" {
		back := (int(day.Weekday()) + 6) % 7
		q.From = day.AddDate(0, 0, -back)
		q.To = q.From.AddDate(0, 0, 7)
	} else {
		q.From = time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, loc)
		q.To = q.From.AddDate(0, 1, 0)
	}
	return q, nil
}

// taskRow is what is read about one task.
type taskRow struct {
	ID                                 int64
	Prompt, IssueKey, Executor         string
	Status, PRURL                      string
	AgentID                            *int64
	RepoID                             int64
	RepoName                           string
	CreatedAt, UpdatedAt               time.Time
	StartedAt, FinishedAt              *time.Time
	Runs                               int
	UsageRows, CostRows                int
	Cost                               float64
	InputTokens, OutputTokens, UsageMs int64
	Outcome                            string // merged or closed, from codingroi_outcomes
	OutcomeAt                          time.Time
}

// report builds the whole report.
func (m *Module) report(ctx context.Context, q query) (api.CodingROIReport, error) {
	// A merge may have happened since the last check.
	if err := m.syncPulls(ctx); err != nil {
		m.d.Log.Warn("codingroi: check pull requests", "err", err)
	}
	rows, err := m.loadTasks(ctx, q)
	if err != nil {
		return api.CodingROIReport{}, err
	}
	agents, err := m.agentNames(ctx)
	if err != nil {
		return api.CodingROIReport{}, err
	}
	screenOK := !m.screenHidden(ctx)

	wins := map[int64][]window{}
	ongoing := map[int64]bool{}
	var lo, hi time.Time
	for _, r := range rows {
		w, ok, on := reviewWindowOf(r, q.Now)
		if !ok {
			continue
		}
		wins[r.ID] = []window{w}
		ongoing[r.ID] = on
		if lo.IsZero() || w.Start.Before(lo) {
			lo = w.Start
		}
		if w.End.After(hi) {
			hi = w.End
		}
	}
	var results map[int64]reviewResult
	if screenOK && len(wins) > 0 {
		mins, err := m.screenMinutes(ctx, lo, hi)
		if err != nil {
			return api.CodingROIReport{}, err
		}
		results = allocateReview(wins, mins)
	}

	tasks := make([]api.CodingROITask, 0, len(rows))
	for _, r := range rows {
		t := taskOf(r, agents)
		if _, has := wins[r.ID]; has {
			if res := results[r.ID]; res.HasData {
				v := round(res.Minutes, 1)
				t.ReviewMinutes = &v
				t.ReviewState = api.CodingROIReviewState("done")
				if ongoing[r.ID] {
					t.ReviewState = api.CodingROIReviewState("ongoing")
				}
			} else {
				t.ReviewState = api.CodingROIReviewState("no_data")
			}
		}
		if t.ReviewMinutes != nil && t.RunSeconds != nil && *t.RunSeconds > 0 {
			v := round(*t.ReviewMinutes/(float64(*t.RunSeconds)/60), 2)
			t.ReviewPerRun = &v
		}
		tasks = append(tasks, t)
	}

	rep := api.CodingROIReport{
		Period: api.CodingROIReportPeriod(q.Period), GroupBy: api.CodingROIReportGroupBy(q.GroupBy),
		From: q.From.Format(dateLayout), To: q.To.AddDate(0, 0, -1).Format(dateLayout),
		ScreenAvailable: screenOK, Tasks: tasks, Groups: groupTasks(tasks, q.GroupBy),
		Totals: summarize("", "", tasks),
	}
	return rep, nil
}

// loadTasks reads the tasks created in the period with everything that is
// kept about them.
func (m *Module) loadTasks(ctx context.Context, q query) ([]taskRow, error) {
	from, to := q.From.UTC(), q.To.UTC()
	rs, err := m.d.DB.QueryContext(ctx, `SELECT t.id, t.prompt, t.issue_key, t.executor, t.status, t.pr_url, t.ai_agent_id,
 t.repo_id, r.name, t.created_at, t.updated_at, t.started_at, t.finished_at
FROM coding_tasks t JOIN coding_repos r ON r.id = t.repo_id
WHERE t.created_at >= ? AND t.created_at < ? ORDER BY t.id DESC`, from, to)
	if err != nil {
		return nil, err
	}
	var rows []taskRow
	byID := map[int64]*taskRow{}
	for rs.Next() {
		var r taskRow
		if err := rs.Scan(&r.ID, &r.Prompt, &r.IssueKey, &r.Executor, &r.Status, &r.PRURL, &r.AgentID,
			&r.RepoID, &r.RepoName, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.FinishedAt); err != nil {
			rs.Close()
			return nil, err
		}
		rows = append(rows, r)
	}
	if err := rs.Err(); err != nil {
		rs.Close()
		return nil, err
	}
	rs.Close()
	for i := range rows {
		byID[rows[i].ID] = &rows[i]
	}
	if len(rows) == 0 {
		return rows, nil
	}

	// Cost, tokens and the time of each run come from the AI usage records
	// the coding module writes for every run (source coding, ref the task id).
	us, err := m.d.DB.QueryContext(ctx, `SELECT ref, COUNT(*), SUM(CASE WHEN cost IS NULL THEN 0 ELSE 1 END), COALESCE(SUM(cost), 0),
 COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(duration_ms), 0)
FROM ai_usage WHERE source = 'coding' AND ref IN (SELECT CAST(id AS TEXT) FROM coding_tasks WHERE created_at >= ? AND created_at < ?)
GROUP BY ref`, from, to)
	if err != nil {
		return nil, err
	}
	for us.Next() {
		var ref string
		var n, costRows int
		var cost float64
		var in, out, ms int64
		if err := us.Scan(&ref, &n, &costRows, &cost, &in, &out, &ms); err != nil {
			us.Close()
			return nil, err
		}
		id, perr := strconv.ParseInt(ref, 10, 64)
		if r := byID[id]; perr == nil && r != nil {
			r.UsageRows, r.CostRows, r.Cost, r.InputTokens, r.OutputTokens, r.UsageMs = n, costRows, cost, in, out, ms
		}
	}
	if err := us.Err(); err != nil {
		us.Close()
		return nil, err
	}
	us.Close()

	// One "done" event with an exit code is written each time the executor
	// ends. Builds also write a "done" event, but without an exit code.
	es, err := m.d.DB.QueryContext(ctx, `SELECT task_id, COUNT(*) FROM coding_task_events
WHERE kind = 'done' AND data LIKE '%"_exitCode":%'
 AND task_id IN (SELECT id FROM coding_tasks WHERE created_at >= ? AND created_at < ?) GROUP BY task_id`, from, to)
	if err != nil {
		return nil, err
	}
	for es.Next() {
		var id int64
		var n int
		if err := es.Scan(&id, &n); err != nil {
			es.Close()
			return nil, err
		}
		if r := byID[id]; r != nil {
			r.Runs = n
		}
	}
	if err := es.Err(); err != nil {
		es.Close()
		return nil, err
	}
	es.Close()

	os, err := m.d.DB.QueryContext(ctx, `SELECT task_id, outcome, at FROM codingroi_outcomes
WHERE task_id IN (SELECT id FROM coding_tasks WHERE created_at >= ? AND created_at < ?)`, from, to)
	if err != nil {
		return nil, err
	}
	for os.Next() {
		var id int64
		var outcome string
		var at time.Time
		if err := os.Scan(&id, &outcome, &at); err != nil {
			os.Close()
			return nil, err
		}
		if r := byID[id]; r != nil {
			r.Outcome, r.OutcomeAt = outcome, at
		}
	}
	if err := os.Err(); err != nil {
		os.Close()
		return nil, err
	}
	os.Close()
	for i := range rows {
		// A task that was started and stopped without a done event (the
		// server restarted) still ran once.
		if rows[i].Runs == 0 && rows[i].StartedAt != nil && rows[i].FinishedAt != nil {
			rows[i].Runs = 1
		}
	}
	return rows, nil
}

// agentNames maps AI agent ids to names.
func (m *Module) agentNames(ctx context.Context) (map[int64]string, error) {
	rs, err := m.d.DB.QueryContext(ctx, `SELECT id, name FROM ai_agents`)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	out := map[int64]string{}
	for rs.Next() {
		var id int64
		var name string
		if err := rs.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rs.Err()
}

// screenMinutes reads the computer time records between from and to, one entry
// per minute (several computers in the same minute count once).
func (m *Module) screenMinutes(ctx context.Context, from, to time.Time) ([]screenMinute, error) {
	rs, err := m.d.DB.QueryContext(ctx, `SELECT minute, MAX(CASE WHEN category = 'coding' THEN 1 ELSE 0 END)
FROM screen_minutes WHERE minute >= ? AND minute < ? GROUP BY minute ORDER BY minute`, from.Unix()/60, to.Unix()/60+1)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []screenMinute
	for rs.Next() {
		var s screenMinute
		var coding int
		if err := rs.Scan(&s.Minute, &coding); err != nil {
			return nil, err
		}
		s.Coding = coding == 1
		out = append(out, s)
	}
	return out, rs.Err()
}

// outcomeOf says how a task ended. A recorded merge wins over everything.
func outcomeOf(r taskRow) api.CodingROIOutcome {
	switch {
	case r.Outcome == "merged":
		return "merged"
	case r.Outcome == "closed", r.Status == "discarded":
		return "reverted"
	}
	switch r.Status {
	case "canceled":
		return "canceled"
	case "failed":
		return "failed"
	case "committed", "pushed", "pr_opened":
		return "waiting"
	}
	return "active"
}

// reviewWindowOf is the stretch in which the user looked at the result: from
// the end of the last run to the merge, the close of the pull request or the
// discard. A task without an end is still being looked at, so the window runs
// to now (on is true). Failed and canceled tasks have none.
func reviewWindowOf(r taskRow, now time.Time) (w window, ok, on bool) {
	if r.FinishedAt == nil {
		return window{}, false, false
	}
	w.Start = *r.FinishedAt
	switch {
	case r.Outcome != "":
		w.End = r.OutcomeAt
	case r.Status == "discarded":
		w.End = r.UpdatedAt
	case r.Status == "review" || r.Status == "committed" || r.Status == "pushed" || r.Status == "pr_opened":
		w.End, on = now, true
	default:
		return window{}, false, false
	}
	w, cut := capWindow(w)
	if cut {
		on = false
	}
	return w, true, on
}

// runSecondsOf is how long the AI ran. One run is the time between the start
// and the end of the task. After several runs those two times include the
// waiting in between, so the length of each run is added up from the usage
// records instead.
func runSecondsOf(r taskRow) *int64 {
	if r.Runs <= 1 && r.StartedAt != nil && r.FinishedAt != nil {
		v := int64(math.Max(0, r.FinishedAt.Sub(*r.StartedAt).Seconds()))
		return &v
	}
	if r.UsageRows > 0 && r.UsageMs > 0 {
		v := r.UsageMs / 1000
		return &v
	}
	return nil
}

func taskOf(r taskRow, agents map[int64]string) api.CodingROITask {
	t := api.CodingROITask{
		Id: r.ID, Title: titleOf(r.Prompt), RepoName: r.RepoName, Executor: api.CodingROITaskExecutor(r.Executor),
		AgentId: r.AgentID, Outcome: outcomeOf(r), Status: r.Status, CreatedAt: r.CreatedAt, StartedAt: r.StartedAt,
		Runs: r.Runs, ReviewState: api.CodingROIReviewState("none"),
	}
	if r.IssueKey != "" {
		t.IssueKey = &r.IssueKey
	}
	if r.PRURL != "" {
		t.PrUrl = &r.PRURL
	}
	if r.AgentID != nil {
		name, ok := agents[*r.AgentID]
		if !ok {
			name = "Agent #" + strconv.FormatInt(*r.AgentID, 10)
		}
		t.AgentName = &name
	}
	if r.Outcome == "merged" {
		at := r.OutcomeAt
		t.MergedAt = &at
	}
	if r.Runs > 0 {
		v := r.Runs - 1
		t.Rounds = &v
	}
	if r.CostRows > 0 {
		v := round(r.Cost, 4)
		t.Cost = &v
		t.CostPartial = r.CostRows < r.UsageRows
	}
	if r.UsageRows > 0 {
		in, out := r.InputTokens, r.OutputTokens
		t.InputTokens, t.OutputTokens = &in, &out
	}
	t.RunSeconds = runSecondsOf(r)
	return t
}

// titleOf is the first non-empty line of the prompt, at most 120 characters.
func titleOf(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			if utf8.RuneCountInString(line) > 120 {
				return string([]rune(line)[:119]) + "…"
			}
			return line
		}
	}
	return ""
}

func round(v float64, places int) float64 {
	p := math.Pow10(places)
	return math.Round(v*p) / p
}

// groupKey says which group a task belongs to.
func groupKey(t api.CodingROITask, by string) (key, label string) {
	switch by {
	case "tool":
		if t.Executor == "codex" {
			return "codex", "Codex"
		}
		return "claude", "Claude Code"
	case "agent":
		if t.AgentId == nil {
			return "", "手动任务"
		}
		return strconv.FormatInt(*t.AgentId, 10), deref(t.AgentName)
	}
	return repoKeyOf(t), t.RepoName
}

// repoKeyOf is the repository name. Two registered repositories with the same
// name are one group, which is what the user sees in the list anyway.
func repoKeyOf(t api.CodingROITask) string { return t.RepoName }

func groupTasks(tasks []api.CodingROITask, by string) []api.CodingROIGroup {
	byKey := map[string][]api.CodingROITask{}
	labels := map[string]string{}
	for _, t := range tasks {
		k, l := groupKey(t, by)
		byKey[k] = append(byKey[k], t)
		labels[k] = l
	}
	out := make([]api.CodingROIGroup, 0, len(byKey))
	for k, list := range byKey {
		out = append(out, summarize(k, labels[k], list))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tasks != out[j].Tasks {
			return out[i].Tasks > out[j].Tasks
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// summarize adds up a group. A value that some tasks have no record of is the
// sum of the ones that have, and the missing count says how many were left out.
func summarize(key, label string, tasks []api.CodingROITask) api.CodingROIGroup {
	g := api.CodingROIGroup{Key: key, Label: label, Tasks: len(tasks)}
	var cost, review float64
	var costN, tokenN, runN, reviewN int
	var in, out, run int64
	for _, t := range tasks {
		switch t.Outcome {
		case "merged":
			g.Merged++
		case "reverted":
			g.Reverted++
		case "canceled":
			g.Canceled++
		case "failed":
			g.Failed++
		case "waiting":
			g.Waiting++
		default:
			g.Active++
		}
		if t.Rounds != nil {
			g.Rounds += *t.Rounds
		}
		started := t.StartedAt != nil
		if t.Cost != nil {
			cost += *t.Cost
			costN++
		} else if started {
			g.CostMissing++
		}
		if t.InputTokens != nil && t.OutputTokens != nil {
			in += *t.InputTokens
			out += *t.OutputTokens
			tokenN++
		} else if started {
			g.TokenMissing++
		}
		if t.RunSeconds != nil {
			run += *t.RunSeconds
			runN++
		} else if started {
			g.RunMissing++
		}
		if t.ReviewMinutes != nil {
			review += *t.ReviewMinutes
			reviewN++
		} else if t.ReviewState == "no_data" {
			g.ReviewMissing++
		}
	}
	decided := g.Merged + g.Reverted + g.Canceled + g.Failed
	if decided > 0 {
		v := round(float64(g.Merged)/float64(decided), 3)
		g.MergeRate = &v
	}
	if costN > 0 {
		v := round(cost, 4)
		g.Cost = &v
	}
	if tokenN > 0 {
		g.InputTokens, g.OutputTokens = &in, &out
	}
	if runN > 0 {
		g.RunSeconds = &run
	}
	if reviewN > 0 {
		v := round(review, 1)
		g.ReviewMinutes = &v
	}
	if g.Merged > 0 {
		n := float64(g.Merged)
		if costN > 0 {
			v := round(cost/n, 4)
			g.CostPerMerge = &v
		}
		if runN > 0 {
			v := round(float64(run)/n, 1)
			g.RunSecondsPerMerge = &v
		}
		if reviewN > 0 {
			v := round(review/n, 1)
			g.ReviewMinutesPerMerge = &v
		}
	}
	return g
}
