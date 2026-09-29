package syslog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func event(record int64, level int, provider, at, message string) string {
	return fmt.Sprintf(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System><Provider Name='%s'/><EventID>7</EventID><Level>%d</Level>`+
		`<TimeCreated SystemTime='%s'/><EventRecordID>%d</EventRecordID><Execution ProcessID='44' ThreadID='2'/></System>`+
		`<EventData><Data>raw data</Data></EventData><RenderingInfo Culture='en-US'><Message>%s</Message></RenderingInfo></Event>`, provider, level, at, record, message)
}

func TestParseEvents(t *testing.T) {
	out := event(10, 2, "Service Control Manager", "2026-09-29T10:00:00.1234567Z", "The service failed") + "\r\n" +
		event(11, 4, "Kernel-General", "2026-09-29T10:00:05.0000000Z", "") + "\r\n" +
		`<Event><System><Provider Name='NoText'/><EventID>99</EventID><Level>3</Level><TimeCreated SystemTime='2026-09-29T10:00:06Z'/><EventRecordID>12</EventRecordID></System><EventData><Data>a</Data><Data> b </Data></EventData></Event>` +
		`<Event><System><Provider Name='BadTime'/><TimeCreated SystemTime='not a time'/></System></Event>`
	rows, err := parseEvents([]byte(out))
	if err != nil || len(rows) != 3 {
		t.Fatalf("rows: %+v %v", rows, err)
	}
	e := rows[0].entry
	if e.Unit != "Service Control Manager" || e.Priority != 3 || e.PID != 44 || e.Message != "The service failed" || rows[0].record != 10 ||
		!e.Time.Equal(time.Date(2026, 9, 29, 10, 0, 0, 123456700, time.UTC)) {
		t.Fatalf("first: %+v", rows[0])
	}
	// Without rendered text the raw data is used, and Information is priority 6.
	if rows[1].entry.Message != "raw data" || rows[1].entry.Priority != 6 {
		t.Fatalf("second: %+v", rows[1])
	}
	if rows[2].entry.Message != "a b" || rows[2].entry.Priority != 4 {
		t.Fatalf("third: %+v", rows[2])
	}
}

func TestBuildXPath(t *testing.T) {
	since := time.Date(2026, 9, 29, 1, 2, 3, 0, time.UTC)
	prio := 3
	got, err := buildXPath(&since, nil, time.Date(2026, 9, 29, 5, 0, 0, 500, time.UTC), &prio, "Kernel-General", 42)
	want := "*[System[Level<=2 and TimeCreated[@SystemTime>='2026-09-29T01:02:03.000Z'] and TimeCreated[@SystemTime<'2026-09-29T05:00:00.0000005Z'] and Provider[@Name='Kernel-General'] and EventRecordID>42]]"
	if err != nil || got != want {
		t.Fatalf("xpath:\n got %q\nwant %q (%v)", got, want, err)
	}
	if got, _ := buildXPath(nil, nil, time.Time{}, nil, "", 0); got != "*" {
		t.Fatalf("empty: %q", got)
	}
	debug := 7
	if got, _ := buildXPath(nil, nil, time.Time{}, &debug, "", 0); got != "*" {
		t.Fatalf("debug: %q", got)
	}
	for _, bad := range []string{"a'] or 1=1 or ['", `x"y`, "a<b", "a&b", "", strings.Repeat("a", 129)} {
		if bad == "" {
			continue
		}
		if _, err := buildXPath(nil, nil, time.Time{}, nil, bad, 0); err == nil {
			t.Errorf("source %q accepted", bad)
		}
	}
	for priority, level := range map[int]int{0: 1, 2: 1, 3: 2, 4: 3, 5: 4, 6: 4, 7: 5} {
		if levelForPriority(priority) != level {
			t.Errorf("level for %d: %d", priority, levelForPriority(priority))
		}
	}
}

func TestWevtQueryMergesLogs(t *testing.T) {
	var calls [][]string
	w := &wevt{run: func(ctx context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		switch args[1] {
		case "System":
			return []byte(event(3, 4, "A", "2026-09-29T10:00:03Z", "sys 3") + event(2, 4, "A", "2026-09-29T10:00:02Z", "sys 2")), nil
		}
		return []byte(event(9, 3, "B", "2026-09-29T10:00:04Z", "app 4") + event(8, 4, "B", "2026-09-29T10:00:01Z", "app 1")), nil
	}}
	page, err := w.query(context.Background(), protocol.SyslogQueryParams{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	// Newest three, oldest first; one more exists, so a cursor at the oldest shown.
	if fmt.Sprint(messages(page)) != "[sys 2 sys 3 app 4]" || page.Cursor != "t:2026-09-29T10:00:02Z" {
		t.Fatalf("page: %v cursor %q", messages(page), page.Cursor)
	}
	if calls[0][0] != "qe" || calls[0][2] != "/f:RenderedXml" || calls[0][3] != "/c:4" || calls[0][4] != "/rd:true" || calls[0][5] != "/q:*" {
		t.Fatalf("args: %q", calls[0])
	}
	page, _ = w.query(context.Background(), protocol.SyslogQueryParams{Limit: 10, Grep: "APP"})
	if fmt.Sprint(messages(page)) != "[app 1 app 4]" {
		t.Fatalf("grep: %v", messages(page))
	}
	if _, err := w.query(context.Background(), protocol.SyslogQueryParams{Limit: 10, Cursor: "off:5"}); err == nil {
		t.Fatal("bad cursor accepted")
	}
	units, err := w.units(context.Background())
	if err != nil || len(units) != 2 {
		t.Fatalf("units: %v %v", units, err)
	}
}

func TestWevtFollowReadsNewRecords(t *testing.T) {
	var seen []string
	round := 0
	w := &wevt{run: func(ctx context.Context, args ...string) ([]byte, error) {
		q := args[len(args)-1]
		seen = append(seen, q)
		if q == "/q:*" {
			return []byte(event(100, 4, "A", "2026-09-29T10:00:00Z", "old")), nil
		}
		if strings.Contains(q, "EventRecordID>100") && args[1] == "System" {
			round++
			return []byte(event(101, 4, "A", "2026-09-29T10:00:01Z", "new one")), nil
		}
		return nil, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []protocol.SyslogEntry, 2)
	go w.follow(ctx, protocol.SyslogFollowParams{}, func(b []protocol.SyslogEntry) error { got <- b; return nil })
	select {
	case b := <-got:
		if len(b) != 1 || b[0].Message != "new one" {
			t.Fatalf("batch: %+v", b)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("nothing followed; queries: %q", seen)
	}
}

func TestRegisterHandlers(t *testing.T) {
	reg := &registrar{h: map[string]rpc.Handler{}, s: map[string]rpc.StreamHandler{}}
	f := &fakeJournalctl{reply: func([]string) (string, error) { return journalLine(1, "6", "a.service", "hello"), nil }}
	RegisterBackend(reg, &journal{run: f.run, grep: true})
	out, err := reg.h[protocol.MethodSyslogQuery](context.Background(), json.RawMessage(`{"limit":10}`))
	if err != nil || len(out.(protocol.SyslogPage).Items) != 1 {
		t.Fatalf("query: %+v %v", out, err)
	}
	var pe *protocol.Error
	if _, err := reg.h[protocol.MethodSyslogQuery](context.Background(), json.RawMessage(`{"priority":12}`)); !errors.As(err, &pe) || pe.Code != protocol.CodeBadParams {
		t.Fatalf("bad priority: %v", err)
	}
	units, err := reg.h[protocol.MethodSyslogUnits](context.Background(), nil)
	if err != nil || len(units.(protocol.SyslogUnits).Items) != 1 {
		t.Fatalf("units: %+v %v", units, err)
	}
	if reg.s[protocol.MethodSyslogFollow] == nil {
		t.Fatal("follow not registered")
	}
}

type registrar struct {
	h map[string]rpc.Handler
	s map[string]rpc.StreamHandler
}

func (r *registrar) Handle(m string, h rpc.Handler)             { r.h[m] = h }
func (r *registrar) HandleStream(m string, h rpc.StreamHandler) { r.s[m] = h }
