package readlater

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	// digestKey keeps the week (like 2026-W41) the digest was last sent for.
	digestKey    = "readlater.digest_week"
	digestHour   = 10
	digestTitles = 5
)

// digest sends the unread list once a week, on Saturday from 10:00 on.
func (m *Module) digest(ctx context.Context) error {
	loc := m.d.Scheduler.Location()
	if loc == nil {
		loc = time.Local
	}
	now := m.now().In(loc)
	if now.Weekday() != time.Saturday || now.Hour() < digestHour {
		return nil
	}
	year, week := now.ISOWeek()
	key := fmt.Sprintf("%d-W%02d", year, week)
	var sent string
	if err := m.d.Settings.Get(ctx, digestKey, &sent); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return err
	}
	if sent == key {
		return nil
	}
	unread, err := m.q.CountUnreadReadItems(ctx)
	if err != nil || unread == 0 {
		return err
	}
	rows, err := m.q.OldestUnreadReadItems(ctx, digestTitles)
	if err != nil {
		return err
	}
	var lines []string
	for i, r := range rows {
		title := r.Title
		if title == "" {
			title = r.Url
		}
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, clipRunes(title, 60)))
	}
	if int(unread) > len(rows) {
		lines = append(lines, fmt.Sprintf("……还有 %d 篇", int(unread)-len(rows)))
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{
		Kind: "readlater.digest", Source: "readlater", Link: "/readlater",
		Title:    fmt.Sprintf("稍后阅读还有 %d 篇没看", unread),
		Body:     strings.Join(lines, "\n"),
		Priority: notify.PriorityNormal,
	})
	if err != nil {
		return err
	}
	return m.d.Settings.Set(ctx, digestKey, key)
}
