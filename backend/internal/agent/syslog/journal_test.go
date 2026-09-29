package syslog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// journalLine builds a journalctl -o json line.
func journalLine(i int, priority, unit, message string) string {
	unitField := ""
	if unit != "" {
		unitField = fmt.Sprintf(`"_SYSTEMD_UNIT":"%s",`, unit)
	}
	return fmt.Sprintf(`{"__CURSOR":"s=x;i=%d","__REALTIME_TIMESTAMP":"%d",%s"PRIORITY":"%s","_PID":"%d","SYSLOG_IDENTIFIER":"prog","MESSAGE":"%s"}`+"\n",
		i, time.Date(2026, 9, 29, 10, 0, i, 0, time.UTC).UnixMicro(), unitField, priority, 100+i, message)
}

// fakeJournalctl records the arguments and answers from a function.
type fakeJournalctl struct {
	mu    sync.Mutex
	calls [][]string
	reply func(args []string) (string, error)
}

func (f *fakeJournalctl) run(ctx context.Context, args []string) (io.ReadCloser, func() error, error) {
	f.mu.Lock()
	f.calls = append(f.calls, args)
	f.mu.Unlock()
	out, err := f.reply(args)
	return io.NopCloser(strings.NewReader(out)), func() error { return err }, nil
}

func (f *fakeJournalctl) last() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestParseJournalLine(t *testing.T) {
	row, ok := parseJournalLine([]byte(journalLine(5, "3", "ssh.service", "Failed password")))
	if !ok || row.entry.Priority != 3 || row.entry.Unit != "ssh.service" || row.entry.PID != 105 || row.entry.Message != "Failed password" ||
		row.cursor != "s=x;i=5" || !row.entry.Time.Equal(time.Date(2026, 9, 29, 10, 0, 5, 0, time.UTC)) {
		t.Fatalf("row: %+v %v", row, ok)
	}
	// No unit: the program name is used. MESSAGE as a list of bytes is text.
	row, ok = parseJournalLine([]byte(`{"__CURSOR":"c","__REALTIME_TIMESTAMP":"1000000","SYSLOG_IDENTIFIER":"sudo","MESSAGE":[104,105,32,228,184,150]}`))
	if !ok || row.entry.Unit != "sudo" || row.entry.Message != "hi 世" || row.entry.Priority != 6 {
		t.Fatalf("binary message: %+v", row)
	}
	// A repeated field is a list of strings; the first one counts.
	row, _ = parseJournalLine([]byte(`{"__REALTIME_TIMESTAMP":"1000000","_SYSTEMD_UNIT":["a.service","b.service"],"MESSAGE":"x"}`))
	if row.entry.Unit != "a.service" {
		t.Fatalf("repeated field: %+v", row)
	}
	for _, bad := range []string{"", "not json", `{"MESSAGE":"no time"}`} {
		if _, ok := parseJournalLine([]byte(bad)); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	long, _ := parseJournalLine([]byte(`{"__REALTIME_TIMESTAMP":"1","MESSAGE":"` + strings.Repeat("字", 10000) + `"}`))
	if !strings.HasSuffix(long.entry.Message, "…") || len(long.entry.Message) > maxMessage+len("…") {
		t.Fatalf("long message: %d bytes", len(long.entry.Message))
	}
}

func TestJournalQueryArguments(t *testing.T) {
	f := &fakeJournalctl{reply: func([]string) (string, error) { return "", nil }}
	j := &journal{run: f.run, grep: true}
	since := time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)
	prio := 3
	_, err := j.query(context.Background(), protocol.SyslogQueryParams{Since: &since, Priority: &prio, Unit: "ssh.service", Grep: "a.b (x)", Limit: 50, Cursor: "s=x;i=9"})
	if err != nil {
		t.Fatal(err)
	}
	args := f.last()
	// 50 wanted, one more to see whether older ones exist, one more for the cursor entry.
	for _, want := range []string{"-r", "52", "--priority=0..3", "--unit=ssh.service", `--grep=a\.b \(x\)`, "--case-sensitive=no", "--cursor=s=x;i=9",
		"--since=" + since.In(time.Local).Format("2006-01-02 15:04:05")} {
		if !hasArg(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}
	// A name that is not a unit is matched as the syslog identifier, after the options.
	_, _ = j.query(context.Background(), protocol.SyslogQueryParams{Unit: "sudo", Limit: 10})
	args = f.last()
	if args[len(args)-1] != "SYSLOG_IDENTIFIER=sudo" || hasArg(args, "--unit=sudo") {
		t.Errorf("identifier match: %q", args)
	}
	// Anything that is not a plain name is refused before a process starts.
	n := len(f.calls)
	for _, unit := range []string{"a;rm -rf /", "x y", "-o", "$(id)", "a\nb", strings.Repeat("a", 129)} {
		if _, err := j.query(context.Background(), protocol.SyslogQueryParams{Unit: unit, Limit: 10}); err == nil {
			t.Errorf("unit %q accepted", unit)
		}
	}
	if _, err := j.query(context.Background(), protocol.SyslogQueryParams{Cursor: "x; rm", Limit: 10}); err == nil {
		t.Error("bad cursor accepted")
	}
	if len(f.calls) != n {
		t.Errorf("journalctl ran for a refused request")
	}
}

func TestJournalPaging(t *testing.T) {
	// The journal has entries 1 to 30. journalctl -r gives the newest first.
	all := 30
	f := &fakeJournalctl{reply: func(args []string) (string, error) {
		var n int
		fmt.Sscan(args[hasIndex(args, "-n")+1], &n)
		start := all
		for _, a := range args {
			if c, ok := strings.CutPrefix(a, "--cursor=s=x;i="); ok {
				fmt.Sscan(c, &start) // starts at the cursor entry itself
			}
		}
		var b strings.Builder
		for i := start; i >= 1 && n > 0; i, n = i-1, n-1 {
			b.WriteString(journalLine(i, "6", "a.service", fmt.Sprint("m", i)))
		}
		return b.String(), nil
	}}
	j := &journal{run: f.run, grep: true}
	page, err := j.query(context.Background(), protocol.SyslogQueryParams{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 10 || page.Items[0].Message != "m21" || page.Items[9].Message != "m30" || page.Cursor != "s=x;i=21" {
		t.Fatalf("first page: %v cursor %q", messages(page), page.Cursor)
	}
	page, err = j.query(context.Background(), protocol.SyslogQueryParams{Limit: 10, Cursor: page.Cursor})
	if err != nil || len(page.Items) != 10 || page.Items[0].Message != "m11" || page.Items[9].Message != "m20" || page.Cursor != "s=x;i=11" {
		t.Fatalf("second page: %v cursor %q %v", messages(page), page.Cursor, err)
	}
	page, err = j.query(context.Background(), protocol.SyslogQueryParams{Limit: 10, Cursor: page.Cursor})
	if err != nil || len(page.Items) != 10 || page.Items[0].Message != "m1" || page.Cursor != "" {
		t.Fatalf("last page: %v cursor %q %v", messages(page), page.Cursor, err)
	}
}

func hasIndex(args []string, want string) int {
	for i, a := range args {
		if a == want {
			return i
		}
	}
	return -1
}

func messages(p protocol.SyslogPage) []string {
	var out []string
	for _, e := range p.Items {
		out = append(out, e.Message)
	}
	return out
}

func TestJournalWithoutGrepSupport(t *testing.T) {
	f := &fakeJournalctl{reply: func(args []string) (string, error) {
		var b strings.Builder
		for i := 30; i >= 1; i-- {
			msg := "plain"
			if i%10 == 0 {
				msg = "Needle here"
			}
			b.WriteString(journalLine(i, "6", "a.service", msg))
		}
		return b.String(), nil
	}}
	j := &journal{run: f.run, grep: false}
	page, err := j.query(context.Background(), protocol.SyslogQueryParams{Grep: "needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if hasArg(f.last(), "--case-sensitive=no") || len(page.Items) != 3 || page.Items[0].Message != "Needle here" {
		t.Fatalf("filtered by the agent: %v args %q", messages(page), f.last())
	}
	if !versionAtLeast("systemd 249 (249.11-ubuntu3)", 244) || versionAtLeast("systemd 237 (237)", 244) || versionAtLeast("nonsense", 244) {
		t.Fatal("version check")
	}
}

func TestJournalUnits(t *testing.T) {
	f := &fakeJournalctl{reply: func(args []string) (string, error) {
		return journalLine(1, "6", "b.service", "x") + journalLine(2, "6", "a.service", "x") + journalLine(3, "6", "b.service", "x") +
			journalLine(4, "6", "", "x"), nil
	}}
	units, err := (&journal{run: f.run}).units(context.Background())
	if err != nil || fmt.Sprint(units) != "[b.service a.service prog]" {
		t.Fatalf("units: %v %v", units, err)
	}
	if !hasArg(f.last(), "--since=7 days ago") {
		t.Fatalf("args: %q", f.last())
	}
}

func TestJournalErrors(t *testing.T) {
	err := journalError("No journal files were opened due to insufficient permissions.", errors.New("exit status 1"))
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != protocol.CodeSyslogPermission || !strings.Contains(pe.Message, "usermod -aG systemd-journal") {
		t.Fatalf("permission: %v", err)
	}
	if err := journalError("", nil); err != nil {
		t.Fatalf("no error: %v", err)
	}
	if err := journalError("boom", errors.New("exit status 1")); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("other: %v", err)
	}
	f := &fakeJournalctl{reply: func([]string) (string, error) { return "", journalError("insufficient permissions", errors.New("x")) }}
	if _, err := (&journal{run: f.run}).query(context.Background(), protocol.SyslogQueryParams{Limit: 5}); !errors.As(err, &pe) || pe.Code != protocol.CodeSyslogPermission {
		t.Fatalf("query error: %v", err)
	}
}

func TestJournalFollow(t *testing.T) {
	pr, pw := io.Pipe()
	var started []string
	j := &journal{grep: false, run: func(ctx context.Context, args []string) (io.ReadCloser, func() error, error) {
		started = args
		go func() { <-ctx.Done(); pw.Close() }()
		return pr, func() error { return nil }, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []protocol.SyslogEntry, 4)
	done := make(chan error, 1)
	go func() {
		done <- j.follow(ctx, protocol.SyslogFollowParams{Grep: "hit"}, func(b []protocol.SyslogEntry) error { got <- b; return nil })
	}()
	go func() {
		io.WriteString(pw, journalLine(1, "6", "a.service", "miss"))
		io.WriteString(pw, journalLine(2, "6", "a.service", "a HIT"))
		io.WriteString(pw, journalLine(3, "6", "a.service", "another hit"))
	}()
	var all []string
	deadline := time.After(3 * time.Second)
	for len(all) < 2 {
		select {
		case b := <-got:
			for _, e := range b {
				all = append(all, e.Message)
			}
		case <-deadline:
			t.Fatalf("got only %v", all)
		}
	}
	if fmt.Sprint(all) != "[a HIT another hit]" || !hasArg(started, "-f") || !hasArg(started, "0") {
		t.Fatalf("follow: %v args %q", all, started)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("follow did not end")
	}
}

func TestBatchEmitSplitsBigBatches(t *testing.T) {
	ch := make(chan protocol.SyslogEntry, 500)
	for i := 0; i < 450; i++ {
		ch <- protocol.SyslogEntry{Message: fmt.Sprint(i)}
	}
	close(ch)
	var sizes []int
	err := batchEmit(context.Background(), ch, func(b []protocol.SyslogEntry) error { sizes = append(sizes, len(b)); return nil })
	if err != nil || fmt.Sprint(sizes) != "[200 200 50]" {
		t.Fatalf("sizes %v %v", sizes, err)
	}
}

// ---- plain syslog files ----

func TestParseTextLine(t *testing.T) {
	loc := time.FixedZone("x", 8*3600)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, loc)
	e, ok := parseTextLine("Sep 29 10:00:01 host sshd[123]: Failed password", now, loc)
	if !ok || e.Unit != "sshd" || e.PID != 123 || e.Message != "Failed password" || !e.Time.Equal(time.Date(2026, 9, 29, 10, 0, 1, 0, loc)) {
		t.Fatalf("classic: %+v %v", e, ok)
	}
	e, ok = parseTextLine("Sep  3 04:05:06 host kernel: [ 12.3] usb 1-1", now, loc)
	if !ok || e.Unit != "kernel" || e.PID != 0 || e.Message != "[ 12.3] usb 1-1" || e.Time.Day() != 2 && e.Time.Day() != 3 {
		t.Fatalf("no pid: %+v %v", e, ok)
	}
	e, ok = parseTextLine("2026-09-29T10:00:01.123456+08:00 host CRON[9]: (root) CMD (run)", now, loc)
	if !ok || e.Unit != "CRON" || e.PID != 9 || e.Message != "(root) CMD (run)" || !e.Time.Equal(time.Date(2026, 9, 29, 2, 0, 1, 123456000, time.UTC)) {
		t.Fatalf("iso: %+v %v", e, ok)
	}
	// A December line read in January belongs to last year.
	jan := time.Date(2027, 1, 2, 0, 0, 0, 0, loc)
	if e, _ := parseTextLine("Dec 31 23:59:59 host a: b", jan, loc); e.Time.Year() != 2026 {
		t.Fatalf("year: %v", e.Time)
	}
	for _, bad := range []string{"", "   continuation line", "Foo 99 10:00:00 host a: b"} {
		if _, ok := parseTextLine(bad, now, loc); ok {
			t.Errorf("accepted %q", bad)
		}
	}
}

func writeLog(t *testing.T, lines int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "syslog")
	var b strings.Builder
	base := time.Now().Add(-time.Hour)
	for i := 1; i <= lines; i++ {
		prog := "app"
		if i%3 == 0 {
			prog = "cron"
		}
		msg := fmt.Sprint("message ", i)
		if i%10 == 0 {
			msg += " NEEDLE"
		}
		fmt.Fprintf(&b, "%s host %s[%d]: %s\n", base.Add(time.Duration(i)*time.Second).Format("Jan _2 15:04:05"), prog, i, msg)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTextLogQueryAndPaging(t *testing.T) {
	// 5000 lines cross several read chunks.
	tl := &textLog{path: writeLog(t, 5000)}
	ctx := context.Background()
	page, err := tl.query(ctx, protocol.SyslogQueryParams{Limit: 100})
	if err != nil || len(page.Items) != 100 || page.Items[99].Message != "message 5000 NEEDLE" || page.Items[0].Message != "message 4901" || page.Cursor == "" {
		t.Fatalf("first page: %v %q %v", len(page.Items), page.Cursor, err)
	}
	seen := 100
	for cursor := page.Cursor; cursor != ""; {
		page, err = tl.query(ctx, protocol.SyslogQueryParams{Limit: 1000, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		seen += len(page.Items)
		if len(page.Items) > 0 && page.Items[len(page.Items)-1].Message == "message 4901" {
			t.Fatal("a line came twice")
		}
		cursor = page.Cursor
	}
	if seen != 5000 {
		t.Fatalf("pages together have %d lines, want 5000", seen)
	}
	page, _ = tl.query(ctx, protocol.SyslogQueryParams{Limit: 5, Unit: "cron", Grep: "needle"})
	if fmt.Sprint(messages(page)) != "[message 4860 NEEDLE message 4890 NEEDLE message 4920 NEEDLE message 4950 NEEDLE message 4980 NEEDLE]" {
		t.Fatalf("unit and keyword: %v", messages(page))
	}
	// The window: the lines are one second apart from an hour ago, so the last 30 minutes hold about 3200.
	since := time.Now().Add(-30 * time.Minute)
	page, _ = tl.query(ctx, protocol.SyslogQueryParams{Limit: 5000, Since: &since})
	if len(page.Items) < 3195 || len(page.Items) > 3205 || page.Cursor != "" {
		t.Fatalf("since: %d lines, cursor %q", len(page.Items), page.Cursor)
	}
	// There are no levels in this file, so a level filter finds nothing.
	prio := 3
	page, err = tl.query(ctx, protocol.SyslogQueryParams{Limit: 10, Priority: &prio})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("priority: %v %v", page.Items, err)
	}
	if _, err := tl.query(ctx, protocol.SyslogQueryParams{Limit: 10, Cursor: "junk"}); err == nil {
		t.Fatal("bad cursor accepted")
	}
	units, err := tl.units(ctx)
	if err != nil || fmt.Sprint(units) != "[app cron]" {
		t.Fatalf("units: %v %v", units, err)
	}
}

func TestTextLogPermissionAndMissing(t *testing.T) {
	dir := t.TempDir()
	_, err := (&textLog{path: filepath.Join(dir, "none")}).query(context.Background(), protocol.SyslogQueryParams{Limit: 5})
	var pe *protocol.Error
	if !errors.As(err, &pe) || pe.Code != protocol.CodeNotFound {
		t.Fatalf("missing: %v", err)
	}
	if os.Getuid() == 0 {
		t.Skip("root reads everything")
	}
	path := filepath.Join(dir, "secret")
	os.WriteFile(path, []byte("x\n"), 0o000)
	_, err = (&textLog{path: path}).query(context.Background(), protocol.SyslogQueryParams{Limit: 5})
	if !errors.As(err, &pe) || pe.Code != protocol.CodeSyslogPermission || !strings.Contains(pe.Message, "adm") {
		t.Fatalf("permission: %v", err)
	}
}

func TestTextLogFollowAndRotation(t *testing.T) {
	path := writeLog(t, 3)
	tl := &textLog{path: path}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan []protocol.SyslogEntry, 8)
	go tl.follow(ctx, protocol.SyslogFollowParams{Unit: "app"}, func(b []protocol.SyslogEntry) error { got <- b; return nil })
	time.Sleep(200 * time.Millisecond)
	line := func(prog, msg string) string {
		return fmt.Sprintf("%s host %s[1]: %s\n", time.Now().Format("Jan _2 15:04:05"), prog, msg)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(line("app", "one") + line("cron", "skipped") + "Sep 29 10:00:00 host app[1]: half")
	f.Close()
	want := func(msg string) {
		t.Helper()
		select {
		case b := <-got:
			if len(b) != 1 || b[0].Message != msg {
				t.Fatalf("batch %+v, want %q", b, msg)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("no batch with %q", msg)
		}
	}
	want("one") // the half line waits for its end
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(" line\n")
	f.Close()
	want("half line")
	// Rotation: a smaller file is read from the start.
	os.WriteFile(path, []byte(line("app", "after rotation")), 0o644)
	want("after rotation")
}

func TestScanBackShortFileAndEmpty(t *testing.T) {
	for _, content := range []string{"", "one", "one\n", "one\ntwo", "\n\none\n\ntwo\n"} {
		var got []string
		reached, err := scanBack(strings.NewReader(content), int64(len(content)), 1<<20, func(l string, start int64) bool {
			if content[start:start+int64(len(l))] != l {
				t.Errorf("%q: line %q is not at offset %d", content, l, start)
			}
			got = append(got, l)
			return true
		})
		want := []string{}
		for _, l := range strings.Split(content, "\n") {
			if l != "" {
				want = append([]string{l}, want...)
			}
		}
		if err != nil || !reached || fmt.Sprint(got) != fmt.Sprint(want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("%q: got %v reached %v %v, want %v", content, got, reached, err, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	p := protocol.SyslogQueryParams{}
	if err := normalize(&p); err != nil || p.Limit != 500 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p = protocol.SyslogQueryParams{Limit: 99999}
	_ = normalize(&p)
	if p.Limit != 5000 {
		t.Fatalf("limit: %d", p.Limit)
	}
	bad := 9
	for _, q := range []protocol.SyslogQueryParams{{Priority: &bad}, {Grep: strings.Repeat("a", 201)}, {Grep: "a\nb"}} {
		if err := normalize(&q); err == nil {
			t.Errorf("accepted %+v", q)
		}
	}
}
