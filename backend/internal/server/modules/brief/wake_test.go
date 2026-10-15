package brief_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
)

func TestSendModeSettings(t *testing.T) {
	env, _, ws := setup(t)
	view := putSettings(t, env, baseSettings(ws))
	if view.SendMode == nil || *view.SendMode != "time" {
		t.Fatalf("default mode: %+v", view.SendMode)
	}
	s := baseSettings(ws)
	s["sendMode"] = "wake"
	s["time"] = "09:30"
	if view = putSettings(t, env, s); *view.SendMode != "wake" || view.Time != "09:30" || view.NextRunAt == nil {
		t.Fatalf("wake: %+v", view)
	}
	// An update without sendMode keeps the stored one.
	if view = putSettings(t, env, baseSettings(ws)); *view.SendMode != "wake" {
		t.Fatalf("kept: %v", *view.SendMode)
	}
	s["sendMode"] = "noon"
	if status, _ := env.Do(http.MethodPut, "/briefs/settings", s, nil); status != http.StatusBadRequest {
		t.Fatalf("bad mode: %d", status)
	}
}

func TestBriefAfterWakeUp(t *testing.T) {
	env, m, ws := setup(t)
	ch := &fakeChannel{name: "fake"}
	env.App.Deps.Notify.RegisterChannel(ch)
	s := baseSettings(ws)
	s["channels"] = []string{"fake"}
	s["sendMode"] = "wake"
	s["time"] = "09:00"
	putSettings(t, env, s)
	ctx := context.Background()

	// Waking up in the small hours does not count.
	if err := brief.OnWake(m, ctx, at(27, 3, 10)); err != nil || ch.count() != 0 {
		t.Fatalf("03:10 wake: %d %v", ch.count(), err)
	}
	// The latest time has not come yet: nothing.
	_ = brief.Tick(m, ctx, at(27, 7, 0))
	if ch.count() != 0 {
		t.Fatal("sent before the wake-up")
	}
	if err := brief.OnWake(m, ctx, at(27, 7, 12)); err != nil || ch.count() != 1 {
		t.Fatalf("wake: %d %v", ch.count(), err)
	}
	// Not again: a second wake-up, and the latest time.
	_ = brief.OnWake(m, ctx, at(27, 8, 0))
	_ = brief.Tick(m, ctx, at(27, 9, 0))
	if ch.count() != 1 {
		t.Fatalf("sent twice: %d", ch.count())
	}

	// No wake-up the next day: the latest time sends it.
	_ = brief.Tick(m, ctx, at(28, 9, 0))
	if ch.count() != 2 {
		t.Fatalf("latest time: %d", ch.count())
	}

	// With a fixed time a wake-up does nothing.
	s["sendMode"] = "time"
	putSettings(t, env, s)
	_ = brief.OnWake(m, ctx, at(29, 7, 0))
	if ch.count() != 2 {
		t.Fatalf("fixed time mode sent on wake-up: %d", ch.count())
	}
}

func TestCompareHighAndRainSpans(t *testing.T) {
	for _, c := range []struct {
		today, yesterday float64
		want             string
	}{
		{24, 30, "最高 24°，比昨天低 6°。"},
		{28, 22, "最高 28°，比昨天高 6°。"},
		{24, 26, "最高 24°，和昨天差不多。"},
		{24, 21, "最高 24°，和昨天差不多。"},
	} {
		if got := brief.CompareHigh(c.today, c.yesterday); got != c.want {
			t.Errorf("%v vs %v: %q", c.today, c.yesterday, got)
		}
	}
	for _, c := range []struct {
		hours []int
		want  string
	}{
		{nil, ""},
		{[]int{15, 16, 17}, "15 点到 18 点有雨。"},
		{[]int{9, 15, 16}, "9 点到 10 点、15 点到 17 点有雨。"},
		{[]int{22, 23}, "22 点以后有雨。"},
	} {
		if got := brief.RainSpans(c.hours); got != c.want {
			t.Errorf("%v: %q", c.hours, got)
		}
	}
}

// briefWeatherFake answers the 和风天气 calls of the brief's weather section.
type briefWeatherFake struct {
	*httptest.Server
	mu         sync.Mutex
	historical string // "" answers 500
	hourly     string
	warnings   string
}

func TestBriefWeatherLines(t *testing.T) {
	env, m, ws := setup(t)
	now := time.Date(2026, 10, 27, 8, 0, 0, 0, shanghai)
	hour := func(h int, precip string) string {
		return fmt.Sprintf(`{"fxTime":%q,"precip":%q}`, time.Date(2026, 10, 27, h, 0, 0, 0, shanghai).Format("2006-01-02T15:04-07:00"), precip)
	}
	f := &briefWeatherFake{historical: `{"tempMax":"30","tempMin":"20"}`, warnings: `[{"id":"y1","title":"上海市气象台发布大风黄色预警","severityColor":"Yellow","pubTime":"2026-10-27T07:00+08:00"}]`}
	f.hourly = "[" + strings.Join([]string{hour(7, "0.5"), hour(9, "0.0"), hour(15, "0.2"), hour(16, "1.0"), hour(17, "0.3"), hour(18, "0.0")}, ",") + "]"
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch p := r.URL.Path; {
		case p == "/geo/v2/city/lookup":
			fmt.Fprint(w, `{"code":"200","location":[{"id":"101020100","name":"上海","lat":"31.23","lon":"121.47"}]}`)
		case p == "/v7/weather/now":
			fmt.Fprint(w, `{"code":"200","now":{"temp":"19","text":"多云","icon":"101","humidity":"70"}}`)
		case p == "/v7/weather/3d":
			fmt.Fprint(w, `{"code":"200","daily":[{"tempMax":"24","tempMin":"18","pop":"40"}]}`)
		case p == "/v7/weather/24h":
			fmt.Fprintf(w, `{"code":"200","hourly":%s}`, f.hourly)
		case p == "/v7/historical/weather" && f.historical != "":
			fmt.Fprintf(w, `{"code":"200","weatherDaily":%s}`, f.historical)
		case p == "/v7/warning/now":
			fmt.Fprintf(w, `{"code":"200","warning":%s}`, f.warnings)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(f.Close)
	brief.SetQWeatherBase(m, f.URL)
	brief.SetNow(m, func() time.Time { return now })
	putSettings(t, env, baseSettings(ws))

	weather := func() string {
		t.Helper()
		brief.DropCaches(m)
		content, _, err := brief.Generate(m, context.Background(), module.NewRegistry(), now)
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	got := weather()
	want := "上海，多云，现在 19°C。今天 18 到 24°C，降水概率 40%。\n最高 24°，比昨天低 6°。\n15 点到 18 点有雨。\n预警：上海市气象台发布大风黄色预警。\n"
	if !strings.HasSuffix(got, want) {
		t.Fatalf("weather:\n%s", got)
	}

	// Yesterday cannot be read: that line is left out, the rest stays.
	f.mu.Lock()
	f.historical = ""
	f.warnings = "[]"
	f.mu.Unlock()
	got = weather()
	if strings.Contains(got, "昨天") || !strings.Contains(got, "15 点到 18 点有雨。") {
		t.Fatalf("without yesterday:\n%s", got)
	}
}
