package journal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// calendarSource lists the events of the days, with the recurring ones
// already expanded by the calendar module.
type calendarSource struct{ m *Module }

func (s calendarSource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	cal, ok := module.Lookup[contracts.Calendar](s.m.d.Registry, contracts.CalendarKey)
	if !ok {
		return nil, nil
	}
	events, err := cal.Events(ctx, from, until)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.Activity, 0, len(events))
	for _, e := range events {
		if strings.TrimSpace(e.Title) == "" {
			continue
		}
		detail := e.Calendar
		if e.Location != "" {
			detail = strings.TrimSpace(e.Location + " · " + e.Calendar)
		}
		if e.AllDay {
			detail = strings.TrimSpace("全天 · " + detail)
		}
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("event:%d:%s", e.Start.Unix(), e.Title), Module: "calendar", Kind: "event",
			At: e.Start, Title: e.Title, Detail: detail, Link: "/calendar",
		})
	}
	return out, nil
}

// hostAlertSource lists the alerts the servers raised.
type hostAlertSource struct{ m *Module }

func (s hostAlertSource) Activity(ctx context.Context, from, until time.Time) ([]contracts.Activity, error) {
	hosts, ok := module.Lookup[contracts.Hosts](s.m.d.Registry, contracts.HostsKey)
	if !ok {
		return nil, nil
	}
	alerts, err := hosts.Alerts(ctx, from)
	if err != nil {
		return nil, err
	}
	loc := s.m.loc()
	var out []contracts.Activity
	for _, a := range alerts {
		if a.FiredAt.Before(from) || !a.FiredAt.Before(until) {
			continue
		}
		detail := "还没有恢复"
		if a.Resolved != nil {
			detail = "已恢复，" + a.Resolved.In(loc).Format("15:04")
		}
		title := strings.TrimSpace(a.HostName + " " + a.Message)
		out = append(out, contracts.Activity{
			Ref: fmt.Sprintf("alert:%s:%s:%d", a.HostID, a.Rule, a.FiredAt.Unix()), Module: "servers", Kind: "alert",
			At: a.FiredAt, Title: title, Detail: detail, Link: "/servers/" + a.HostID,
		})
	}
	return out, nil
}
