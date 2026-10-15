package router

// B230: what the panel keeps for the log page. Two kinds of lines: the events
// the panel writes itself (router_events) and the router's own system log
// (router_logs), which the script sends from `logread`.

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/db"
)

const (
	// maxLogsPerReport is how many lines the script sends at most. The panel
	// cuts there too, in case an old script sends more.
	maxLogsPerReport = 500
	// maxLogRows is the most rows of system log kept. dnsmasq writes several
	// lines for every DHCP renewal.
	maxLogRows = 200000
	// maxLogLine cuts a message that is longer than any real log line.
	maxLogLine = 2000
	// logPageDefault and logPageMax are the page sizes of the two lists.
	logPageDefault = 100
	logPageMax     = 200
	// programsTTL is how long the list of program names is reused.
	programsTTL = 30 * time.Second
)

// eventGroups are the kinds behind each filter of GET /router/events.
var eventGroups = map[string][]string{
	"device":  {"device_new", "device_online", "device_offline", "devices_seeded"},
	"wan":     {"wan_down", "wan_up"},
	"report":  {"report_lost", "report_back"},
	"command": {"interface_restart", "reboot"},
}

// logLevels are the levels in order of weight. A filter of "warn" means this
// level and the ones before it.
var logLevels = []string{"emerg", "alert", "crit", "err", "warn", "notice", "info", "debug"}

// logLine is one parsed line of logread.
type logLine struct {
	at       time.Time
	level    string
	facility string
	program  string
	message  string
}

// logRe is a line of `logread` on OpenWrt:
// "Wed Oct 15 12:00:01 2026 daemon.info dnsmasq-dhcp[1234]: DHCPACK(br-lan) ...".
var logRe = regexp.MustCompile(`^(\w{3} \w{3} +\d{1,2} \d{2}:\d{2}:\d{2} \d{4}) (\w+)\.(\w+) ([^\s:\[]+)(?:\[\d+\])?: ?(.*)$`)

// normalizeLevel maps the names syslog tools print to the eight in logLevels.
func normalizeLevel(s string) string {
	switch strings.ToLower(s) {
	case "warning":
		return "warn"
	case "error":
		return "err"
	case "panic":
		return "emerg"
	case "critical":
		return "crit"
	}
	for _, l := range logLevels {
		if strings.EqualFold(s, l) {
			return l
		}
	}
	return ""
}

// parseLogs reads the #xc:log section. Times are in the router's clock and
// time zone: loc is the zone the script reported (date +%z), skew is how far
// the panel's clock is ahead of the router's. A line that does not look like
// logread's is kept as it is with the time of the report.
func parseLogs(text string, loc *time.Location, skew time.Duration, now time.Time) []logLine {
	var out []logLine
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(out) >= maxLogsPerReport {
			break
		}
		if len(line) > maxLogLine {
			line = line[:maxLogLine]
		}
		l := logLine{at: now, message: line}
		if m := logRe.FindStringSubmatch(line); m != nil {
			if t, err := time.ParseInLocation("Mon Jan _2 15:04:05 2006", m[1], loc); err == nil {
				l.at = t.Add(skew)
				if l.at.After(now) {
					l.at = now
				}
				l.facility, l.program, l.message = m[2], m[4], m[5]
				l.level = normalizeLevel(m[3])
			}
		}
		out = append(out, l)
	}
	return out
}

// zoneFromOffset reads the output of `date +%z`, like "+0800".
func zoneFromOffset(text string) *time.Location {
	text = strings.TrimSpace(text)
	if len(text) != 5 || (text[0] != '+' && text[0] != '-') {
		return nil
	}
	h, err1 := strconv.Atoi(text[1:3])
	m, err2 := strconv.Atoi(text[3:5])
	if err1 != nil || err2 != nil || h > 14 || m > 59 {
		return nil
	}
	secs := h*3600 + m*60
	if text[0] == '-' {
		secs = -secs
	}
	return time.FixedZone("", secs)
}

// storeLogs saves the lines of one report.
func (m *Module) storeLogs(ctx context.Context, rep report, now time.Time) {
	if strings.TrimSpace(rep.logText) == "" {
		return
	}
	loc := zoneFromOffset(rep.tz)
	if loc == nil {
		loc = m.d.Config.Location
		if loc == nil {
			loc = time.UTC
		}
	}
	var skew time.Duration
	if !rep.routerTime.IsZero() {
		skew = now.Sub(rep.routerTime).Round(time.Second)
	}
	lines := parseLogs(rep.logText, loc, skew, now)
	if len(lines) == 0 {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err == nil {
		q := m.q.WithTx(tx)
		for _, l := range lines {
			if err = q.InsertRouterLog(ctx, dbLog(l)); err != nil {
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
		m.d.Log.Warn("router logs save", "err", err)
	}
}

// pruneLogs drops what is older than the keep days and what is over the row
// limit. It runs with the other cleanups once an hour.
func (m *Module) pruneLogs(ctx context.Context, now time.Time, days int) {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	if err := m.q.PruneRouterEvents(ctx, cutoff); err != nil {
		m.d.Log.Warn("router prune events", "err", err)
	}
	if err := m.q.PruneRouterLogs(ctx, cutoff); err != nil {
		m.d.Log.Warn("router prune logs", "err", err)
	}
	if err := m.q.TrimRouterLogs(ctx, maxLogRows); err != nil {
		m.d.Log.Warn("router trim logs", "err", err)
	}
}

func dbLog(l logLine) db.InsertRouterLogParams {
	return db.InsertRouterLogParams{At: l.at.Unix(), Level: l.level, Facility: l.facility, Program: l.program, Message: l.message}
}

// pageLimit reads the limit of a list.
func pageLimit(p *int) int {
	if p == nil || *p < 1 {
		return logPageDefault
	}
	return min(*p, logPageMax)
}

// ListRouterEvents lists the events, the newest first.
func (m *Module) ListRouterEvents(w http.ResponseWriter, r *http.Request, params api.ListRouterEventsParams) {
	ctx := r.Context()
	var where []string
	var args []any
	if params.Group != nil {
		kinds, ok := eventGroups[string(*params.Group)]
		if !ok {
			httpx.Fail(w, r, httpx.Invalid("group 只能是 device、wan、report、command"))
			return
		}
		where = append(where, "kind IN ("+strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",")+")")
		for _, k := range kinds {
			args = append(args, k)
		}
	}
	if params.Mac != nil && *params.Mac != "" {
		mac := deviceMAC(*params.Mac)
		if mac == "" {
			httpx.Fail(w, r, httpx.Invalid("MAC 格式不对"))
			return
		}
		where = append(where, "mac = ?")
		args = append(args, mac)
	}
	if params.Before != nil && *params.Before > 0 {
		where = append(where, "id < ?")
		args = append(args, *params.Before)
	}
	limit := pageLimit(params.Limit)
	query := "SELECT id, at, kind, mac, title, detail FROM router_events" + whereClause(where) + " ORDER BY id DESC LIMIT ?"
	rows, err := m.d.DB.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]api.RouterEvent, 0, limit)
	hasMore := false
	for rows.Next() {
		var e api.RouterEvent
		var at int64
		var mac sql.NullString
		if err := rows.Scan(&e.Id, &at, &e.Kind, &mac, &e.Title, &e.Detail); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		e.At = time.Unix(at, 0).UTC()
		if mac.Valid {
			v := mac.String
			e.Mac = &v
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "hasMore": hasMore})
}

func whereClause(where []string) string {
	if len(where) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(where, " AND ")
}

// likeEscape makes text safe inside a LIKE pattern that uses \ as the escape.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// programCache keeps the list of program names for a short time. Finding them
// reads every row.
type programCache struct {
	mu    sync.Mutex
	at    time.Time
	names []string
}

func (m *Module) programs(ctx context.Context) ([]string, error) {
	c := &m.programNames
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.names != nil && time.Since(c.at) < programsTTL {
		return c.names, nil
	}
	names, err := m.q.ListRouterLogPrograms(ctx)
	if err != nil {
		return nil, err
	}
	if names == nil {
		names = []string{}
	}
	c.names, c.at = names, time.Now()
	return names, nil
}

// ListRouterLogs lists the system log, the newest first.
func (m *Module) ListRouterLogs(w http.ResponseWriter, r *http.Request, params api.ListRouterLogsParams) {
	ctx := r.Context()
	var where []string
	var args []any
	if params.Level != nil {
		var levels []string
		switch string(*params.Level) {
		case "warn":
			levels = logLevels[:5]
		case "err":
			levels = logLevels[:4]
		default:
			httpx.Fail(w, r, httpx.Invalid("level 只能是 warn 或 err"))
			return
		}
		where = append(where, "level IN ("+strings.TrimSuffix(strings.Repeat("?,", len(levels)), ",")+")")
		for _, l := range levels {
			args = append(args, l)
		}
	}
	if params.Program != nil && *params.Program != "" {
		where = append(where, "program = ?")
		args = append(args, *params.Program)
	}
	if params.Q != nil && strings.TrimSpace(*params.Q) != "" {
		like := "%" + likeEscape(strings.TrimSpace(*params.Q)) + "%"
		where = append(where, `(message LIKE ? ESCAPE '\' OR program LIKE ? ESCAPE '\')`)
		args = append(args, like, like)
	}
	if params.Before != nil && *params.Before > 0 {
		where = append(where, "id < ?")
		args = append(args, *params.Before)
	}
	limit := pageLimit(params.Limit)
	query := "SELECT id, at, level, facility, program, message FROM router_logs" + whereClause(where) + " ORDER BY id DESC LIMIT ?"
	rows, err := m.d.DB.QueryContext(ctx, query, append(args, limit+1)...)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	defer rows.Close()
	items := make([]api.RouterLogLine, 0, limit)
	hasMore := false
	for rows.Next() {
		var l api.RouterLogLine
		var at int64
		if err := rows.Scan(&l.Id, &at, &l.Level, &l.Facility, &l.Program, &l.Message); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if len(items) == limit {
			hasMore = true
			break
		}
		l.At = time.Unix(at, 0).UTC()
		items = append(items, l)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	programs, err := m.programs(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "hasMore": hasMore, "programs": programs})
}
