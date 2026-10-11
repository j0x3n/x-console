package checkins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/checkins/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// actionPrefix starts every button id: "checkin.answer:<question>:<date>:<HHMM>:<value>".
const actionPrefix = "checkin."

// askTick asks the questions whose time has come. A time point is asked once
// per day: checkin_asks has a unique row for it. A question is still asked up
// to askWindow after its time, so a restart does not skip it.
func (m *Module) askTick(ctx context.Context) error {
	if !m.enabled(ctx) {
		return nil
	}
	now := m.now().In(m.loc())
	date := now.Format(dateLayout)
	qs, err := m.q.ListCheckinQuestions(ctx)
	if err != nil {
		return err
	}
	for _, row := range qs {
		q := toQuestion(row)
		if !q.Enabled {
			continue
		}
		for _, t := range q.Times {
			at := m.slotTime(now, t)
			if now.Before(at) || !now.Before(at.Add(askWindow)) {
				continue
			}
			if err := m.askOnce(ctx, q.Id, date, t); err != nil {
				return err
			}
		}
	}
	// Old rows are not needed: a time point of a past day is never asked again.
	return m.q.PruneCheckinAsks(ctx, now.AddDate(0, 0, -3).Format(dateLayout))
}

func (m *Module) askOnce(ctx context.Context, questionID int64, date, slot string) error {
	answers, err := m.q.ListCheckinAnswersOn(ctx, date)
	if err != nil {
		return err
	}
	for _, a := range answers {
		if a.QuestionID == questionID && a.Slot == slot {
			return nil // answered on the page before the question came
		}
	}
	n, err := m.q.InsertCheckinAsk(ctx, dbAsk(questionID, date, slot, m.now().UTC()))
	if err != nil || n == 0 {
		return err
	}
	row, err := m.q.GetCheckinQuestion(ctx, questionID)
	if err != nil {
		return err
	}
	q := toQuestion(row)
	body := fmt.Sprintf("1 分：%s，5 分：%s。点一个分数。", orDash(q.LowLabel), orDash(q.HighLabel))
	compact := strings.ReplaceAll(slot, ":", "")
	actions := make([]notify.Action, 0, 5)
	for v := 1; v <= 5; v++ {
		actions = append(actions, notify.Action{ID: fmt.Sprintf("%sanswer:%d:%s:%s:%d", actionPrefix, q.Id, date, compact, v), Label: strconv.Itoa(v)})
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{Kind: "checkin.ask", Title: q.Label, Body: body, Link: "/checkins",
		Source: "checkins", Actions: actions, Immediate: true})
	return err
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// handleAction records the score of a pressed button.
func (m *Module) handleAction(ctx context.Context, id string) error {
	parts := strings.Split(strings.TrimPrefix(id, actionPrefix), ":")
	if len(parts) != 5 || parts[0] != "answer" || len(parts[3]) != 4 {
		return httpx.Invalid("不认识的按钮")
	}
	qid, err1 := strconv.ParseInt(parts[1], 10, 64)
	value, err2 := strconv.Atoi(parts[4])
	if err1 != nil || err2 != nil {
		return httpx.Invalid("不认识的按钮")
	}
	slot := parts[3][:2] + ":" + parts[3][2:]
	if _, err := time.Parse(dateLayout, parts[2]); err != nil {
		return httpx.Invalid("不认识的按钮")
	}
	_, err := m.record(ctx, qid, value, slot, parts[2], "notify")
	return err
}

func dbAsk(questionID int64, date, slot string, at time.Time) db.InsertCheckinAskParams {
	return db.InsertCheckinAskParams{QuestionID: questionID, Date: date, Slot: slot, AskedAt: at}
}
