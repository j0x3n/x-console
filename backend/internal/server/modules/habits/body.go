package habits

// B119: body data. Weight, waist, sleep, steps and resting heart rate live in
// the daily personal record (personal.go). This file adds the ways to fill it
// without typing: a report token for a phone shortcut or Home Assistant, and
// the series the trend views and the AI read.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	bodyPushKey    = "habits.body_push"
	bodyReportPath = "/habits/body/report"
	maxBodyReport  = 4 << 10
	// bodyEarliest is the oldest date a report may write.
	bodyEarliest = "2000-01-01"
)

// bodyPush is the stored state of the report token. The token itself is never
// stored, only its hash.
type bodyPush struct {
	Hash           string     `json:"hash"`
	CreatedAt      time.Time  `json:"createdAt"`
	LastReportAt   *time.Time `json:"lastReportAt,omitempty"`
	LastReportDate string     `json:"lastReportDate,omitempty"`
}

// PublicPaths implements module.PublicPather: phones and Home Assistant have
// no session, the report is checked against the token instead.
func (m *Module) PublicPaths() []string { return []string{bodyReportPath} }

func hashBodyToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (m *Module) loadBodyPush(ctx context.Context) (bodyPush, error) {
	var p bodyPush
	if err := m.d.Settings.Get(ctx, bodyPushKey, &p); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return bodyPush{}, err
	}
	return p, nil
}

func (m *Module) reportURL(r *http.Request) string {
	base := strings.TrimRight(m.d.Config.PublicURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/api/v1" + bodyReportPath
}

// GetBodyPush answers the settings card.
func (m *Module) GetBodyPush(w http.ResponseWriter, r *http.Request) {
	p, err := m.loadBodyPush(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := api.BodyPushStatus{Enabled: p.Hash != "", ReportUrl: m.reportURL(r)}
	if p.Hash != "" {
		created := p.CreatedAt
		out.CreatedAt = &created
		out.LastReportAt = p.LastReportAt
		if p.LastReportDate != "" {
			date := p.LastReportDate
			out.LastReportDate = &date
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateBodyPushToken makes a new token. The old one stops working at once.
func (m *Module) CreateBodyPushToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := hex.EncodeToString(raw)
	m.bodyMu.Lock()
	err := m.d.Settings.Set(ctx, bodyPushKey, bodyPush{Hash: hashBodyToken(token), CreatedAt: time.Now().UTC()})
	m.bodyMu.Unlock()
	m.d.Audit.Record(ctx, "habit.body.token", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	url := m.reportURL(r)
	example := "curl -X POST '" + url + "' \\\n  -H 'Authorization: Bearer " + token + "' \\\n  -H 'Content-Type: application/json' \\\n  -d '{\"weight\": 72.4, \"sleep\": 7.5, \"restingHr\": 58, \"steps\": 8000}'"
	httpx.JSON(w, http.StatusOK, api.BodyPushToken{Token: token, ReportUrl: url, Example: example})
}

// DeleteBodyPush turns the report off.
func (m *Module) DeleteBodyPush(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m.bodyMu.Lock()
	err := m.d.Settings.Delete(ctx, bodyPushKey)
	m.bodyMu.Unlock()
	m.d.Audit.Record(ctx, "habit.body.token_delete", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func bearerOf(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// ReportBody takes one report from a phone shortcut, Home Assistant or a script.
func (m *Module) ReportBody(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p, err := m.loadBodyPush(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	token := bearerOf(r)
	if p.Hash == "" || token == "" || subtle.ConstantTimeCompare([]byte(hashBodyToken(token)), []byte(p.Hash)) != 1 {
		httpx.Fail(w, r, httpx.ErrUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyReport))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("上报内容读不出来或者太大"))
		return
	}
	date, err := m.recordBody(ctx, body, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	now := time.Now().UTC()
	m.bodyMu.Lock()
	// Keep the token that was checked: a new one may have been made meanwhile.
	if cur, e := m.loadBodyPush(ctx); e == nil && cur.Hash == p.Hash {
		cur.LastReportAt, cur.LastReportDate = &now, date
		_ = m.d.Settings.Set(ctx, bodyPushKey, cur)
	}
	m.bodyMu.Unlock()
	httpx.NoContent(w)
}

// bodyMetrics are the fields a report or an action may carry, with the key of
// the daily record each one is written to.
var bodyMetrics = []string{"weight", "waist", "sleep", "restingHr", "steps"}

// parseBodyReport reads the JSON of a report: the optional date and the
// metrics. Numbers and numeric strings are both fine; null, "" and missing
// fields mean "leave it". Anything else is refused as a whole.
func parseBodyReport(body []byte, today string) (string, api.PersonalDayInput, error) {
	var in api.PersonalDayInput
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&raw); err != nil || raw == nil {
		return "", in, httpx.Invalid("请求体要是 JSON 对象")
	}
	if dec.More() {
		return "", in, httpx.Invalid("请求体要是 JSON 对象")
	}
	date := today
	known := map[string]bool{"date": true}
	for _, k := range bodyMetrics {
		known[k] = true
	}
	for k := range raw {
		if !known[k] {
			return "", in, httpx.Invalid("不认识的字段：" + k)
		}
	}
	if v, ok := raw["date"]; ok && string(v) != "null" {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return "", in, httpx.Invalid("date 要写成 2026-10-10 这样")
		}
		if s = strings.TrimSpace(s); s != "" {
			date = s
		}
	}
	if err := personalDate(date); err != nil {
		return "", in, err
	}
	t, _ := time.Parse(time.DateOnly, today)
	if date < bodyEarliest || date > t.AddDate(0, 0, 1).Format(time.DateOnly) {
		return "", in, httpx.Invalid("日期不能早于 2000 年，也不能晚于明天")
	}
	set := 0
	for _, k := range bodyMetrics {
		v, ok := raw[k]
		if !ok || string(v) == "null" {
			continue
		}
		var text string
		if v[0] == '"' {
			if err := json.Unmarshal(v, &text); err != nil {
				return "", in, httpx.Invalid(k + " 不是数字")
			}
			text = strings.TrimSpace(text)
		} else {
			text = string(v)
		}
		if text == "" {
			continue
		}
		n, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return "", in, httpx.Invalid(k + " 不是数字")
		}
		s := strconv.FormatFloat(n, 'f', -1, 64)
		switch k {
		case "weight":
			in.Weight = &s
		case "waist":
			in.Waist = &s
		case "sleep":
			in.Sleep = &s
		case "restingHr":
			in.RestingHr = &s
		case "steps":
			in.Steps = &s
		}
		set++
	}
	if set == 0 {
		return "", in, httpx.Invalid("至少要有一项数据：weight、waist、sleep、restingHr、steps")
	}
	return date, in, nil
}

// recordBody writes a report or an action call into the daily record and
// returns the date it went to. Nothing is written when any value is refused.
func (m *Module) recordBody(ctx context.Context, body []byte, report bool) (string, error) {
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return "", err
	}
	date, in, err := parseBodyReport(body, dateKey(time.Now(), rules.location()))
	if err != nil {
		return "", err
	}
	action := "habit.body.record"
	if report {
		action = "habit.body.report"
	}
	_, err = m.personalTx(ctx, action, func(tx *sql.Tx, _ *personalProfileState) error {
		day, err := m.personalDay(ctx, tx, date)
		if err != nil {
			return err
		}
		if err = personalApplyDay(&day, in); err != nil {
			return err
		}
		return personalPut(ctx, tx, personalDayPrefix+date, day)
	})
	return date, err
}

// bodyPoint is one recorded value.
type bodyPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// bodySeries is one metric over a range. Days without a record are absent,
// not zero.
type bodySeries struct {
	Metric  string      `json:"metric"`
	Unit    string      `json:"unit"`
	Count   int         `json:"count"`
	Latest  *float64    `json:"latest,omitempty"`
	Average *float64    `json:"average,omitempty"`
	Min     *float64    `json:"min,omitempty"`
	Max     *float64    `json:"max,omitempty"`
	Change  *float64    `json:"change,omitempty"` // last value minus first value in the range
	Points  []bodyPoint `json:"points"`
}

func round1(v float64) *float64 {
	r := math.Round(v*10) / 10
	return &r
}

func bodyValue(d api.PersonalDay, metric string) string {
	switch metric {
	case "weight":
		return d.Weight
	case "waist":
		return d.Waist
	case "sleep":
		return d.Sleep
	case "restingHr":
		return d.RestingHr
	}
	return d.Steps
}

var bodyUnits = map[string]string{"weight": "kg", "waist": "cm", "sleep": "hours", "restingHr": "bpm", "steps": "steps"}

// bodyTrend reads the last days days (today included) of every metric.
func (m *Module) bodyTrend(ctx context.Context, days int) (map[string]any, error) {
	if days < 2 || days > 366 {
		return nil, httpx.Invalid("天数要在 2 到 366 之间")
	}
	rules, err := m.loadSchedule(ctx)
	if err != nil {
		return nil, err
	}
	loc := rules.location()
	today := dateKey(time.Now(), loc)
	since := dateKey(time.Now().In(loc).AddDate(0, 0, -(days-1)), loc)
	rows, err := m.personalDays(ctx, since)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
	series := make([]bodySeries, 0, len(bodyMetrics))
	for _, metric := range bodyMetrics {
		s := bodySeries{Metric: metric, Unit: bodyUnits[metric], Points: []bodyPoint{}}
		var sum float64
		for _, d := range rows {
			if d.Date > today {
				continue
			}
			text := bodyValue(d.PersonalDay, metric)
			if text == "" {
				continue
			}
			v, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				continue
			}
			s.Points = append(s.Points, bodyPoint{Date: d.Date, Value: v})
			sum += v
		}
		s.Count = len(s.Points)
		if s.Count > 0 {
			lo, hi := s.Points[0].Value, s.Points[0].Value
			for _, p := range s.Points {
				lo, hi = math.Min(lo, p.Value), math.Max(hi, p.Value)
			}
			first, last := s.Points[0].Value, s.Points[s.Count-1].Value
			s.Latest, s.Average, s.Min, s.Max = round1(last), round1(sum/float64(s.Count)), round1(lo), round1(hi)
			s.Change = round1(last - first)
		}
		series = append(series, s)
	}
	return map[string]any{"from": since, "to": today, "days": days, "series": series}, nil
}

func (m *Module) registerBodyActions() {
	m.d.Actions.Register(actions.Action{
		Name:        "habits.body_trend",
		Title:       "身体数据趋势",
		Description: "Weight (kg), waist (cm), sleep (hours), resting heart rate (bpm) and steps over the last N days (default 90, 2 to 366). Per metric: recorded points, latest, average, min, max and change from the first to the last recorded value. Days without a record are left out, never counted as zero.",
		Input:       actions.Schema(`{"type":"object","properties":{"days":{"type":"integer","minimum":2,"maximum":366}},"additionalProperties":false}`),
		Effect:      actions.Read,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				Days int `json:"days"`
			}
			if err := decodeStrict(raw, &in); err != nil {
				return nil, err
			}
			if in.Days == 0 {
				in.Days = 90
			}
			return m.bodyTrend(ctx, in.Days)
		},
	})
	m.d.Actions.Register(actions.Action{
		Name:        "habits.body_record",
		Title:       "记身体数据",
		Description: "Record body data for a day (date YYYY-MM-DD, default today). Any of weight (kg), waist (cm), sleep (hours), restingHr (bpm) and steps. Fields left out are not changed. Out-of-range values are refused.",
		Input:       actions.Schema(`{"type":"object","properties":{"date":{"type":"string"},"weight":{"type":"number"},"waist":{"type":"number"},"sleep":{"type":"number"},"restingHr":{"type":"number"},"steps":{"type":"number"}},"additionalProperties":false}`),
		Effect:      actions.Write,
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			date, err := m.recordBody(ctx, raw, false)
			if err != nil {
				return nil, err
			}
			d, err := m.personalDay(ctx, m.d.DB, date)
			return d.PersonalDay, err
		},
	})
}
