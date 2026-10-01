package hosts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// maxByteRate is 10 Gbit/s. A step of the counters that is bigger than
	// this over the time between two samples cannot be real.
	maxByteRate = 1_250_000_000
	// maxEstimate is the longest gap an estimate from the speed covers. After
	// a longer break the agent was probably offline, and nothing is guessed.
	maxEstimate = 60 * time.Second

	briefTTL = time.Minute
	// keepTrafficHours is how long the hour table is kept. The day table stays.
	keepTrafficHours = 7 * 24 * time.Hour
	dayLayout        = "2006-01-02"
	hourLayout       = "2006-01-02T15"
)

// ---- Cycles ----

// cycleAt returns the statistics period that holds day: [start, end], both
// included, in the location of day. A period starts on startDay of a month
// and lasts periodMonths months. A month without that day uses its last day.
// Periods of several months are aligned to January 2000, so every day gets
// the same period whenever it is asked about.
func cycleAt(day time.Time, startDay, periodMonths int) (start, end time.Time) {
	loc := day.Location()
	if periodMonths < 1 {
		periodMonths = 1
	}
	monthStart := func(index int) time.Time {
		year, month := index/12, time.Month(index%12+1)
		last := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
		return time.Date(year, month, min(startDay, last), 0, 0, 0, 0, loc)
	}
	today := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	index := day.Year()*12 + int(day.Month()) - 1
	base := 2000 * 12
	for (index-base)%periodMonths != 0 {
		index--
	}
	for monthStart(index).After(today) {
		index -= periodMonths
	}
	start = monthStart(index)
	next := monthStart(index + periodMonths)
	end = time.Date(next.Year(), next.Month(), next.Day()-1, 0, 0, 0, 0, loc)
	return start, end
}

// ---- Counting ----

type trafficLast struct {
	at     time.Time
	rx, tx uint64
	totals bool // the sample carried counters, not only a speed
}

// trafficCell is the place an amount of traffic is booked at: one UTC hour
// and one day of the server's time zone.
type trafficCell struct{ hour, day string }

type trafficAmount struct {
	rx, tx int64
	est    bool
}

func (a *trafficAmount) add(b trafficAmount) {
	a.rx += b.rx
	a.tx += b.tx
	a.est = a.est || b.est
}

// trafficTracker turns the samples of every host into traffic. The traffic
// waits in memory and the scheduler writes it to the database every minute.
type trafficTracker struct {
	mu       sync.Mutex
	last     map[string]trafficLast
	pending  map[string]map[trafficCell]*trafficAmount
	inflight map[string]map[trafficCell]*trafficAmount // being written right now
}

func newTrafficTracker() *trafficTracker {
	return &trafficTracker{last: map[string]trafficLast{}, pending: map[string]map[trafficCell]*trafficAmount{},
		inflight: map[string]map[trafficCell]*trafficAmount{}}
}

// observe books the traffic between the previous sample of a host and this one.
func (t *trafficTracker) observe(host string, x protocol.MetricsSample, loc *time.Location) {
	cur := trafficLast{at: x.At, rx: x.NetRxTotal, tx: x.NetTxTotal, totals: x.NetRxTotal > 0 || x.NetTxTotal > 0}
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, ok := t.last[host]
	t.last[host] = cur
	if !ok {
		return // the first sample after a restart only sets the starting point
	}
	seconds := cur.at.Sub(prev.at).Seconds()
	if seconds <= 0 {
		return
	}
	var amount trafficAmount
	switch {
	case cur.totals && prev.totals:
		// A counter that went down was reset by a reboot or a changed set of
		// interfaces: that step counts as nothing.
		if cur.rx >= prev.rx {
			amount.rx = int64(cur.rx - prev.rx)
		}
		if cur.tx >= prev.tx {
			amount.tx = int64(cur.tx - prev.tx)
		}
	case cur.totals:
		return // the previous sample has nothing to compare with
	default:
		// An older agent: guess from the speed, for at most a minute.
		window := min(seconds, maxEstimate.Seconds())
		amount = trafficAmount{rx: int64(x.NetRxRate * window), tx: int64(x.NetTxRate * window), est: true}
	}
	limit := int64(maxByteRate * seconds)
	if amount.rx > limit {
		amount.rx = 0
	}
	if amount.tx > limit {
		amount.tx = 0
	}
	if amount.rx == 0 && amount.tx == 0 {
		return
	}
	cell := trafficCell{hour: cur.at.UTC().Format(hourLayout), day: cur.at.In(loc).Format(dayLayout)}
	if t.pending[host] == nil {
		t.pending[host] = map[trafficCell]*trafficAmount{}
	}
	if t.pending[host][cell] == nil {
		t.pending[host][cell] = &trafficAmount{}
	}
	t.pending[host][cell].add(amount)
}

// forget drops what is kept in memory about a host.
func (t *trafficTracker) forget(host string) {
	t.mu.Lock()
	delete(t.last, host)
	delete(t.pending, host)
	delete(t.inflight, host)
	t.mu.Unlock()
}

// take moves the waiting traffic to the "being written" set and returns it.
func (t *trafficTracker) take() map[string]map[trafficCell]*trafficAmount {
	t.mu.Lock()
	defer t.mu.Unlock()
	batch := t.pending
	t.pending = map[string]map[trafficCell]*trafficAmount{}
	for host, cells := range batch {
		t.inflight[host] = cells
	}
	return batch
}

// done ends a write. When it failed the traffic goes back to waiting.
func (t *trafficTracker) done(batch map[string]map[trafficCell]*trafficAmount, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for host, cells := range batch {
		delete(t.inflight, host)
		if ok {
			continue
		}
		if t.pending[host] == nil {
			t.pending[host] = map[trafficCell]*trafficAmount{}
		}
		for cell, amount := range cells {
			if t.pending[host][cell] == nil {
				t.pending[host][cell] = &trafficAmount{}
			}
			t.pending[host][cell].add(*amount)
		}
	}
}

// byDay adds up what is not in the database yet for one host.
func (t *trafficTracker) byDay(host string) map[string]trafficAmount {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := map[string]trafficAmount{}
	for _, cells := range []map[trafficCell]*trafficAmount{t.pending[host], t.inflight[host]} {
		for cell, amount := range cells {
			sum := out[cell.day]
			sum.add(*amount)
			out[cell.day] = sum
		}
	}
	return out
}

// flushTraffic writes the waiting traffic to the hour and day tables.
func (m *Module) flushTraffic(ctx context.Context) error {
	batch := m.traffic.take()
	if len(batch) == 0 {
		return nil
	}
	err := m.writeTraffic(ctx, batch)
	m.traffic.done(batch, err == nil)
	return err
}

func (m *Module) writeTraffic(ctx context.Context, batch map[string]map[trafficCell]*trafficAmount) error {
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	for host, cells := range batch {
		for cell, a := range cells {
			est := int64(0)
			if a.est {
				est = 1
			}
			if err := q.AddTrafficHourly(ctx, db.AddTrafficHourlyParams{HostID: host, Hour: cell.hour, Rx: a.rx, Tx: a.tx, Estimated: est}); err != nil {
				return err
			}
			if err := q.AddTrafficDaily(ctx, db.AddTrafficDailyParams{HostID: host, Day: cell.day, Rx: a.rx, Tx: a.tx, Estimated: est}); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ---- Plans and reports ----

func defaultPlan(hostID string) db.HostTrafficPlan {
	return db.HostTrafficPlan{HostID: hostID, StartDay: 1, PeriodMonths: 1, CountMode: "both", AlertPercent: 80}
}

// trafficPlan returns the plan of a host, or the default one.
func (m *Module) trafficPlan(ctx context.Context, hostID string) (db.HostTrafficPlan, bool, error) {
	row, err := m.q.GetTrafficPlan(ctx, hostID)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPlan(hostID), false, nil
	}
	return row, err == nil, err
}

func planToAPI(p db.HostTrafficPlan) api.TrafficPlan {
	return api.TrafficPlan{StartDay: int(p.StartDay), PeriodMonths: api.TrafficPlanPeriodMonths(p.PeriodMonths),
		LimitBytes: p.LimitBytes, CountMode: api.TrafficCountMode(p.CountMode), AlertPercent: int(p.AlertPercent)}
}

// usedBytes is the traffic that counts against the limit.
func usedBytes(mode string, rx, tx int64) int64 {
	switch mode {
	case "out":
		return tx
	case "in":
		return rx
	case "max":
		return max(rx, tx)
	}
	return rx + tx
}

func (m *Module) today() time.Time {
	now := m.now().In(m.d.Config.Location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.d.Config.Location)
}

// cycleTotals adds up the traffic of a host from start to end, both included.
func (m *Module) cycleTotals(ctx context.Context, hostID string, start, end time.Time) (days []api.TrafficDay, rx, tx int64, estimated bool, err error) {
	rows, err := m.q.ListTrafficDays(ctx, db.ListTrafficDaysParams{HostID: hostID, FromDay: start.Format(dayLayout), ToDay: end.Format(dayLayout)})
	if err != nil {
		return nil, 0, 0, false, err
	}
	byDay := map[string]trafficAmount{}
	for _, r := range rows {
		byDay[r.Day] = trafficAmount{rx: r.Rx, tx: r.Tx, est: r.Estimated != 0}
	}
	for day, a := range m.traffic.byDay(hostID) {
		sum := byDay[day]
		sum.add(a)
		byDay[day] = sum
	}
	days = []api.TrafficDay{}
	for d := start; !d.After(end); d = time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, d.Location()) {
		a := byDay[d.Format(dayLayout)]
		days = append(days, api.TrafficDay{Day: openapi_types.Date{Time: d}, Rx: a.rx, Tx: a.tx})
		rx += a.rx
		tx += a.tx
		estimated = estimated || a.est
	}
	return days, rx, tx, estimated, nil
}

// trafficReport builds the answer of GET /hosts/{id}/traffic.
func (m *Module) trafficReport(ctx context.Context, hostID string, plan db.HostTrafficPlan, previous bool) (api.HostTraffic, error) {
	today := m.today()
	start, end := cycleAt(today, int(plan.StartDay), int(plan.PeriodMonths))
	if previous {
		start, end = cycleAt(start.AddDate(0, 0, -1), int(plan.StartDay), int(plan.PeriodMonths))
	}
	last := end
	if today.Before(last) {
		last = today
	}
	days, rx, tx, estimated, err := m.cycleTotals(ctx, hostID, start, last)
	if err != nil {
		return api.HostTraffic{}, err
	}
	out := api.HostTraffic{Plan: planToAPI(plan), CycleStart: openapi_types.Date{Time: start}, CycleEnd: openapi_types.Date{Time: end},
		Rx: rx, Tx: tx, UsedBytes: usedBytes(plan.CountMode, rx, tx), Days: days, Estimated: estimated}
	if !previous {
		elapsed := max(1, len(days))
		total := int(end.Sub(start).Hours()/24+0.5) + 1
		projected := out.UsedBytes * int64(total) / int64(elapsed)
		out.ProjectedBytes = &projected
	}
	return out, nil
}

// GetHostTraffic is GET /hosts/{hostId}/traffic.
func (m *Module) GetHostTraffic(w http.ResponseWriter, r *http.Request, hostID api.HostId, params api.GetHostTrafficParams) {
	ctx := r.Context()
	if _, err := m.host(ctx, hostID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	plan, _, err := m.trafficPlan(ctx, hostID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	previous := params.Cycle != nil && *params.Cycle == api.Previous
	out, err := m.trafficReport(ctx, hostID, plan, previous)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// PutHostTrafficPlan is PUT /hosts/{hostId}/traffic/plan.
func (m *Module) PutHostTrafficPlan(w http.ResponseWriter, r *http.Request, hostID api.HostId) {
	ctx := r.Context()
	if _, err := m.host(ctx, hostID); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.TrafficPlan
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validatePlan(in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	old, existed, err := m.trafficPlan(ctx, hostID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := db.HostTrafficPlan{HostID: hostID, StartDay: int64(in.StartDay), PeriodMonths: int64(in.PeriodMonths), LimitBytes: in.LimitBytes,
		CountMode: string(in.CountMode), AlertPercent: int64(in.AlertPercent), AlertedCycle: old.AlertedCycle, AlertedLevel: old.AlertedLevel,
		UpdatedAt: m.now()}
	if !existed || old.StartDay != next.StartDay || old.PeriodMonths != next.PeriodMonths {
		next.AlertedCycle, next.AlertedLevel = "", 0
	}
	err = m.q.UpsertTrafficPlan(ctx, db.UpsertTrafficPlanParams{HostID: next.HostID, StartDay: next.StartDay, PeriodMonths: next.PeriodMonths,
		LimitBytes: next.LimitBytes, CountMode: next.CountMode, AlertPercent: next.AlertPercent, AlertedCycle: next.AlertedCycle,
		AlertedLevel: next.AlertedLevel, UpdatedAt: next.UpdatedAt})
	m.d.Audit.Record(ctx, "host.traffic_plan", hostID, map[string]any{"startDay": in.StartDay, "periodMonths": in.PeriodMonths,
		"limitBytes": in.LimitBytes, "countMode": in.CountMode, "alertPercent": in.AlertPercent}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.briefs.forget(hostID)
	m.d.Bus.Publish("host.traffic_plan_changed", map[string]string{"hostId": hostID})
	out, err := m.trafficReport(ctx, hostID, next, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func validatePlan(p api.TrafficPlan) error {
	switch {
	case p.StartDay < 1 || p.StartDay > 31:
		return httpx.Invalid("统计开始日要在 1 到 31 之间")
	case p.PeriodMonths != 1 && p.PeriodMonths != 3 && p.PeriodMonths != 6 && p.PeriodMonths != 12:
		return httpx.Invalid("统计周期只能是 1、3、6 或 12 个月")
	case p.LimitBytes < 0:
		return httpx.Invalid("流量上限不能是负数")
	case p.AlertPercent < 0 || p.AlertPercent > 100:
		return httpx.Invalid("提醒比例要在 0 到 100 之间")
	case !p.CountMode.Valid():
		return httpx.Invalid("计算方式不正确")
	}
	return nil
}

// ---- List cards ----

// briefCache remembers the usage of hosts with a limit for a minute, so the
// list does not read the database for every host on every request.
type briefCache struct {
	mu   sync.Mutex
	from map[string]cachedBrief
}

type cachedBrief struct {
	at    time.Time
	brief api.TrafficBrief
}

func (c *briefCache) forget(host string) {
	c.mu.Lock()
	delete(c.from, host)
	c.mu.Unlock()
}

// trafficBriefs returns the usage of every host that has a limit.
func (m *Module) trafficBriefs(ctx context.Context) (map[string]api.TrafficBrief, error) {
	plans, err := m.q.ListTrafficPlansWithLimit(ctx)
	if err != nil {
		return nil, err
	}
	now := m.now()
	out := map[string]api.TrafficBrief{}
	for _, p := range plans {
		m.briefs.mu.Lock()
		hit, ok := m.briefs.from[p.HostID]
		m.briefs.mu.Unlock()
		if ok && now.Sub(hit.at) < briefTTL && hit.brief.LimitBytes == p.LimitBytes {
			out[p.HostID] = hit.brief
			continue
		}
		used, _, _, err := m.usedInCycle(ctx, p)
		if err != nil {
			return nil, err
		}
		brief := api.TrafficBrief{UsedBytes: used, LimitBytes: p.LimitBytes}
		m.briefs.mu.Lock()
		m.briefs.from[p.HostID] = cachedBrief{at: now, brief: brief}
		m.briefs.mu.Unlock()
		out[p.HostID] = brief
	}
	return out, nil
}

// usedInCycle is the traffic of the current cycle of a plan.
func (m *Module) usedInCycle(ctx context.Context, p db.HostTrafficPlan) (used int64, start, end time.Time, err error) {
	start, end = cycleAt(m.today(), int(p.StartDay), int(p.PeriodMonths))
	last := end
	if today := m.today(); today.Before(last) {
		last = today
	}
	_, rx, tx, _, err := m.cycleTotals(ctx, p.HostID, start, last)
	return usedBytes(p.CountMode, rx, tx), start, end, err
}

// ---- Alerts ----

// checkTraffic sends a notice when a host with a limit reaches its alert
// line and another one when the limit is used up, once per cycle each.
func (m *Module) checkTraffic(ctx context.Context) error {
	plans, err := m.q.ListTrafficPlansWithLimit(ctx)
	if err != nil {
		return err
	}
	for _, p := range plans {
		if err := m.checkPlan(ctx, p); err != nil {
			m.d.Log.Warn("traffic alert check failed", "host", p.HostID, "err", err)
		}
	}
	return nil
}

func (m *Module) checkPlan(ctx context.Context, p db.HostTrafficPlan) error {
	used, start, end, err := m.usedInCycle(ctx, p)
	if err != nil {
		return err
	}
	level := int64(0)
	switch {
	case used >= p.LimitBytes:
		level = 2
	case p.AlertPercent > 0 && used*100 >= p.LimitBytes*p.AlertPercent:
		level = 1
	}
	cycle := start.Format(dayLayout)
	alerted := p.AlertedLevel
	if p.AlertedCycle != cycle {
		alerted = 0 // a new cycle starts counting again
	}
	if level <= alerted {
		if p.AlertedCycle != cycle {
			return m.q.SetTrafficAlerted(ctx, db.SetTrafficAlertedParams{AlertedCycle: cycle, AlertedLevel: 0, HostID: p.HostID})
		}
		return nil
	}
	h, err := m.lookupHost(ctx, p.HostID)
	if errors.Is(err, httpx.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	daysLeft := int(end.Sub(m.today()).Hours()/24+0.5) + 1
	body := fmt.Sprintf("已用 %s，上限 %s，本周期还剩 %d 天。", byteText(used), byteText(p.LimitBytes), max(daysLeft, 0))
	n := notify.Notification{Kind: "host.traffic_alert", Body: body, Link: trafficLink(h), Priority: notify.PriorityNormal, Source: "hosts",
		Data: map[string]any{"hostId": p.HostID, "level": level}}
	if level == 2 {
		n.Title, n.Priority = h.Name+" 流量已用完", notify.PriorityHigh
	} else {
		n.Title = fmt.Sprintf("%s 流量已用 %d%%", h.Name, p.AlertPercent)
	}
	if _, err := m.d.Notify.Send(audit.WithActor(ctx, "system:traffic"), n); err != nil {
		return err
	}
	return m.q.SetTrafficAlerted(ctx, db.SetTrafficAlertedParams{AlertedCycle: cycle, AlertedLevel: level, HostID: p.HostID})
}

func trafficLink(h hostRef) string {
	if h.Kind == "desktop" {
		return "/pc"
	}
	return "/servers/" + h.ID
}

// byteText writes a byte count with a unit, for notices.
func byteText(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, suffix := float64(n), []string{"KB", "MB", "GB", "TB", "PB"}
	i := -1
	for value >= unit && i < len(suffix)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, suffix[i])
}

// dropTraffic removes everything counted for a host that is deleted.
func (m *Module) dropTraffic(ctx context.Context, hostID string) {
	m.traffic.forget(hostID)
	m.briefs.forget(hostID)
	for _, err := range []error{m.q.DeleteTrafficHostHourly(ctx, hostID), m.q.DeleteTrafficHostDaily(ctx, hostID), m.q.DeleteTrafficPlan(ctx, hostID)} {
		if err != nil {
			m.d.Log.Warn("drop traffic failed", "host", hostID, "err", err)
		}
	}
}
