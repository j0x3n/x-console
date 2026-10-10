package habits_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/habits/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// report sends a body report like a phone shortcut: bearer token, no cookies.
func report(t *testing.T, env *testutil.Env, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.URL("/habits/body/report"), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func newBodyToken(t *testing.T, env *testutil.Env) api.BodyPushToken {
	t.Helper()
	env.Elevate()
	var tok api.BodyPushToken
	env.MustDo(http.MethodPost, "/habits/body/push/token", nil, &tok)
	return tok
}

func bodyDay(t *testing.T, env *testutil.Env, date string) api.PersonalDay {
	t.Helper()
	var d api.PersonalDay
	env.MustDo(http.MethodGet, "/habits/personal/days/"+date, nil, &d)
	return d
}

func today(env *testutil.Env) string {
	return time.Now().In(env.App.Deps.Config.Location).Format(time.DateOnly)
}

func TestRestingHeartRate(t *testing.T) {
	env, _ := setup(t)
	path := "/habits/personal/days/2026-10-01"
	var d api.PersonalDay
	env.MustDo(http.MethodPatch, path, map[string]any{"restingHr": "58", "weight": "72.4"}, &d)
	if d.RestingHr != "58" || bodyDay(t, env, "2026-10-01").RestingHr != "58" {
		t.Fatalf("resting heart rate not saved: %+v", d)
	}
	// other fields in a later patch keep it
	env.MustDo(http.MethodPatch, path, map[string]any{"sleep": "7"}, &d)
	if d.RestingHr != "58" {
		t.Fatalf("patch of another field lost the heart rate: %+v", d)
	}
	for _, bad := range []string{"19", "221", "58.5", "abc"} {
		if status, _ := env.Do(http.MethodPatch, path, map[string]any{"restingHr": bad}, nil); status != http.StatusBadRequest {
			t.Fatalf("restingHr %q: status %d", bad, status)
		}
	}
	if bodyDay(t, env, "2026-10-01").RestingHr != "58" {
		t.Fatal("a refused value changed the record")
	}
	// empty clears
	env.MustDo(http.MethodPatch, path, map[string]any{"restingHr": ""}, &d)
	if d.RestingHr != "" {
		t.Fatalf("not cleared: %+v", d)
	}

	// backup round trip
	env.MustDo(http.MethodPatch, path, map[string]any{"restingHr": "61"}, nil)
	var backup api.PersonalBackup
	env.MustDo(http.MethodGet, "/habits/personal/backup", nil, &backup)
	got := backup.Logs["2026-10-01"].RestingHr
	if got == nil || *got != "61" {
		t.Fatalf("backup lost the heart rate: %+v", backup.Logs["2026-10-01"])
	}
	env.MustDo(http.MethodPatch, path, map[string]any{"restingHr": ""}, nil)
	env.MustDo(http.MethodPost, "/habits/personal/backup", backup, nil)
	if bodyDay(t, env, "2026-10-01").RestingHr != "61" {
		t.Fatal("import lost the heart rate")
	}
}

func TestBodyTokenLifecycle(t *testing.T) {
	env, _ := setup(t)
	var st api.BodyPushStatus
	env.MustDo(http.MethodGet, "/habits/body/push", nil, &st)
	if st.Enabled || !strings.HasSuffix(st.ReportUrl, "/api/v1/habits/body/report") {
		t.Fatalf("fresh status: %+v", st)
	}
	// making a token needs the elevated window
	if status, _ := env.Do(http.MethodPost, "/habits/body/push/token", nil, nil); status < 400 {
		t.Fatalf("token created without elevation: %d", status)
	}
	if status, _ := report(t, env, "", `{"weight":70}`); status != http.StatusUnauthorized {
		t.Fatalf("no token: %d", status)
	}

	tok := newBodyToken(t, env)
	if len(tok.Token) != 64 || tok.ReportUrl != st.ReportUrl || !strings.Contains(tok.Example, "Bearer "+tok.Token) || !strings.Contains(tok.Example, tok.ReportUrl) {
		t.Fatalf("token: %+v", tok)
	}
	// only the hash is stored
	var stored string
	env.App.Deps.DB.QueryRow("SELECT value FROM settings WHERE key = 'habits.body_push'").Scan(&stored)
	if strings.Contains(stored, tok.Token) {
		t.Fatal("the token is stored in clear")
	}
	env.MustDo(http.MethodGet, "/habits/body/push", nil, &st)
	if !st.Enabled || st.LastReportAt != nil || st.CreatedAt == nil {
		t.Fatalf("status after create: %+v", st)
	}

	if status, _ := report(t, env, tok.Token, `{"weight":72.4}`); status != http.StatusNoContent {
		t.Fatalf("report: %d", status)
	}
	if status, _ := report(t, env, "wrong", `{"weight":72.4}`); status != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", status)
	}
	env.MustDo(http.MethodGet, "/habits/body/push", nil, &st)
	if st.LastReportAt == nil || st.LastReportDate == nil || *st.LastReportDate != today(env) {
		t.Fatalf("status after report: %+v", st)
	}

	// a new token cancels the old one
	tok2 := newBodyToken(t, env)
	if status, _ := report(t, env, tok.Token, `{"weight":72.4}`); status != http.StatusUnauthorized {
		t.Fatalf("old token still works: %d", status)
	}
	if status, _ := report(t, env, tok2.Token, `{"weight":72.5}`); status != http.StatusNoContent {
		t.Fatalf("new token: %d", status)
	}

	env.MustDo(http.MethodDelete, "/habits/body/push", nil, nil)
	env.MustDo(http.MethodDelete, "/habits/body/push", nil, nil) // twice is fine
	if status, _ := report(t, env, tok2.Token, `{"weight":72.5}`); status != http.StatusUnauthorized {
		t.Fatalf("token works after delete: %d", status)
	}
	env.MustDo(http.MethodGet, "/habits/body/push", nil, &st)
	if st.Enabled {
		t.Fatalf("still enabled: %+v", st)
	}
}

func TestBodyReport(t *testing.T) {
	env, _ := setup(t)
	tok := newBodyToken(t, env).Token
	day := today(env)

	// numbers and numeric strings, today by default
	if status, msg := report(t, env, tok, `{"weight":72.4,"sleep":"7.5","restingHr":58,"steps":"8123","waist":80}`); status != http.StatusNoContent {
		t.Fatalf("report: %d %s", status, msg)
	}
	d := bodyDay(t, env, day)
	if d.Weight != "72.4" || d.Sleep != "7.5" || d.RestingHr != "58" || d.Steps != "8123" || d.Waist != "80" {
		t.Fatalf("record: %+v", d)
	}

	// a second report with one field leaves the others, null and "" are skipped
	if status, msg := report(t, env, tok, `{"weight":72.0,"sleep":null,"steps":"","restingHr":" "}`); status != http.StatusNoContent {
		t.Fatalf("partial: %d %s", status, msg)
	}
	d = bodyDay(t, env, day)
	if d.Weight != "72" || d.Sleep != "7.5" || d.RestingHr != "58" || d.Steps != "8123" {
		t.Fatalf("partial report changed other fields: %+v", d)
	}

	// an explicit date, and exponent notation is normalised
	if status, msg := report(t, env, tok, `{"date":"2026-09-30","steps":1.2e4}`); status != http.StatusNoContent {
		t.Fatalf("dated: %d %s", status, msg)
	}
	if got := bodyDay(t, env, "2026-09-30").Steps; got != "12000" {
		t.Fatalf("steps: %q", got)
	}

	// refused as a whole, nothing written
	before := bodyDay(t, env, day)
	for name, body := range map[string]string{
		"out of range":  `{"weight":72,"restingHr":300}`,
		"fraction":      `{"weight":72,"steps":8000.5}`,
		"not a number":  `{"weight":"72 kg"}`,
		"bool":          `{"weight":true}`,
		"unknown key":   `{"weight":72,"weigth":1}`,
		"no metric":     `{"date":"2026-09-30"}`,
		"empty object":  `{}`,
		"not an object": `[1]`,
		"not json":      `weight=72`,
		"bad date":      `{"date":"yesterday","weight":72}`,
		"future":        `{"date":"2999-01-01","weight":72}`,
		"too old":       `{"date":"1999-12-31","weight":72}`,
		"trailing":      `{"weight":72} {"weight":73}`,
	} {
		if status, _ := report(t, env, tok, body); status != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, status)
		}
	}
	after := bodyDay(t, env, day)
	if after.Weight != before.Weight || after.RestingHr != before.RestingHr {
		t.Fatalf("a refused report changed the record: %+v -> %+v", before, after)
	}
	// too big
	if status, _ := report(t, env, tok, `{"weight":72,"x":"`+strings.Repeat("a", 5000)+`"}`); status != http.StatusBadRequest {
		t.Fatalf("big body: %d", status)
	}

	// tomorrow is allowed (time zones), the day after is not
	tomorrow := time.Now().In(env.App.Deps.Config.Location).AddDate(0, 0, 1).Format(time.DateOnly)
	if status, msg := report(t, env, tok, `{"date":"`+tomorrow+`","weight":71}`); status != http.StatusNoContent {
		t.Fatalf("tomorrow: %d %s", status, msg)
	}
}

func TestBodyTrendAndRecordActions(t *testing.T) {
	env, _ := setup(t)
	ctx := context.Background()
	run := func(name, input string) (any, error) {
		return env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
	}
	loc := env.App.Deps.Config.Location
	at := func(ago int) string { return time.Now().In(loc).AddDate(0, 0, -ago).Format(time.DateOnly) }

	// empty: every metric has no values
	out, err := run("habits.body_trend", `{}`)
	if err != nil {
		t.Fatal(err)
	}
	empty := decodeTrend(t, out)
	if len(empty.Series) != 5 || empty.Days != 90 {
		t.Fatalf("empty: %+v", empty)
	}
	for _, s := range empty.Series {
		if s.Count != 0 || s.Latest != nil || len(s.Points) != 0 {
			t.Fatalf("empty series has data: %+v", s)
		}
	}

	for ago, w := range map[int]string{20: "74", 10: "73", 3: "72", 0: "71"} {
		if _, err := run("habits.body_record", `{"date":"`+at(ago)+`","weight":`+w+`,"restingHr":60}`); err != nil {
			t.Fatal(err)
		}
	}
	// a record out of the range, and a day with only steps
	run("habits.body_record", `{"date":"`+at(60)+`","weight":90}`)
	run("habits.body_record", `{"date":"`+at(2)+`","steps":5000}`)

	out, err = run("habits.body_trend", `{"days":30}`)
	if err != nil {
		t.Fatal(err)
	}
	tr := decodeTrend(t, out)
	by := map[string]trendSeries{}
	for _, s := range tr.Series {
		by[s.Metric] = s
	}
	w := by["weight"]
	if w.Count != 4 || w.Unit != "kg" || *w.Latest != 71 || *w.Min != 71 || *w.Max != 74 || *w.Average != 72.5 || *w.Change != -3 {
		t.Fatalf("weight: %+v", w)
	}
	if w.Points[0].Date != at(20) || w.Points[3].Date != at(0) {
		t.Fatalf("points are not oldest first: %+v", w.Points)
	}
	if by["restingHr"].Count != 4 || by["steps"].Count != 1 || *by["steps"].Latest != 5000 || by["sleep"].Count != 0 {
		t.Fatalf("other metrics: %+v", by)
	}
	// 90 days sees the old record too
	out, _ = run("habits.body_trend", `{"days":90}`)
	if got := decodeTrend(t, out); func() int {
		for _, s := range got.Series {
			if s.Metric == "weight" {
				return s.Count
			}
		}
		return -1
	}() != 5 {
		t.Fatalf("90 days: %+v", got)
	}

	for _, bad := range []string{`{"days":1}`, `{"days":400}`, `{"days":"x"}`, `{"extra":1}`} {
		if _, err := run("habits.body_trend", bad); err == nil {
			t.Errorf("trend %s accepted", bad)
		}
	}
	for _, bad := range []string{`{}`, `{"weight":10}`, `{"restingHr":10}`, `{"weight":70,"x":1}`, `{"date":"2026-13-01","weight":70}`} {
		if _, err := run("habits.body_record", bad); err == nil {
			t.Errorf("record %s accepted", bad)
		}
	}
}

type trendPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}
type trendSeries struct {
	Metric  string       `json:"metric"`
	Unit    string       `json:"unit"`
	Count   int          `json:"count"`
	Latest  *float64     `json:"latest"`
	Average *float64     `json:"average"`
	Min     *float64     `json:"min"`
	Max     *float64     `json:"max"`
	Change  *float64     `json:"change"`
	Points  []trendPoint `json:"points"`
}
type trendResult struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Days   int           `json:"days"`
	Series []trendSeries `json:"series"`
}

func decodeTrend(t *testing.T, v any) trendResult {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out trendResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
