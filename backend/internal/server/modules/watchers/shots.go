package watchers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
)

// B234: the screenshots of a watcher can be cleaned up. A shot belongs to a
// run through the steps of that run (the shotId of each step), so the table
// of shots has no run column.

const (
	defaultRunPage = 10
	maxRunPage     = 50
)

// stepShots lists the screenshots a run's steps point at.
func stepShots(raw string) []int64 {
	var steps []step
	if json.Unmarshal([]byte(raw), &steps) != nil {
		return nil
	}
	var ids []int64
	for _, s := range steps {
		if s.ShotID != nil {
			ids = append(ids, *s.ShotID)
		}
	}
	return ids
}

// stripShots takes the dropped screenshots out of the steps. changed says
// whether anything was taken out.
func stripShots(raw string, drop map[int64]bool) (string, bool) {
	var steps []step
	if json.Unmarshal([]byte(raw), &steps) != nil {
		return raw, false
	}
	changed := false
	for i := range steps {
		if steps[i].ShotID != nil && drop[*steps[i].ShotID] {
			steps[i].ShotID = nil
			changed = true
		}
	}
	if !changed {
		return raw, false
	}
	b, _ := json.Marshal(steps)
	return string(b), true
}

// keepLastShot lets the steps of a failed run point at the last screenshot
// only. It returns the ones it took out.
func keepLastShot(steps []step) []int64 {
	last := -1
	for i := range steps {
		if steps[i].ShotID != nil {
			last = i
		}
	}
	var dropped []int64
	for i := range steps {
		if i != last && steps[i].ShotID != nil {
			dropped = append(dropped, *steps[i].ShotID)
			steps[i].ShotID = nil
		}
	}
	return dropped
}

// dropRunShots deletes the screenshots a failed run no longer keeps. A
// screenshot a change points at stays.
func (m *Module) dropRunShots(ctx context.Context, wid int64, ids []int64) {
	for _, id := range ids {
		_, _ = m.d.DB.ExecContext(ctx, `DELETE FROM watcher_shots WHERE id = ? AND watcher_id = ?
 AND id NOT IN (SELECT shot_id FROM watcher_changes WHERE shot_id IS NOT NULL)`, id, wid)
	}
}

// shotScan is what a cleanup decides from.
type shotScan struct {
	sizes        map[int64]int64
	created      map[int64]time.Time
	runs         []db.ListRunStepsRow // newest first
	changeShots  map[int64]bool
	runningSince *time.Time // start of the run in progress, if any
}

func (s *shotScan) all() map[int64]bool {
	out := make(map[int64]bool, len(s.sizes))
	for id := range s.sizes {
		out[id] = true
	}
	return out
}

// dropShots deletes the screenshots pick chooses, in one transaction. The
// shots of a run in progress stay: they are the ones made since it started.
func (m *Module) dropShots(ctx context.Context, wid int64, pick func(s *shotScan) map[int64]bool) (api.WatcherShotsDeleted, error) {
	out := api.WatcherShotsDeleted{}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback() }()
	q := m.q.WithTx(tx)

	scan := &shotScan{sizes: map[int64]int64{}, created: map[int64]time.Time{}, changeShots: map[int64]bool{}}
	sizes, err := q.ListShotSizes(ctx, wid)
	if err != nil {
		return out, err
	}
	for _, s := range sizes {
		scan.sizes[s.ID], scan.created[s.ID] = s.Size, s.CreatedAt
	}
	if scan.runs, err = q.ListRunSteps(ctx, wid); err != nil {
		return out, err
	}
	for _, r := range scan.runs {
		if r.Outcome == "running" {
			t := r.StartedAt
			scan.runningSince = &t
		}
	}
	cs, err := q.ListChangeShots(ctx, wid)
	if err != nil {
		return out, err
	}
	for _, id := range cs {
		if id != nil {
			scan.changeShots[*id] = true
		}
	}

	drop := map[int64]bool{}
	for id := range pick(scan) {
		if _, ok := scan.sizes[id]; !ok {
			continue // not a shot of this watcher, or gone already
		}
		if scan.runningSince != nil && !scan.created[id].Before(*scan.runningSince) {
			continue
		}
		drop[id] = true
	}
	for id := range drop {
		if err := q.ClearChangeShot(ctx, &id); err != nil {
			return out, err
		}
		if err := q.DeleteShot(ctx, db.DeleteShotParams{ID: id, WatcherID: wid}); err != nil {
			return out, err
		}
		out.Deleted++
		out.Bytes += scan.sizes[id]
	}
	if len(drop) > 0 {
		for _, r := range scan.runs {
			if r.Outcome == "running" {
				continue
			}
			if steps, changed := stripShots(r.Steps, drop); changed {
				if err := q.SetRunSteps(ctx, db.SetRunStepsParams{Steps: steps, ID: r.ID}); err != nil {
					return out, err
				}
			}
		}
	}
	return out, tx.Commit()
}

func isSuccess(outcome string) bool {
	return outcome == "same" || outcome == "changed" || outcome == "ok"
}

// CleanupWatcherShots implements api.ServerInterface.
func (m *Module) CleanupWatcherShots(w http.ResponseWriter, r *http.Request, id api.WatcherId) {
	var body struct {
		Keep string `json:"keep"`
	}
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.Keep != "latest" && body.Keep != "none" {
		fail(w, r, httpx.Invalid("keep 只能是 latest 或 none"))
		return
	}
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	res, err := m.dropShots(r.Context(), id, func(s *shotScan) map[int64]bool {
		del := s.all()
		if body.Keep == "none" {
			return del
		}
		for id := range s.changeShots {
			delete(del, id)
		}
		for _, run := range s.runs {
			if isSuccess(run.Outcome) {
				for _, sid := range stepShots(run.Steps) {
					delete(del, sid)
				}
				break
			}
		}
		return del
	})
	m.d.Audit.Record(r.Context(), "watcher.shots", strconv.FormatInt(id, 10),
		map[string]any{"keep": body.Keep, "deleted": res.Deleted, "bytes": res.Bytes}, err)
	if fail(w, r, err) {
		return
	}
	m.publish(id)
	httpx.JSON(w, http.StatusOK, res)
}

// DeleteWatcherRunShots implements api.ServerInterface.
func (m *Module) DeleteWatcherRunShots(w http.ResponseWriter, r *http.Request, id api.WatcherId, runID int64) {
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	run, err := m.q.GetRun(r.Context(), runID)
	if err != nil || run.WatcherID != id {
		fail(w, r, httpx.ErrNotFound)
		return
	}
	if run.Outcome == "running" {
		fail(w, r, httpx.NewError(http.StatusConflict, "busy", "这次检查还没结束，等它结束再删"))
		return
	}
	mine := map[int64]bool{}
	for _, sid := range stepShots(run.Steps) {
		mine[sid] = true
	}
	res, err := m.dropShots(r.Context(), id, func(*shotScan) map[int64]bool { return mine })
	m.d.Audit.Record(r.Context(), "watcher.shots", strconv.FormatInt(id, 10),
		map[string]any{"run": runID, "deleted": res.Deleted, "bytes": res.Bytes}, err)
	if fail(w, r, err) {
		return
	}
	m.publish(id)
	httpx.JSON(w, http.StatusOK, res)
}

// ListWatcherRuns implements api.ServerInterface.
func (m *Module) ListWatcherRuns(w http.ResponseWriter, r *http.Request, id api.WatcherId, params api.ListWatcherRunsParams) {
	if _, err := m.get(r.Context(), id); fail(w, r, err) {
		return
	}
	limit := defaultRunPage
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 || limit > maxRunPage {
		fail(w, r, httpx.Invalid("limit 要在 1 到 50 之间"))
		return
	}
	before := int64(1<<63 - 1)
	if params.Before != nil {
		before = *params.Before
	}
	rows, err := m.q.ListRunsPage(r.Context(), db.ListRunsPageParams{WatcherID: id, ID: before, Limit: int64(limit) + 1})
	if fail(w, r, err) {
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]api.WatcherRun, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAPIRun(row))
	}
	httpx.JSON(w, http.StatusOK, api.WatcherRunPage{Items: items, HasMore: hasMore})
}

func toAPIRun(run db.WatcherRun) api.WatcherRun {
	item := api.WatcherRun{Id: run.ID, Kind: api.WatcherRunKind(run.Kind), StartedAt: run.StartedAt, FinishedAt: run.FinishedAt,
		Outcome: api.WatcherRunOutcome(run.Outcome), Detail: run.Detail, Tokens: run.Tokens, Steps: []api.WatcherStep{}}
	_ = json.Unmarshal([]byte(run.Steps), &item.Steps)
	if item.Steps == nil {
		item.Steps = []api.WatcherStep{}
	}
	return item
}

func toAPILastChange(summary string, important int64, at time.Time) *api.WatcherLastChange {
	return &api.WatcherLastChange{Summary: summary, Important: important == 1, CreatedAt: at}
}
