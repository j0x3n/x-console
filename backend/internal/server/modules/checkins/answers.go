package checkins

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/db"
)

const manualSlot = "manual"

func toAnswer(a db.CheckinAnswer) api.CheckinAnswer {
	return api.CheckinAnswer{Id: a.ID, QuestionId: a.QuestionID, Value: int(a.Value), Date: a.Date, Slot: a.Slot,
		AnsweredAt: a.AnsweredAt, Source: api.CheckinAnswerSource(a.Source)}
}

// record stores one answer after checking it. slot is "" or manual for a
// free answer; otherwise it must be one of the question's times.
func (m *Module) record(ctx context.Context, questionID int64, value int, slot, date, source string) (db.CheckinAnswer, error) {
	if value < 1 || value > 5 {
		return db.CheckinAnswer{}, httpx.Invalid("分数要在 1 到 5 之间")
	}
	q, err := m.q.GetCheckinQuestion(ctx, questionID)
	if errors.Is(err, sql.ErrNoRows) {
		return db.CheckinAnswer{}, httpx.ErrNotFound
	}
	if err != nil {
		return db.CheckinAnswer{}, err
	}
	now := m.now().In(m.loc())
	today := now.Format(dateLayout)
	if date == "" {
		date = today
	}
	d, err := time.ParseInLocation(dateLayout, date, m.loc())
	if err != nil {
		return db.CheckinAnswer{}, httpx.Invalid("日期要写成 YYYY-MM-DD")
	}
	if date > today {
		return db.CheckinAnswer{}, httpx.Invalid("不能回答未来的日期")
	}
	if d.Before(now.AddDate(-1, 0, 0)) {
		return db.CheckinAnswer{}, httpx.Invalid("只能补最近一年的回答")
	}
	if slot == "" {
		slot = manualSlot
	}
	if slot != manualSlot {
		if !hasTime(toQuestion(q), slot) {
			return db.CheckinAnswer{}, httpx.Invalid("这个问题没有这个时间点")
		}
	}
	row, err := m.q.UpsertCheckinAnswer(ctx, db.UpsertCheckinAnswerParams{QuestionID: questionID, Value: int64(value),
		Date: date, Slot: slot, AnsweredAt: m.now().UTC(), Source: source})
	if err != nil {
		return db.CheckinAnswer{}, err
	}
	m.d.Bus.Publish("checkin.answered", map[string]any{"questionId": questionID, "date": date, "slot": slot, "value": value})
	return row, nil
}

func (m *Module) PutCheckinAnswer(w http.ResponseWriter, r *http.Request) {
	var body api.CheckinAnswerInput
	if err := httpx.Decode(r, &body); fail(w, r, err) {
		return
	}
	slot, date := "", ""
	if body.Slot != nil {
		slot = *body.Slot
	}
	if body.Date != nil {
		date = *body.Date
	}
	row, err := m.record(r.Context(), body.QuestionId, body.Value, slot, date, "web")
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toAnswer(row))
}

func (m *Module) GetCheckinsToday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := m.now().In(m.loc())
	date := now.Format(dateLayout)
	qs, err := m.q.ListCheckinQuestions(ctx)
	if fail(w, r, err) {
		return
	}
	answers, err := m.q.ListCheckinAnswersOn(ctx, date)
	if fail(w, r, err) {
		return
	}
	type key struct {
		q    int64
		slot string
	}
	got := map[key]int{}
	for _, a := range answers {
		got[key{a.QuestionID, a.Slot}] = int(a.Value)
	}
	slots := []api.CheckinSlot{}
	for _, row := range qs {
		q := toQuestion(row)
		if !q.Enabled {
			continue
		}
		for _, t := range q.Times {
			s := api.CheckinSlot{QuestionId: q.Id, Label: q.Label, LowLabel: q.LowLabel, HighLabel: q.HighLabel, Slot: t}
			if v, ok := got[key{q.Id, t}]; ok {
				s.Status, s.Value = api.Answered, &v
			} else {
				at := m.slotTime(now, t)
				switch {
				case now.Before(at):
					s.Status = api.Upcoming
				case now.Before(at.Add(askWindow)):
					s.Status = api.Pending
				default:
					s.Status = api.Missed
				}
			}
			slots = append(slots, s)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"date": date, "slots": slots})
}

// slotTime is the moment of an HH:MM on the day of now.
func (m *Module) slotTime(now time.Time, hhmm string) time.Time {
	h, mi := int(hhmm[0]-'0')*10+int(hhmm[1]-'0'), int(hhmm[3]-'0')*10+int(hhmm[4]-'0')
	return time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, now.Location())
}

func hasTime(q api.CheckinQuestion, slot string) bool {
	for _, t := range q.Times {
		if t == slot {
			return true
		}
	}
	return false
}

func (m *Module) GetCheckinSeries(w http.ResponseWriter, r *http.Request, p api.GetCheckinSeriesParams) {
	days := 30
	if p.Days != nil {
		days = min(max(*p.Days, 1), 365)
	}
	to := m.now().In(m.loc())
	from := to.AddDate(0, 0, -(days - 1))
	items, err := m.series(r.Context(), from, to)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"from": from.Format(dateLayout), "to": to.Format(dateLayout), "items": items})
}

func (m *Module) series(ctx context.Context, from, to time.Time) ([]api.CheckinSeries, error) {
	qs, err := m.q.ListCheckinQuestions(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := m.q.ListCheckinAnswersBetween(ctx, db.ListCheckinAnswersBetweenParams{Date: from.Format(dateLayout), Date_2: to.Format(dateLayout)})
	if err != nil {
		return nil, err
	}
	type acc struct {
		sum   int
		count int
	}
	byDay := map[int64]map[string]*acc{}
	total := map[int64]*acc{}
	for _, a := range rows {
		if byDay[a.QuestionID] == nil {
			byDay[a.QuestionID] = map[string]*acc{}
			total[a.QuestionID] = &acc{}
		}
		d := byDay[a.QuestionID][a.Date]
		if d == nil {
			d = &acc{}
			byDay[a.QuestionID][a.Date] = d
		}
		d.sum += int(a.Value)
		d.count++
		total[a.QuestionID].sum += int(a.Value)
		total[a.QuestionID].count++
	}
	out := make([]api.CheckinSeries, 0, len(qs))
	for _, row := range qs {
		s := api.CheckinSeries{QuestionId: row.ID, Label: row.Label, Points: []struct {
			Average float32 `json:"average"`
			Count   int     `json:"count"`
			Date    string  `json:"date"`
		}{}}
		for d := from; !d.After(to) && d.Format(dateLayout) <= to.Format(dateLayout); d = d.AddDate(0, 0, 1) {
			if a := byDay[row.ID][d.Format(dateLayout)]; a != nil {
				s.Points = append(s.Points, struct {
					Average float32 `json:"average"`
					Count   int     `json:"count"`
					Date    string  `json:"date"`
				}{Average: float32(a.sum) / float32(a.count), Count: a.count, Date: d.Format(dateLayout)})
			}
		}
		if t := total[row.ID]; t != nil {
			s.Average, s.Count = float32(t.sum)/float32(t.count), t.count
		}
		out = append(out, s)
	}
	return out, nil
}

// Daily implements contracts.CheckinSeries.
func (m *Module) Daily(ctx context.Context, from, to time.Time) ([]contracts.CheckinDay, error) {
	qs, err := m.q.ListCheckinQuestions(ctx)
	if err != nil {
		return nil, err
	}
	labels := map[int64]string{}
	for _, q := range qs {
		labels[q.ID] = q.Label
	}
	rows, err := m.q.ListCheckinAnswersBetween(ctx, db.ListCheckinAnswersBetweenParams{
		Date: from.In(m.loc()).Format(dateLayout), Date_2: to.In(m.loc()).Format(dateLayout)})
	if err != nil {
		return nil, err
	}
	type key struct {
		q    int64
		date string
	}
	sums := map[key][2]int{}
	var order []key
	for _, a := range rows {
		k := key{a.QuestionID, a.Date}
		v, ok := sums[k]
		if !ok {
			order = append(order, k)
		}
		sums[k] = [2]int{v[0] + int(a.Value), v[1] + 1}
	}
	out := make([]contracts.CheckinDay, 0, len(order))
	for _, k := range order {
		v := sums[k]
		out = append(out, contracts.CheckinDay{QuestionID: k.q, Label: labels[k.q], Date: k.date, Average: float64(v[0]) / float64(v[1]), Count: v[1]})
	}
	return out, nil
}
