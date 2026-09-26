package brief

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief/api"
)

// Each section of the brief is built by its own function. A section whose
// data source is missing (the module is not built) or fails is skipped, the
// others still appear.

// errSkip means "leave this section out" without logging an error.
var errSkip = errors.New("skip section")

// input is what every section builder gets.
type input struct {
	now       time.Time // in the user's zone
	loc       *time.Location
	today     time.Time // local midnight
	tomorrow  time.Time
	yesterday time.Time
	reg       *module.Registry
	cfg       config
	weather   func(ctx context.Context, base string, loc api.BriefLocation) (api.Weather, error)
}

type section struct {
	key   string
	title string
	build func(ctx context.Context, in input) (string, error)
}

var sections = []section{
	{"weather", "天气", buildWeather},
	{"calendar", "今天的日程", buildCalendar},
	{"issues", "到期的 Issue", buildIssues},
	{"reminders", "今天的提醒", buildReminders},
	{"alerts", "服务器告警", buildAlerts},
	{"habits", "习惯", buildHabits},
	{"renewals", "续费", buildRenewals},
}

// ---- optional providers without a contract yet ----

// RenewalsKey is where a subscriptions module (M10) can register a
// RenewalSource. The brief lists renewals due within 7 days.
const RenewalsKey = "monitoring.renewals"

// Renewal is one upcoming renewal. It is an alias of an unnamed struct so a
// provider can implement RenewalSource without importing this package.
type Renewal = struct {
	Name     string
	Date     time.Time
	Amount   float64
	Currency string
}

// RenewalSource lists renewals due before until.
type RenewalSource interface {
	UpcomingRenewals(ctx context.Context, until time.Time) ([]Renewal, error)
}

// PolisherKey is where the AI module (M12) can register a Polisher. When it
// is present and brief.ai_polish is on, the brief starts with its summary.
const PolisherKey = "ai.brief_polisher"

// Polisher writes a short Chinese summary of the brief's Markdown.
type Polisher interface {
	Polish(ctx context.Context, markdown string) (string, error)
}

// ---- builders ----

func buildWeather(ctx context.Context, in input) (string, error) {
	if in.cfg.Location == nil {
		return "", errSkip
	}
	w, err := in.weather(ctx, in.cfg.WeatherBase, *in.cfg.Location)
	if err != nil {
		return "", err
	}
	return weatherLine(w), nil
}

func buildCalendar(ctx context.Context, in input) (string, error) {
	cal, ok := module.Lookup[contracts.Calendar](in.reg, contracts.CalendarKey)
	if !ok {
		return "", errSkip
	}
	events, err := cal.Events(ctx, in.today, in.tomorrow)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return "今天没有日程。", nil
	}
	var b strings.Builder
	for _, e := range events {
		when := "全天"
		if !e.AllDay {
			s, end := e.Start.In(in.loc), e.End.In(in.loc)
			when = s.Format("15:04")
			if end.After(s) {
				if end.Before(in.tomorrow) || end.Equal(in.tomorrow) {
					when += "–" + end.Format("15:04")
				} else {
					when += " 起"
				}
			}
		}
		fmt.Fprintf(&b, "- %s %s", when, e.Title)
		if e.Location != "" {
			fmt.Fprintf(&b, "（%s）", e.Location)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func buildIssues(ctx context.Context, in input) (string, error) {
	issues, ok := module.Lookup[contracts.Issues](in.reg, contracts.IssuesKey)
	if !ok {
		return "", errSkip
	}
	// ListDue includes the day of until, so ask with the last second of today.
	due, err := issues.ListDue(ctx, in.tomorrow.Add(-time.Second))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, is := range due {
		if is.DueDate == nil {
			continue
		}
		d := localDate(*is.DueDate, in.loc)
		if d.After(in.today) {
			continue
		}
		fmt.Fprintf(&b, "- %s %s", is.Key, is.Title)
		if d.Before(in.today) {
			days := int(in.today.Sub(d).Hours()+12) / 24
			fmt.Fprintf(&b, "（逾期 %d 天）", days)
		} else {
			b.WriteString("（今天到期）")
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "今天没有到期的 Issue。", nil
	}
	return strings.TrimSpace(b.String()), nil
}

// localDate reads a due date as a calendar day. Dates are stored as UTC
// midnight, so the UTC components are the date.
func localDate(t time.Time, loc *time.Location) time.Time {
	u := t.UTC()
	if u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 {
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, loc)
	}
	l := t.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
}

func buildReminders(ctx context.Context, in input) (string, error) {
	rem, ok := module.Lookup[contracts.Reminders](in.reg, contracts.RemindersKey)
	if !ok {
		return "", errSkip
	}
	list, err := rem.Upcoming(ctx, in.tomorrow)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, r := range list {
		if !r.At.Before(in.tomorrow) {
			continue
		}
		fmt.Fprintf(&b, "- %s %s\n", r.At.In(in.loc).Format("15:04"), r.Title)
	}
	if b.Len() == 0 {
		return "今天没有提醒。", nil
	}
	return strings.TrimSpace(b.String()), nil
}

func buildAlerts(ctx context.Context, in input) (string, error) {
	hosts, ok := module.Lookup[contracts.Hosts](in.reg, contracts.HostsKey)
	if !ok {
		return "", errSkip
	}
	alerts, err := hosts.Alerts(ctx, in.yesterday)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, a := range alerts {
		if a.FiredAt.Before(in.yesterday) {
			continue
		}
		name := a.HostName
		if name == "" {
			name = a.HostID
		}
		fmt.Fprintf(&b, "- %s %s %s", a.FiredAt.In(in.loc).Format("01-02 15:04"), name, a.Message)
		if a.Resolved != nil {
			b.WriteString("（已恢复）")
		}
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "从昨天到现在没有告警。", nil
	}
	return strings.TrimSpace(b.String()), nil
}

func buildHabits(ctx context.Context, in input) (string, error) {
	habits, ok := module.Lookup[contracts.Habits](in.reg, contracts.HabitsKey)
	if !ok {
		return "", errSkip
	}
	list, err := habits.Today(ctx)
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", errSkip
	}
	var b strings.Builder
	for _, h := range list {
		fmt.Fprintf(&b, "- %s：今天目标 %s %s", h.Name, num(h.Target), h.Unit)
		if h.Done > 0 {
			fmt.Fprintf(&b, "，已完成 %s", num(h.Done))
		}
		if h.Streak > 0 {
			fmt.Fprintf(&b, "，已连续 %d 天", h.Streak)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func buildRenewals(ctx context.Context, in input) (string, error) {
	src, ok := module.Lookup[RenewalSource](in.reg, RenewalsKey)
	if !ok {
		return "", errSkip
	}
	list, err := src.UpcomingRenewals(ctx, in.today.AddDate(0, 0, 8))
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", errSkip
	}
	var b strings.Builder
	for _, r := range list {
		fmt.Fprintf(&b, "- %s %s", r.Date.In(in.loc).Format("01-02"), r.Name)
		if r.Amount > 0 {
			fmt.Fprintf(&b, " %s %s", num(r.Amount), r.Currency)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

// ---- assembling ----

var weekdays = []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// dateTitle is "早报 · 10月27日 周二".
func dateTitle(t time.Time) string {
	return fmt.Sprintf("早报 · %d月%d日 %s", t.Month(), t.Day(), weekdays[t.Weekday()])
}

// result is a generated brief.
type result struct {
	date     string
	content  string
	sections []api.BriefSection
}

// generate builds the brief for the day of now. It never fails: sections
// that cannot be built are left out.
func (m *Module) generate(ctx context.Context, cfg config, reg *module.Registry, now time.Time) result {
	loc := m.d.Config.Location
	l := now.In(loc)
	today := time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, loc)
	in := input{
		now: l, loc: loc, today: today, tomorrow: today.AddDate(0, 0, 1), yesterday: today.AddDate(0, 0, -1),
		reg: reg, cfg: cfg, weather: m.fetchWeather,
	}
	out := result{date: today.Format(time.DateOnly), sections: []api.BriefSection{}}
	var b strings.Builder
	b.WriteString("# " + dateTitle(l) + "\n")
	for _, s := range sections {
		if !cfg.has(s.key) {
			continue
		}
		md, err := s.build(ctx, in)
		if errors.Is(err, errSkip) {
			continue
		}
		if err != nil {
			m.d.Log.Warn("brief section skipped", "section", s.key, "err", err)
			continue
		}
		out.sections = append(out.sections, api.BriefSection{Key: api.BriefSectionKey(s.key), Title: s.title, Markdown: md})
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", s.title, md)
	}
	out.content = b.String()

	// Hook for M12: an AI summary on top, when available and switched on.
	if p, ok := module.Lookup[Polisher](reg, PolisherKey); ok && cfg.AIPolish {
		if summary, err := p.Polish(ctx, out.content); err != nil {
			m.d.Log.Warn("brief summary failed", "err", err)
		} else if summary = strings.TrimSpace(summary); summary != "" {
			out.sections = append([]api.BriefSection{{Key: "summary", Title: "总结", Markdown: summary}}, out.sections...)
			title, rest, _ := strings.Cut(out.content, "\n")
			out.content = title + "\n\n## 总结\n\n" + summary + "\n" + rest
		}
	}
	return out
}

// plainText turns the brief into text for push channels: headings become
// 【title】 lines and the top heading is dropped (it is the title).
func plainText(content string) string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "# "):
			continue
		case strings.HasPrefix(line, "## "):
			line = "【" + strings.TrimPrefix(line, "## ") + "】"
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
