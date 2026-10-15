package router

// B229: the traffic of each device. The script sends `nlbw -c json -g mac`.
// nlbwmon counts from the connection tracking table, which still works when
// the fast path (Shortcut-FE) keeps the packets away from the firewall.
// The counters run from the start of nlbwmon's period, so the panel works with
// the difference between two reports.

import (
	"context"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/db"
)

const (
	// rateWindow is the span a speed is averaged over. nlbwmon refreshes its
	// counters every 5 seconds, so two reports 3 seconds apart would show a
	// speed that jumps between 0 and double.
	rateWindow = 10 * time.Second
	// flushEvery bounds the writes of the hourly sums.
	flushEvery = 30 * time.Second
	// maxTrackSamples bounds the samples kept for one device.
	maxTrackSamples = 64
	// forgetTrackAfter is how long the counters of a device that left the
	// nlbwmon list are kept.
	forgetTrackAfter = time.Hour
)

// accSample is the sum of the increases seen so far, at a time. It only grows,
// also when nlbwmon's own counter was reset.
type accSample struct {
	at     time.Time
	rx, tx uint64
}

// nlbwTrack follows the counters of one device.
type nlbwTrack struct {
	last   nlbwRow
	lastAt time.Time
	acc    []accSample
}

// rates returns the speed over the last rateWindow: from the newest sample
// that is at least that old to the latest one, or from the oldest kept.
func (t *nlbwTrack) rates() (rx, tx float64, ok bool) {
	n := len(t.acc)
	if n < 2 {
		return 0, 0, false
	}
	last := t.acc[n-1]
	start := 0
	for i := n - 2; i >= 0; i-- {
		if last.at.Sub(t.acc[i].at) >= rateWindow {
			start = i
			break
		}
	}
	t.acc = t.acc[start:]
	first := t.acc[0]
	dt := last.at.Sub(first.at).Seconds()
	if dt <= 0 {
		return 0, 0, false
	}
	return float64(last.rx-first.rx) / dt, float64(last.tx-first.tx) / dt, true
}

// nlbwDelta is what a device moved since the previous report.
type nlbwDelta struct{ rx, tx uint64 }

type trafficKey struct {
	mac  string
	hour int64
}

// counterDelta is the increase of a counter. A counter that went down was
// reset (nlbwmon restarted or a new period began), so the new value is all
// new traffic.
func counterDelta(prev, cur uint64) uint64 {
	if cur >= prev {
		return cur - prev
	}
	return cur
}

// trackNlbw takes the counters of one report and returns the increase of
// every device that has an earlier reading less than maxGap ago.
// The caller holds m.devs.mu.
func (m *Module) trackNlbw(rows map[string]nlbwRow, now time.Time) map[string]nlbwDelta {
	s := &m.devs
	if s.tracks == nil {
		s.tracks = map[string]*nlbwTrack{}
	}
	out := map[string]nlbwDelta{}
	for mac, row := range rows {
		t := s.tracks[mac]
		if t != nil && !now.After(t.lastAt) {
			continue // the same instant twice
		}
		if t == nil || now.Sub(t.lastAt) > maxGap {
			s.tracks[mac] = &nlbwTrack{last: row, lastAt: now, acc: []accSample{{at: now}}}
			continue
		}
		drx, dtx := counterDelta(t.last.rx, row.rx), counterDelta(t.last.tx, row.tx)
		prev := t.acc[len(t.acc)-1]
		t.acc = append(t.acc, accSample{at: now, rx: prev.rx + drx, tx: prev.tx + dtx})
		if len(t.acc) > maxTrackSamples {
			t.acc = t.acc[len(t.acc)-maxTrackSamples:]
		}
		t.last, t.lastAt = row, now
		out[mac] = nlbwDelta{rx: drx, tx: dtx}
	}
	for mac, t := range s.tracks {
		if now.Sub(t.lastAt) > forgetTrackAfter {
			delete(s.tracks, mac)
		}
	}
	return out
}

// hourOf is the start of the hour a time is in, as Unix seconds.
func hourOf(t time.Time) int64 {
	u := t.Unix()
	return u - u%3600
}

// addTraffic adds an increase to the hourly sums in memory. The caller holds
// m.devs.mu.
func (s *deviceStore) addTraffic(mac string, hour int64, d nlbwDelta, now time.Time) {
	if d.rx == 0 && d.tx == 0 {
		return
	}
	if len(s.pending) == 0 {
		s.pending, s.pendingSince = map[trafficKey]nlbwDelta{}, now
	}
	k := trafficKey{mac, hour}
	p := s.pending[k]
	s.pending[k] = nlbwDelta{rx: p.rx + d.rx, tx: p.tx + d.tx}
}

// flushTraffic writes the sums in memory to the database, at most every
// flushEvery unless force is set. The caller holds m.devs.mu.
func (m *Module) flushTraffic(ctx context.Context, now time.Time, force bool) {
	s := &m.devs
	if len(s.pending) == 0 || (!force && now.Sub(s.pendingSince) < flushEvery) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err == nil {
		q := m.q.WithTx(tx)
		for k, v := range s.pending {
			if err = q.AddDeviceTraffic(ctx, db.AddDeviceTrafficParams{Mac: k.mac, Hour: k.hour, Rx: int64(v.rx), Tx: int64(v.tx)}); err != nil {
				break
			}
		}
		if err == nil {
			err = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
	}
	if err != nil {
		m.d.Log.Warn("router device traffic", "err", err)
		return
	}
	s.pending = nil
}

// startOfToday is midnight in the panel's time zone.
func (m *Module) startOfToday(now time.Time) time.Time {
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.Local
	}
	y, mo, d := now.In(loc).Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, loc)
}

// todayTotals returns what each device moved today and the sum of them all.
// It adds the sums that are not written yet. The caller holds m.devs.mu.
func (m *Module) todayTotals(ctx context.Context, now time.Time) (map[string]nlbwDelta, nlbwDelta, error) {
	since := hourOf(m.startOfToday(now))
	rows, err := m.q.SumDeviceTraffic(ctx, since)
	if err != nil {
		return nil, nlbwDelta{}, err
	}
	per := make(map[string]nlbwDelta, len(rows))
	for _, r := range rows {
		per[r.Mac] = nlbwDelta{rx: uint64(max(r.Rx, 0)), tx: uint64(max(r.Tx, 0))}
	}
	for k, v := range m.devs.pending {
		if k.hour >= since {
			p := per[k.mac]
			per[k.mac] = nlbwDelta{rx: p.rx + v.rx, tx: p.tx + v.tx}
		}
	}
	var sum nlbwDelta
	for _, v := range per {
		sum.rx += v.rx
		sum.tx += v.tx
	}
	return per, sum, nil
}

// trafficAvailable says the latest report carried nlbwmon's numbers.
func (m *Module) trafficAvailable() bool {
	m.mu.Lock()
	snap := m.report
	m.mu.Unlock()
	return snap != nil && snap.hasNlbw && m.now().Sub(snap.at) <= pushStaleAfter
}

// GetRouterDeviceTraffic is the hourly traffic of one device.
func (m *Module) GetRouterDeviceTraffic(w http.ResponseWriter, r *http.Request, rawMAC string, params api.GetRouterDeviceTrafficParams) {
	ctx := r.Context()
	mac, err := macParam(rawMAC)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rng, span := "24h", 24*time.Hour
	if params.Range != nil && *params.Range == "7d" {
		rng, span = "7d", 7*24*time.Hour
	}
	now := m.now()
	s := &m.devs
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := m.loadDevices(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if s.byMAC[mac] == nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m.flushTraffic(ctx, now, true)
	rows, err := m.q.ListDeviceTraffic(ctx, db.ListDeviceTrafficParams{Mac: mac, Hour: hourOf(now.Add(-span))})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.RouterTraffic{Range: rng, StepSeconds: 3600, Points: []api.RouterTrafficPoint{}}
	cur := hourOf(now)
	for _, row := range rows {
		secs := int64(3600)
		if row.Hour == cur {
			secs = max(now.Unix()-cur, 1) // the hour is not over
		}
		out.Points = append(out.Points, api.RouterTrafficPoint{At: time.Unix(row.Hour, 0).UTC(), RxRate: float64(row.Rx) / float64(secs), TxRate: float64(row.Tx) / float64(secs)})
		out.RxBytes += row.Rx
		out.TxBytes += row.Tx
	}
	httpx.JSON(w, http.StatusOK, out)
}

// addTodayTotals adds the "today" fields to GET /router/traffic: what all the
// devices moved and what the WAN moved, so the two can be compared.
func (m *Module) addTodayTotals(ctx context.Context, out *api.RouterTraffic) error {
	if !m.trafficAvailable() {
		return nil
	}
	now := m.now()
	s := &m.devs
	s.mu.Lock()
	_, sum, err := m.todayTotals(ctx, now)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	wan, err := m.q.SumTrafficSince(ctx, m.startOfToday(now).Unix())
	if err != nil {
		return err
	}
	drx, dtx, wrx, wtx := int64(sum.rx), int64(sum.tx), wan.Rx, wan.Tx
	out.DevicesTodayRx, out.DevicesTodayTx, out.WanTodayRx, out.WanTodayTx = &drx, &dtx, &wrx, &wtx
	return nil
}
