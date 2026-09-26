package linear

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// How the sync decides, per issue:
//
// linear_issues remembers both sides' updated times from the last time the
// issue was in sync. A side changed if its time is newer than remembered.
// Only Linear changed → copy to local. Only local changed → push to Linear.
// Both changed → the newer updated time wins (a conflict). The local copy
// keeps Linear's updatedAt (IssueSync.Upsert stores the given time) and
// publishes issue.synced, which the push ignores, so nothing echoes back.

// Status mapping by Linear state type.
func localStatus(stateType string) string {
	switch stateType {
	case "unstarted":
		return "todo"
	case "started":
		return "in_progress"
	case "completed":
		return "done"
	case "canceled":
		return "canceled"
	}
	return "backlog" // backlog, triage
}

func linearType(status string) string {
	switch status {
	case "backlog":
		return "backlog"
	case "in_progress", "in_review":
		return "started"
	case "done":
		return "completed"
	case "canceled":
		return "canceled"
	}
	return "unstarted" // todo
}

func reviewNamed(name string) bool { return strings.Contains(strings.ToLower(name), "review") }

// remoteStatus is the local status for a Linear state. A started state
// named like "In Review" means in_review.
func remoteStatus(st lnState) string {
	if st.Type == "started" && reviewNamed(st.Name) {
		return "in_review"
	}
	return localStatus(st.Type)
}

// hasReviewState reports whether the team has a started state for review.
func (m *Module) hasReviewState(teamID string) bool {
	for _, s := range m.states[teamID] {
		if s.Type == "started" && reviewNamed(s.Name) {
			return true
		}
	}
	return false
}

// fits reports whether a local status already matches a Linear state, so
// neither side needs to change. in_progress and in_review both fit any
// started state when the team has no review state to tell them apart.
func (m *Module) fits(status string, st lnState, teamID string) bool {
	switch {
	case remoteStatus(st) == status:
		return true
	case st.Type == "triage" && status == "backlog":
		return true
	case linearType(status) == st.Type && st.Type == "started":
		return !m.hasReviewState(teamID)
	}
	return false
}

// statusFor is the local status after taking the Linear state: the current
// one when it fits, otherwise the mapped one.
func (m *Module) statusFor(st lnState, teamID, current string) string {
	if current != "" && m.fits(current, st, teamID) {
		return current
	}
	return remoteStatus(st)
}

func closedType(t string) bool { return t == "completed" || t == "canceled" }

const dateLayout = "2006-01-02"

func (m *Module) loc() *time.Location {
	if m.d.Config.Location != nil {
		return m.d.Config.Location
	}
	return time.Local
}

func (m *Module) dueOf(s *string) *time.Time {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.ParseInLocation(dateLayout, *s, m.loc())
	if err != nil {
		return nil
	}
	return &t
}

func (m *Module) dateString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.In(m.loc()).Format(dateLayout)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// run collects the result of one sync pass.
type run struct {
	res api.LinearSyncResult
}

func newRun(at time.Time) *run {
	return &run{res: api.LinearSyncResult{At: at, Ok: true, Errors: []string{}}}
}

func (r *run) errorf(format string, args ...any) {
	r.res.Ok = false
	if len(r.res.Errors) < 20 {
		r.res.Errors = append(r.res.Errors, fmt.Sprintf(format, args...))
	}
}

// sync pulls every mapped team and pushes local changes the pull did not
// cover. Problems with single issues are collected in the result.
func (m *Module) sync(ctx context.Context) (api.LinearSyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return api.LinearSyncResult{}, err
	}
	is, err := m.issueSync()
	if err != nil {
		return api.LinearSyncResult{}, err
	}
	m.setSyncing(true)
	defer m.setSyncing(false)

	started := m.now()
	r := newRun(started)
	c := m.client(cfg)
	teams, err := m.q.ListTeams(ctx)
	if err != nil {
		return r.res, err
	}
	handled := map[string]bool{}
	for _, t := range teams {
		if err := m.pullTeam(ctx, c, is, t, handled, r); err != nil {
			if ctx.Err() != nil {
				return r.res, ctx.Err()
			}
			r.errorf("%s：%v", t.TeamKey, err)
		}
	}
	if len(teams) > 0 && r.res.Ok {
		m.pushLocal(ctx, c, is, teams, handled, r)
	}
	if r.res.Ok {
		// Next time only look at local issues changed from now on.
		if err := m.d.Settings.Set(ctx, keyLocalCursor, started.Add(-time.Second)); err != nil {
			return r.res, err
		}
	}
	r.res.At = m.now()
	if err := m.d.Settings.Set(ctx, keyLastSync, r.res); err != nil {
		return r.res, err
	}
	if !r.res.Ok {
		m.log.Warn("linear sync finished with errors", "errors", strings.Join(r.res.Errors, "; "))
	}
	m.d.Bus.Publish("linear.synced", r.res)
	return r.res, nil
}

// teamStates returns the workflow states of a team, cached until refresh.
func (m *Module) teamStates(ctx context.Context, c *gqlClient, teamID string, refresh bool) ([]lnState, error) {
	if s, ok := m.states[teamID]; ok && !refresh {
		return s, nil
	}
	s, err := c.teamStates(ctx, teamID)
	if err != nil {
		return nil, err
	}
	m.states[teamID] = s
	return s, nil
}

func (m *Module) pullTeam(ctx context.Context, c *gqlClient, is contracts.IssueSync, t db.LinearTeam, handled map[string]bool, r *run) error {
	if _, err := m.teamStates(ctx, c, t.TeamID, true); err != nil {
		return err
	}
	since := time.Unix(0, 0).UTC()
	if t.Cursor != nil {
		since = *t.Cursor
	}
	list, err := c.issuesSince(ctx, t.TeamID, since)
	if err != nil {
		return err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].UpdatedAt.Before(list[j].UpdatedAt) })
	cursor, failed := since, false
	for _, ri := range list {
		handled[ri.ID] = true
		if err := m.reconcile(ctx, c, is, t.ProjectID, ri, r); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			r.errorf("%s：%v", ri.Identifier, err)
			failed = true // retry from here next time
			continue
		}
		if !failed && ri.UpdatedAt.After(cursor) {
			cursor = ri.UpdatedAt
		}
	}
	if !cursor.Equal(since) {
		c := cursor.UTC()
		return m.q.SetTeamCursor(ctx, db.SetTeamCursorParams{Cursor: &c, TeamID: t.TeamID})
	}
	return nil
}

// pushLocal pushes linked local issues changed since the last pass.
func (m *Module) pushLocal(ctx context.Context, c *gqlClient, is contracts.IssueSync, teams []db.LinearTeam, handled map[string]bool, r *run) {
	var since time.Time
	if err := m.d.Settings.Get(ctx, keyLocalCursor, &since); err != nil && !errors.Is(err, settings.ErrNotSet) {
		r.errorf("%v", err)
		return
	}
	ids := make([]int64, len(teams))
	for i, t := range teams {
		ids[i] = t.ProjectID
	}
	changed, err := is.ChangedSince(ctx, ids, since)
	if err != nil {
		r.errorf("%v", err)
		return
	}
	for _, li := range changed {
		if li.ExternalSource != source || li.ExternalID == "" || handled[li.ExternalID] {
			continue
		}
		if err := m.pushOne(ctx, c, is, li, r); err != nil {
			r.errorf("%s：%v", li.Key, err)
		}
	}
}

// pushOne reconciles a local issue whose Linear copy did not show up in the
// pull, if it changed locally since the last sync.
func (m *Module) pushOne(ctx context.Context, c *gqlClient, is contracts.IssueSync, li contracts.SyncIssue, r *run) error {
	rec, hasRec, err := m.record(ctx, li.ExternalID)
	if err != nil {
		return err
	}
	if hasRec && !li.UpdatedAt.After(rec.LocalUpdatedAt) {
		return nil
	}
	ri, err := c.issue(ctx, li.ExternalID)
	if err != nil {
		return err
	}
	if ri == nil {
		return nil // deleted in Linear
	}
	return m.reconcile(ctx, c, is, 0, *ri, r)
}

// pushKey handles a local change event.
func (m *Module) pushKey(ctx context.Context, key, externalID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg, err := m.requireConfigured(ctx)
	if err != nil {
		return err
	}
	is, err := m.issueSync()
	if err != nil {
		return err
	}
	if externalID == "" {
		rec, err := m.q.GetIssueByKey(ctx, key)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		externalID = rec.LinearID
	}
	li, found, err := is.FindByExternal(ctx, source, externalID)
	if err != nil || !found {
		return err
	}
	if _, err := m.q.GetTeamByProject(ctx, li.ProjectID); errors.Is(err, sql.ErrNoRows) {
		return nil // the project is not synced with Linear
	} else if err != nil {
		return err
	}
	r := newRun(m.now())
	if err := m.pushOne(ctx, m.client(cfg), is, li, r); err != nil {
		return err
	}
	if !r.res.Ok {
		return errors.New(strings.Join(r.res.Errors, "; "))
	}
	return nil
}

func (m *Module) record(ctx context.Context, linearID string) (db.LinearIssue, bool, error) {
	rec, err := m.q.GetIssue(ctx, linearID)
	if errors.Is(err, sql.ErrNoRows) {
		return rec, false, nil
	}
	return rec, err == nil, err
}

func (m *Module) saveRecord(ctx context.Context, ri lnIssue, local contracts.SyncIssue) error {
	return m.q.UpsertIssue(ctx, db.UpsertIssueParams{
		LinearID: ri.ID, IssueKey: local.Key, Identifier: ri.Identifier,
		RemoteUpdatedAt: ri.UpdatedAt.UTC(), LocalUpdatedAt: local.UpdatedAt.UTC(), SyncedAt: m.now(),
	})
}

// same reports whether both sides already hold the same content.
func (m *Module) same(local contracts.SyncIssue, ri lnIssue) bool {
	return local.Title == ri.Title &&
		local.Description == deref(ri.Description) &&
		local.Priority == int(ri.Priority) &&
		m.dateString(local.DueDate) == deref(ri.DueDate) &&
		m.fits(local.Status, ri.State, ri.Team.ID)
}

// fromRemote is the local issue as Linear has it.
func (m *Module) fromRemote(projectID int64, key, current string, ri lnIssue) contracts.SyncIssue {
	return contracts.SyncIssue{
		ProjectID: projectID, Key: key, Title: ri.Title, Description: deref(ri.Description),
		Status: m.statusFor(ri.State, ri.Team.ID, current), Priority: int(ri.Priority), DueDate: m.dueOf(ri.DueDate),
		ExternalSource: source, ExternalID: ri.ID, UpdatedAt: ri.UpdatedAt,
	}
}

// reconcile brings one issue in sync. projectID is where to create the
// issue locally when it does not exist yet (0 means do not create).
func (m *Module) reconcile(ctx context.Context, c *gqlClient, is contracts.IssueSync, projectID int64, ri lnIssue, r *run) error {
	if _, err := m.teamStates(ctx, c, ri.Team.ID, false); err != nil {
		return err
	}
	local, found, err := is.FindByExternal(ctx, source, ri.ID)
	if err != nil {
		return err
	}
	if !found {
		// Finished issues are not imported; only open work comes over.
		if projectID == 0 || closedType(ri.State.Type) {
			return nil
		}
		out, err := is.Upsert(ctx, m.fromRemote(projectID, "", "", ri))
		if err != nil {
			return err
		}
		r.res.Created++
		return m.saveRecord(ctx, ri, out)
	}
	rec, hasRec, err := m.record(ctx, ri.ID)
	if err != nil {
		return err
	}
	remoteChanged := !hasRec || ri.UpdatedAt.After(rec.RemoteUpdatedAt)
	localChanged := !hasRec || local.UpdatedAt.After(rec.LocalUpdatedAt)
	if !remoteChanged && !localChanged {
		return nil
	}
	if m.same(local, ri) {
		return m.saveRecord(ctx, ri, local)
	}
	remoteWins := remoteChanged
	if remoteChanged && localChanged {
		remoteWins = !local.UpdatedAt.After(ri.UpdatedAt)
		if hasRec {
			// Without a record this is the first sync of a linked issue,
			// not a real conflict.
			r.res.Conflicts++
			winner := "本地"
			if remoteWins {
				winner = "Linear"
			}
			m.log.Info("linear conflict", "issue", local.Key, "linear", ri.Identifier, "winner", winner)
		}
	}
	if remoteWins {
		out, err := is.Upsert(ctx, m.fromRemote(local.ProjectID, local.Key, local.Status, ri))
		if err != nil {
			return err
		}
		r.res.Pulled++
		return m.saveRecord(ctx, ri, out)
	}
	updated, err := m.push(ctx, c, local, ri)
	if err != nil {
		return err
	}
	r.res.Pushed++
	return m.saveRecord(ctx, updated, local)
}

// push writes the local fields that differ to Linear.
func (m *Module) push(ctx context.Context, c *gqlClient, local contracts.SyncIssue, ri lnIssue) (lnIssue, error) {
	input := map[string]any{}
	if local.Title != ri.Title {
		input["title"] = local.Title
	}
	if local.Description != deref(ri.Description) {
		input["description"] = local.Description
	}
	if local.Priority != int(ri.Priority) {
		input["priority"] = local.Priority
	}
	if due := m.dateString(local.DueDate); due != deref(ri.DueDate) {
		if due == "" {
			input["dueDate"] = nil
		} else {
			input["dueDate"] = due
		}
	}
	if !m.fits(local.Status, ri.State, ri.Team.ID) {
		stateID, err := m.stateFor(ctx, c, ri.Team.ID, local.Status)
		if err != nil {
			return lnIssue{}, err
		}
		input["stateId"] = stateID
	}
	if len(input) == 0 {
		return ri, nil
	}
	return c.updateIssue(ctx, ri.ID, input)
}

// stateFor picks the Linear workflow state for a local status: the first
// state of the matching type. in_review prefers a state named like
// "review"; in_progress avoids one.
func (m *Module) stateFor(ctx context.Context, c *gqlClient, teamID, status string) (string, error) {
	states, err := m.teamStates(ctx, c, teamID, false)
	if err != nil {
		return "", err
	}
	pick := func(t string) string {
		var best *lnState
		score := func(s *lnState) int {
			switch {
			case status == "in_review" && reviewNamed(s.Name), status != "in_review" && !reviewNamed(s.Name):
				return 0
			}
			return 1
		}
		for i := range states {
			s := &states[i]
			if s.Type != t {
				continue
			}
			if best == nil || score(s) < score(best) || score(s) == score(best) && s.Position < best.Position {
				best = s
			}
		}
		if best == nil {
			return ""
		}
		return best.ID
	}
	want := linearType(status)
	if id := pick(want); id != "" {
		return id, nil
	}
	if want == "backlog" {
		if id := pick("unstarted"); id != "" {
			return id, nil
		}
	}
	return "", fmt.Errorf("团队里没有 %s 类型的状态", want)
}

// recordError adds an error from an event push to the last result.
func (m *Module) recordError(ctx context.Context, msg string) {
	var last api.LinearSyncResult
	if err := m.d.Settings.Get(ctx, keyLastSync, &last); err != nil {
		last = api.LinearSyncResult{Errors: []string{}}
	}
	last.At = m.now()
	last.Ok = false
	if len(last.Errors) >= 20 {
		last.Errors = last.Errors[1:]
	}
	last.Errors = append(last.Errors, msg)
	_ = m.d.Settings.Set(ctx, keyLastSync, last)
	m.d.Bus.Publish("linear.synced", last)
}
