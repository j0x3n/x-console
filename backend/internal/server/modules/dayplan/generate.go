package dayplan

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dayplan/api"
)

// dayBounds returns local midnight of the date and of the next day.
func (m *Module) dayBounds(date string) (time.Time, time.Time, error) {
	d, err := time.ParseInLocation(dateLayout, date, m.loc())
	if err != nil {
		return d, d, err
	}
	return d, d.AddDate(0, 0, 1), nil
}

func minutesOf(t time.Time, loc *time.Location) int {
	t = t.In(loc)
	return t.Hour()*60 + t.Minute()
}

// slots works out the free stretches of a day. For today they start no
// earlier than now.
func (m *Module) slots(ctx context.Context, date string, opts api.DayPlanSettings) ([]span, error) {
	start, end, err := m.dayBounds(date)
	if err != nil {
		return nil, err
	}
	ws, _ := parseHM(opts.WorkStart)
	we, _ := parseHM(opts.WorkEnd)
	from := 0
	now := m.now().In(m.loc())
	if date == now.Format(dateLayout) {
		from = roundUp(now.Hour()*60 + now.Minute())
	} else if date < now.Format(dateLayout) {
		return nil, nil
	}
	var busy []span
	if cal, ok := module.Lookup[contracts.Calendar](m.d.Registry, contracts.CalendarKey); ok {
		events, err := cal.Events(ctx, start, end)
		if err != nil {
			return nil, err
		}
		for _, e := range events {
			if e.AllDay {
				continue
			}
			s, en := e.Start.In(m.loc()), e.End.In(m.loc())
			a, b := 0, 24*60
			if s.After(start) {
				a = minutesOf(s, m.loc())
			}
			if en.Before(end) {
				b = minutesOf(en, m.loc())
			}
			if b > a {
				busy = append(busy, span{a, b})
			}
		}
	}
	return freeSlots(ws, we, from, busy), nil
}

// candidates collects what could go into the plan of the date.
func (m *Module) candidates(ctx context.Context, date string) ([]candidate, error) {
	start, end, err := m.dayBounds(date)
	if err != nil {
		return nil, err
	}
	var out []candidate
	seen := map[string]bool{}
	add := func(c candidate) {
		if c.Ref != "" {
			k := c.Kind + ":" + c.Ref
			if seen[k] {
				return
			}
			seen[k] = true
		}
		out = append(out, c)
	}
	// What was left over yesterday.
	prev := start.AddDate(0, 0, -1).Format(dateLayout)
	if plan, err := m.q.GetDayPlan(ctx, prev); err == nil && plan.Status == "confirmed" {
		items, err := m.q.ListDayPlanItems(ctx, prev)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Done == 0 {
				add(candidate{Title: it.Title, Kind: "carry", Ref: it.Ref, Detail: "昨天计划了，没做完", Score: 80, At: -1, Duration: defaultIssue})
				if it.Ref != "" {
					seen["issue:"+it.Ref] = true
					seen["reminder:"+it.Ref] = true
				}
			}
		}
	}
	if is, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey); ok {
		issues, err := is.ListDue(ctx, end)
		if err != nil {
			return nil, err
		}
		for _, it := range issues {
			if it.DueDate == nil || it.Status == "done" || it.Status == "canceled" {
				continue
			}
			overdue := it.DueDate.In(m.loc()).Before(start)
			detail := "今天到期"
			if overdue {
				detail = "已经逾期，原定 " + it.DueDate.In(m.loc()).Format("1月2日")
			}
			if it.Status == "in_progress" {
				detail += "，已经在做"
			}
			add(candidate{Title: it.Title, Kind: "issue", Ref: it.Key, Detail: detail, Score: scoreIssue(overdue, it.Priority, it.Status == "in_progress"), At: -1, Duration: defaultIssue})
		}
	}
	if rs, ok := module.Lookup[contracts.Reminders](m.d.Registry, contracts.RemindersKey); ok {
		rems, err := rs.Upcoming(ctx, end)
		if err != nil {
			return nil, err
		}
		for _, r := range rems {
			if r.At.Before(start) || !r.At.Before(end) {
				continue
			}
			add(candidate{Title: r.Title, Kind: "reminder", Ref: fmt.Sprint(r.ID), Detail: "提醒定在 " + r.At.In(m.loc()).Format("15:04"), Score: 60, At: minutesOf(r.At, m.loc()), Duration: defaultRemind})
		}
	}
	return rank(out), nil
}

const planSystem = `你是日程助手。用户要排今天最重要的几件事。下面给出今天的空档和候选事项。
规则：
- 最多选 3 件，只能从候选里选，用候选的序号（candidate）。
- 每件事定开始和结束时间（HH:MM），必须完全落在某个空档里，两件事不能重叠。排不下的事，start 和 end 都留空字符串。
- 有固定时间的提醒尽量排在它的时间。
- reason 用一句话说明为什么选它、为什么排在这个时间。
- 优先选已逾期的、今天到期的、已经在做的。不要编造候选里没有的事。`

var planSchema = []byte(`{"type":"object","properties":{"items":{"type":"array","items":{"type":"object","properties":{
"candidate":{"type":"integer"},"start":{"type":"string"},"end":{"type":"string"},"reason":{"type":"string"}},
"required":["candidate","start","end","reason"],"additionalProperties":false}}},"required":["items"],"additionalProperties":false}`)

// choose picks and places the items: with the AI when it is on and its
// answer holds up, otherwise by rule. source says which one it was.
func (m *Module) choose(ctx context.Context, date string, opts api.DayPlanSettings, cands []candidate, slots []span) (items []planned, source string) {
	top := cands
	if len(top) > maxCandidates {
		top = top[:maxCandidates]
	}
	if len(top) == 0 {
		return nil, "rule"
	}
	if opts.UseAi {
		if llm, ok := module.Lookup[contracts.LLM](m.d.Registry, contracts.LLMKey); ok && llm.Available(ctx) {
			var sb strings.Builder
			d, _ := time.ParseInLocation(dateLayout, date, m.loc())
			fmt.Fprintf(&sb, "日期：%s %s\n空档：", date, [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}[d.Weekday()])
			if len(slots) == 0 {
				sb.WriteString("没有")
			}
			for i, s := range slots {
				if i > 0 {
					sb.WriteString("、")
				}
				sb.WriteString(fmtHM(s.Start) + "-" + fmtHM(s.End))
			}
			sb.WriteString("\n候选事项：\n")
			for i, c := range top {
				fmt.Fprintf(&sb, "%d. %s（%s，预计 %d 分钟）\n", i, c.Title, c.Detail, c.Duration)
			}
			var out struct {
				Items []aiChoice `json:"items"`
			}
			if err := llm.CompleteJSON(contracts.WithAIUsage(ctx, "dayplan", date), "fast", planSystem, sb.String(), planSchema, &out); err != nil {
				m.d.Log.Warn("day plan ai", "err", err)
			} else if res, ok := validateChoices(out.Items, top, slots, maxPlanned); ok {
				return res, "ai"
			} else {
				m.d.Log.Warn("day plan ai answer rejected")
			}
		}
	}
	return pack(top, slots, maxPlanned), "rule"
}

// generate makes a draft for the date and stores it.
func (m *Module) generate(ctx context.Context, date string, replace bool) (api.DayPlan, error) {
	if _, _, err := m.dayBounds(date); err != nil {
		return api.DayPlan{}, errBadDate
	}
	if date < m.today() {
		return api.DayPlan{}, errPast
	}
	if cur, err := m.q.GetDayPlan(ctx, date); err == nil && cur.Status == "confirmed" && !replace {
		return api.DayPlan{}, errConfirmed
	}
	opts := m.options(ctx)
	slots, err := m.slots(ctx, date, opts)
	if err != nil {
		return api.DayPlan{}, err
	}
	cands, err := m.candidates(ctx, date)
	if err != nil {
		return api.DayPlan{}, err
	}
	picked, source := m.choose(ctx, date, opts, cands, slots)
	items := make([]itemRow, 0, len(picked))
	for _, p := range picked {
		row := itemRow{Title: p.Title, Kind: p.Kind, Ref: p.Ref, Reason: p.Reason}
		if row.Reason == "" {
			row.Reason = p.Detail
		}
		if p.Start >= 0 {
			row.Start, row.End = fmtHM(p.Start), fmtHM(p.End)
		}
		items = append(items, row)
	}
	if err := m.savePlan(ctx, date, "draft", source, items); err != nil {
		return api.DayPlan{}, err
	}
	m.d.Audit.Record(ctx, "dayplan.generate", date, map[string]any{"source": source, "items": len(items)}, nil)
	m.d.Bus.Publish("dayplan.changed", map[string]any{"date": date})
	return m.load(ctx, date)
}

// freeSlotsFor lists the free stretches of a date for the page.
func (m *Module) freeSlotsFor(ctx context.Context, date string) []api.DayPlanSlot {
	slots, err := m.slots(ctx, date, m.options(ctx))
	if err != nil {
		m.d.Log.Warn("day plan slots", "err", err)
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Start < slots[j].Start })
	out := make([]api.DayPlanSlot, 0, len(slots))
	for _, s := range slots {
		out = append(out, api.DayPlanSlot{Start: fmtHM(s.Start), End: fmtHM(s.End)})
	}
	return out
}
