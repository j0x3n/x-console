package dayplan

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/db"
)

var (
	errBadDate   = httpx.Invalid("日期要写成 YYYY-MM-DD")
	errPast      = httpx.Invalid("不能给过去的日子排计划")
	errConfirmed = httpx.NewError(http.StatusConflict, "conflict", "这一天的计划已经确认了。要重排请选“重新排”")
)

// itemRow is an item before it is stored.
type itemRow struct {
	Title, Kind, Ref, Start, End, Reason string
	Done, Notified                       bool
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// savePlan writes a plan: the header and all its items, in one transaction.
func (m *Module) savePlan(ctx context.Context, date, status, source string, items []itemRow) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	now := m.now().UTC()
	var confirmed *time.Time
	if status == "confirmed" {
		confirmed = &now
	}
	if err := q.UpsertDayPlan(ctx, db.UpsertDayPlanParams{Date: date, Status: status, Source: source, GeneratedAt: now, ConfirmedAt: confirmed}); err != nil {
		return err
	}
	if err := q.DeleteDayPlanItems(ctx, date); err != nil {
		return err
	}
	for i, it := range items {
		if err := q.InsertDayPlanItem(ctx, db.InsertDayPlanItemParams{PlanDate: date, Position: int64(i), Title: it.Title, Kind: it.Kind,
			Ref: it.Ref, StartTime: it.Start, EndTime: it.End, Reason: it.Reason, Done: b2i(it.Done), Notified: b2i(it.Notified)}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func toItem(r db.DayplanItem) api.DayPlanItem {
	return api.DayPlanItem{Id: r.ID, Position: int(r.Position), Title: r.Title, Kind: api.DayPlanItemKind(r.Kind), Ref: r.Ref,
		Start: r.StartTime, End: r.EndTime, Reason: r.Reason, Done: r.Done == 1}
}

// load reads a plan. ErrNotFound means the date has none.
func (m *Module) load(ctx context.Context, date string) (api.DayPlan, error) {
	row, err := m.q.GetDayPlan(ctx, date)
	if errors.Is(err, sql.ErrNoRows) {
		return api.DayPlan{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.DayPlan{}, err
	}
	rows, err := m.q.ListDayPlanItems(ctx, date)
	if err != nil {
		return api.DayPlan{}, err
	}
	out := api.DayPlan{Date: row.Date, Status: api.DayPlanStatus(row.Status), Source: api.DayPlanSource(row.Source), GeneratedAt: row.GeneratedAt,
		ConfirmedAt: row.ConfirmedAt, Items: make([]api.DayPlanItem, 0, len(rows))}
	for _, r := range rows {
		out.Items = append(out.Items, toItem(r))
	}
	return out, nil
}

func (m *Module) nowUTC() *time.Time {
	t := m.now().UTC()
	return &t
}

func doneParams(id int64, done bool) db.SetDayPlanItemDoneParams {
	return db.SetDayPlanItemDoneParams{Done: b2i(done), ID: id}
}
