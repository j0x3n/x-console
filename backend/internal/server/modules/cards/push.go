package cards

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// pushWindow is how long after the chosen time a missed push is still sent
// (the server was down at 20:00 and came back at 21:00).
const pushWindow = 3 * time.Hour

// actionPrefix is the start of every button id of this module. The ids are
// "cards.start", "cards.show:<card>" and "cards.rate:<card>:<reps>:<rating>".
const actionPrefix = "cards."

// pushJob sends the daily "cards to review" notice once, when the chosen time
// has come. It checks once a minute and writes the date of the last push, so
// a restart does not send it twice.
func (m *Module) pushJob(ctx context.Context) error {
	o := m.options(ctx)
	if !o.PushEnabled {
		return nil
	}
	h, mi, ok := parseClock(o.PushTime)
	if !ok {
		return nil
	}
	now := m.now().In(m.location())
	at := time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, m.location())
	if now.Before(at) || now.After(at.Add(pushWindow)) {
		return nil
	}
	today := now.Format(time.DateOnly)
	var last string
	if err := m.d.Settings.Get(ctx, lastPushKey, &last); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	if last == today {
		return nil
	}
	// Written before sending: a failed send is not retried every minute.
	if err := m.d.Settings.Set(ctx, lastPushKey, today); err != nil {
		return err
	}
	n, err := m.DueCount(ctx)
	if err != nil || n == 0 {
		return err
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{
		Kind: "card.due", Title: fmt.Sprintf("今天有 %d 张卡待复习", n), Link: "/cards", Source: "cards",
		Actions: []notify.Action{{ID: "cards.start", Label: "开始"}},
	})
	return err
}

// ---- Telegram review ----
//
// The Telegram channel answers a button press by editing the original message
// to "<text> / 已处理：<button>" and removing the buttons. A handler cannot
// change the message itself. So every step sends a new message: the question,
// the answer with the four ratings, and the next question.

var ratingLabels = []string{"重来", "困难", "良好", "简单"}

// handleAction runs the button presses of the Telegram review.
func (m *Module) handleAction(ctx context.Context, id string) error {
	switch {
	case id == "cards.start":
		return m.sendNext(ctx)
	case strings.HasPrefix(id, "cards.show:"):
		cardID, err := strconv.ParseInt(strings.TrimPrefix(id, "cards.show:"), 10, 64)
		if err != nil {
			return httpx.ErrNotFound
		}
		return m.sendAnswer(ctx, cardID)
	case strings.HasPrefix(id, "cards.rate:"):
		parts := strings.Split(strings.TrimPrefix(id, "cards.rate:"), ":")
		if len(parts) != 3 {
			return httpx.ErrNotFound
		}
		cardID, err1 := strconv.ParseInt(parts[0], 10, 64)
		reps, err2 := strconv.Atoi(parts[1])
		rating, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return httpx.ErrNotFound
		}
		if _, err := m.reviewCard(ctx, cardID, rating, &reps); err != nil {
			return err
		}
		return m.sendNext(ctx)
	default:
		return httpx.ErrNotFound
	}
}

// telegram sends a message with buttons to the Telegram chat, straight to the
// channel: it is an answer to a button the user pressed, so it does not go
// through the routing rules or quiet hours and it is not kept as a notice.
func (m *Module) telegram(ctx context.Context, n notify.Notification) error {
	ch, ok := m.d.Notify.Channel("telegram")
	if !ok {
		return httpx.ErrIntegrationMissing
	}
	n.Source, n.Link = "cards", "/cards"
	return ch.Send(ctx, notify.Stored{CreatedAt: m.now(), Notification: n})
}

// sendNext sends the first card still to review, or the closing message.
func (m *Module) sendNext(ctx context.Context) error {
	q, err := m.queue(ctx, "", 1)
	if err != nil {
		return err
	}
	if len(q.Items) == 0 {
		body := "今天没有待复习的卡片。"
		if done := q.Summary.ReviewedToday + q.Summary.LearnedToday; done > 0 {
			body = fmt.Sprintf("今天复习了 %d 张。", done)
		}
		return m.telegram(ctx, notify.Notification{Kind: "card.done", Title: "今天的卡片复习完了", Body: body})
	}
	c := q.Items[0].Card
	return m.telegram(ctx, notify.Notification{
		Kind: "card.question", Title: fmt.Sprintf("复习卡片，还剩 %d 张", q.Summary.Due), Body: "问：" + c.Question,
		Actions: []notify.Action{{ID: "cards.show:" + idString(c.Id), Label: "显示答案"}},
	})
}

// sendAnswer sends the answer and the four rating buttons.
func (m *Module) sendAnswer(ctx context.Context, id int64) error {
	c, err := m.getCard(ctx, id)
	if err != nil {
		return err
	}
	if c.Suspended != 0 {
		return httpx.NewError(http.StatusConflict, "card_suspended", "这张卡已暂停")
	}
	actions := make([]notify.Action, 0, 4)
	for i, label := range ratingLabels {
		actions = append(actions, notify.Action{
			ID: fmt.Sprintf("cards.rate:%d:%d:%d", c.ID, c.Reps, i+1), Label: label,
		})
	}
	return m.telegram(ctx, notify.Notification{
		Kind: "card.answer", Title: "答案", Body: "问：" + c.Question + "\n答：" + c.Answer, Actions: actions,
	})
}
