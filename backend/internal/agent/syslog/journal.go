package syslog

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	journalPermissionMsg = "代理没有读取系统日志的权限。在服务器上执行：sudo usermod -aG systemd-journal <代理运行的用户>，然后重启代理。"
	// unitScan is how many entries the unit list looks at.
	unitScan = 20000
	// grepScan is how many entries are read per wanted entry when the keyword
	// has to be tested by the agent.
	grepScan = 10
)

// cmdRunner starts journalctl. wait is called after stdout is read to the end
// or closed, and returns what the command reported.
type cmdRunner func(ctx context.Context, args []string) (stdout io.ReadCloser, wait func() error, err error)

// journal reads the systemd journal through journalctl. Every argument is a
// separate string handed to exec, so no shell is involved.
type journal struct {
	run cmdRunner
	// grep is true when journalctl can filter by keyword itself (systemd 244
	// and later). Otherwise the agent reads more entries and filters them.
	grep bool
}

// newJournal builds the backend for the journalctl on this machine.
func newJournal() *journal {
	j := &journal{run: execJournalctl}
	if out, err := exec.Command("journalctl", "--version").Output(); err == nil {
		j.grep = versionAtLeast(string(out), 244)
	}
	return j
}

var versionRe = regexp.MustCompile(`systemd (\d+)`)

func versionAtLeast(out string, n int) bool {
	m := versionRe.FindStringSubmatch(out)
	if m == nil {
		return false
	}
	v, _ := strconv.Atoi(m[1])
	return v >= n
}

// execJournalctl is the real cmdRunner.
func execJournalctl(ctx context.Context, args []string) (io.ReadCloser, func() error, error) {
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, rpcutil.Failed("journalctl: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, rpcutil.Failed("journalctl: %v", err)
	}
	wait := func() error {
		err := cmd.Wait()
		if ctx.Err() != nil {
			return nil
		}
		return journalError(stderr.String(), err)
	}
	return out, wait, nil
}

// limitedBuffer keeps the first 8 KB written to it.
type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := 8<<10 - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// journalError turns what journalctl printed into a protocol error.
func journalError(stderr string, err error) error {
	if strings.Contains(stderr, "insufficient permissions") || strings.Contains(stderr, "Permission denied") {
		return &protocol.Error{Code: protocol.CodeSyslogPermission, Message: journalPermissionMsg}
	}
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = err.Error()
	}
	return rpcutil.Failed("journalctl: %s", msg)
}

var unitSuffixes = []string{".service", ".socket", ".timer", ".mount", ".target", ".path", ".scope", ".slice", ".swap", ".automount", ".device"}

var cursorRe = regexp.MustCompile(`^[A-Za-z0-9=;_.-]{1,500}$`)

// filterArgs turns the filters into journalctl arguments. Match arguments
// (FIELD=value) have to come after the options, so they are returned apart.
func (j *journal) filterArgs(since, until *time.Time, priority *int, unit, grep string) (opts, matches []string, err error) {
	const layout = "2006-01-02 15:04:05"
	if since != nil {
		opts = append(opts, "--since="+since.In(time.Local).Format(layout))
	}
	if until != nil {
		opts = append(opts, "--until="+until.In(time.Local).Format(layout))
	}
	if priority != nil {
		opts = append(opts, "--priority=0.."+strconv.Itoa(*priority))
	}
	if unit != "" {
		if !unitRe.MatchString(unit) {
			return nil, nil, rpcutil.BadParams("invalid unit %q", unit)
		}
		isUnit := false
		for _, s := range unitSuffixes {
			isUnit = isUnit || strings.HasSuffix(unit, s)
		}
		if isUnit {
			opts = append(opts, "--unit="+unit)
		} else {
			matches = append(matches, "SYSLOG_IDENTIFIER="+unit)
		}
	}
	if grep != "" && j.grep {
		opts = append(opts, "--grep="+regexp.QuoteMeta(grep), "--case-sensitive=no")
	}
	return opts, matches, nil
}

// journalRow is one parsed line of journalctl -o json.
type journalRow struct {
	entry  protocol.SyslogEntry
	cursor string
}

// parseJournalLine reads one line of journalctl -o json. ok is false for
// lines that are not entries.
func parseJournalLine(line []byte) (row journalRow, ok bool) {
	var f map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(line), &f); err != nil {
		return row, false
	}
	usec, err := strconv.ParseInt(fieldString(f["__REALTIME_TIMESTAMP"]), 10, 64)
	if err != nil {
		return row, false
	}
	e := &row.entry
	e.Time = time.UnixMicro(usec).UTC()
	e.Priority = 6
	if p, err := strconv.Atoi(fieldString(f["PRIORITY"])); err == nil && p >= 0 && p <= 7 {
		e.Priority = p
	}
	e.Unit = fieldString(f["_SYSTEMD_UNIT"])
	if e.Unit == "" {
		e.Unit = fieldString(f["SYSLOG_IDENTIFIER"])
	}
	e.PID, _ = strconv.Atoi(fieldString(f["_PID"]))
	e.Message = clipMessage(fieldString(f["MESSAGE"]))
	row.cursor = fieldString(f["__CURSOR"])
	return row, true
}

// fieldString reads a journal field. It is a string, a list of numbers when
// journald marks the value as binary, or a list when the field repeats.
func fieldString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		_ = json.Unmarshal(raw, &s)
		return s
	case '[':
		var bytesList []int
		if json.Unmarshal(raw, &bytesList) == nil {
			b := make([]byte, 0, len(bytesList))
			for _, v := range bytesList {
				b = append(b, byte(v))
			}
			return string(b)
		}
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil && len(list) > 0 {
			return fieldString(list[0])
		}
	}
	return ""
}

// readRows reads journalctl output and calls fn for every entry. A false
// return from fn stops reading.
func readRows(r io.Reader, fn func(journalRow) bool) error {
	br := bufio.NewReaderSize(r, 256<<10)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && len(line) < 4<<20 {
			if row, ok := parseJournalLine(line); ok && !fn(row) {
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// collect runs journalctl and returns the entries it printed.
func (j *journal) collect(ctx context.Context, args []string) ([]journalRow, error) {
	out, wait, err := j.run(ctx, args)
	if err != nil {
		return nil, err
	}
	var rows []journalRow
	rerr := readRows(out, func(r journalRow) bool { rows = append(rows, r); return true })
	out.Close()
	if werr := wait(); werr != nil {
		return nil, werr
	}
	return rows, rerr
}

func (j *journal) query(ctx context.Context, p protocol.SyslogQueryParams) (protocol.SyslogPage, error) {
	opts, matches, err := j.filterArgs(p.Since, p.Until, p.Priority, p.Unit, p.Grep)
	if err != nil {
		return protocol.SyslogPage{}, err
	}
	want := p.Limit + 1 // one more than shown tells whether older entries exist
	if p.Cursor != "" {
		if !cursorRe.MatchString(p.Cursor) {
			return protocol.SyslogPage{}, rpcutil.BadParams("invalid cursor")
		}
		want++ // the entry at the cursor comes first and is dropped
	}
	agentGrep := p.Grep != "" && !j.grep
	fetch := want
	if agentGrep {
		fetch = min(want*grepScan, unitScan)
	}
	args := []string{"-o", "json", "--no-pager", "-r", "-n", strconv.Itoa(fetch)}
	args = append(args, opts...)
	if p.Cursor != "" {
		args = append(args, "--cursor="+p.Cursor)
	}
	args = append(args, matches...)
	rows, err := j.collect(ctx, args)
	if err != nil {
		return protocol.SyslogPage{}, err
	}
	scanned := len(rows)
	if p.Cursor != "" && len(rows) > 0 && rows[0].cursor == p.Cursor {
		rows = rows[1:]
	}
	oldestScanned := ""
	if len(rows) > 0 {
		oldestScanned = rows[len(rows)-1].cursor
	}
	if agentGrep {
		kept := rows[:0]
		for _, r := range rows {
			if matchGrep(p.Grep, r.entry.Message) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	page := protocol.SyslogPage{Items: []protocol.SyslogEntry{}}
	if len(rows) > p.Limit {
		rows = rows[:p.Limit]
		page.Cursor = rows[len(rows)-1].cursor
	} else if agentGrep && scanned >= fetch {
		// The scan ended before the journal did: go on from where it stopped.
		page.Cursor = oldestScanned
	}
	for i := len(rows) - 1; i >= 0; i-- {
		page.Items = append(page.Items, rows[i].entry)
	}
	return page, nil
}

func (j *journal) units(ctx context.Context) ([]string, error) {
	args := []string{"-o", "json", "--no-pager", "-r", "-n", strconv.Itoa(unitScan), "--since=7 days ago",
		"--output-fields=_SYSTEMD_UNIT,SYSLOG_IDENTIFIER"}
	rows, err := j.collect(ctx, args)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.entry.Unit]++
	}
	return countUnits(counts), nil
}

func (j *journal) follow(ctx context.Context, p protocol.SyslogFollowParams, emit func([]protocol.SyslogEntry) error) error {
	opts, matches, err := j.filterArgs(nil, nil, p.Priority, p.Unit, p.Grep)
	if err != nil {
		return err
	}
	args := append([]string{"-o", "json", "--no-pager", "-f", "-n", "0"}, opts...)
	args = append(args, matches...)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, wait, err := j.run(ctx, args)
	if err != nil {
		return err
	}
	ch := make(chan protocol.SyslogEntry, 256)
	readErr := make(chan error, 1)
	agentGrep := p.Grep != "" && !j.grep
	go func() {
		err := readRows(out, func(r journalRow) bool {
			if agentGrep && !matchGrep(p.Grep, r.entry.Message) {
				return true
			}
			select {
			case ch <- r.entry:
				return true
			case <-ctx.Done():
				return false
			}
		})
		readErr <- err
		close(ch)
	}()
	err = batchEmit(ctx, ch, emit)
	cancel()
	out.Close()
	werr := wait()
	if rerr := <-readErr; err == nil && ctx.Err() == nil {
		err = rerr
	}
	if err == nil && werr != nil {
		err = werr
	}
	return err
}
