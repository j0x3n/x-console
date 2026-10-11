package inbox

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// reviewTick sends the evening reminder once a day when items are waiting.
func (m *Module) reviewTick(ctx context.Context) error {
	opts := m.options(ctx)
	if !opts.ReviewReminder {
		return nil
	}
	now := m.now().In(m.loc())
	hh, _ := strconv.Atoi(opts.ReviewTime[:2])
	mm, _ := strconv.Atoi(opts.ReviewTime[3:])
	if now.Hour()*60+now.Minute() < hh*60+mm {
		return nil
	}
	today := now.Format("2006-01-02")
	var last string
	if err := m.d.Settings.Get(ctx, lastReviewKey, &last); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	if last == today {
		return nil
	}
	n, err := m.q.CountOpenInboxItems(ctx)
	if err != nil || n == 0 {
		return err
	}
	// Written before sending: a failed send is not retried every minute.
	if err := m.d.Settings.Set(ctx, lastReviewKey, today); err != nil {
		return err
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{Kind: "inbox.review", Title: fmt.Sprintf("收集箱里有 %d 条没处理", n),
		Body: "花一分钟清一下：归档或忽略。", Link: "/inbox", Source: "inbox", Immediate: true})
	return err
}
