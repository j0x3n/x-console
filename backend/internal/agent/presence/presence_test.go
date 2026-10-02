package presence

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWindowsSessionZeroIsUnknown(t *testing.T) {
	idle := func() (int64, bool) { t.Fatal("session 0 queried input"); return 0, true }
	locked := func() *bool { t.Fatal("session 0 queried desktop"); return new(false) }
	if sample := sampleUserSession(0, idle, locked); sample.Known || sample.Locked != nil || sample.DisplayOff != nil {
		t.Fatalf("session 0: %+v", sample)
	}
	if sample := sampleUserSession(1, func() (int64, bool) { return 30, true }, func() *bool { return new(true) }); !sample.Known || sample.IdleSeconds != 30 || sample.Locked == nil || !*sample.Locked {
		t.Fatalf("user session: %+v", sample)
	}
	if sample := sampleUserSession(1, func() (int64, bool) { return 0, false }, locked); sample.Known {
		t.Fatal("input failure reported known")
	}
}

func TestUnixSamples(t *testing.T) {
	cases := []struct {
		name            string
		replies         map[string]string
		wantKnown       bool
		idle            int64
		locked, display *bool
	}{
		{"linux x11", map[string]string{"xprintidle": "120000\n", "loginctl": "yes\n", "xset": "Monitor is Off"}, true, 120, new(true), new(true)},
		{"linux wayland", map[string]string{"gdbus": "(uint32 42,)"}, true, 42, nil, nil},
		{"linux unavailable", map[string]string{}, false, 0, nil, nil},
		{"linux invalid idle", map[string]string{"xprintidle": "-1", "gdbus": "(int64 -3,)"}, false, 0, nil, nil},
		{"linux malformed bus", map[string]string{"gdbus": "Error 42: not available"}, false, 0, nil, nil},
		{"linux lock without idle", map[string]string{"loginctl": "yes\n"}, false, 0, new(true), nil},
		{"macOS", map[string]string{"ioreg": "\"HIDIdleTime\" = 9000000000\n\"CGSSessionScreenIsLocked\" = Yes\n\"CurrentPowerState\"=0"}, true, 9, new(true), new(true)},
		{"macOS spacing", map[string]string{"ioreg": "\"HIDIdleTime\"=1000000000\n\"CGSSessionScreenIsLocked\"=true\n\"CurrentPowerState\"=4"}, true, 1, new(true), new(false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if out, ok := tc.replies[name]; ok {
					return []byte(out), nil
				}
				return nil, errors.New("unavailable")
			}
			var got Sample
			if strings.HasPrefix(tc.name, "macOS") {
				got = sampleDarwin(context.Background(), run)
			} else {
				got = sampleLinux(context.Background(), run, "session")
			}
			if got.Known != tc.wantKnown || got.IdleSeconds != tc.idle {
				t.Fatalf("sample: %+v", got)
			}
			if tc.locked != nil && (got.Locked == nil || *got.Locked != *tc.locked) {
				t.Fatalf("lock: %+v", got)
			}
			if tc.display != nil && (got.DisplayOff == nil || *got.DisplayOff != *tc.display) {
				t.Fatalf("display: %+v", got)
			}
		})
	}
}

func TestChangeReportsAndHeartbeat(t *testing.T) {
	base := time.Now()
	last := Sample{Known: true, IdleSeconds: 1}
	if shouldReport(last, Sample{Known: true, IdleSeconds: 2}, base, base.Add(2*time.Second)) {
		t.Fatal("idle drift reported")
	}
	if !shouldReport(last, Sample{Known: true, IdleSeconds: 0}, base, base.Add(2*time.Second)) {
		t.Fatal("input resume not reported")
	}
	if !shouldReport(last, Sample{Known: true, IdleSeconds: 30, Locked: new(true)}, base, base.Add(time.Second)) {
		t.Fatal("lock not reported")
	}
	if !shouldReport(last, last, base, base.Add(30*time.Second)) {
		t.Fatal("heartbeat not reported")
	}
	if !shouldReport(Sample{Known: true, IdleSeconds: 299}, Sample{Known: true, IdleSeconds: 300}, base, base.Add(2*time.Second)) {
		t.Fatal("idle threshold not reported")
	}
	if !shouldReport(last, Sample{Known: false}, base, base.Add(2*time.Second)) {
		t.Fatal("known transition not reported")
	}
	if !shouldReport(last, Sample{Known: true, IdleSeconds: 1, DisplayOff: new(true)}, base, base.Add(2*time.Second)) {
		t.Fatal("display change not reported")
	}
}
