package syslog

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	// textScanLimit is how many bytes a query reads from the end of the file.
	textScanLimit = 64 << 20
	textChunk     = 64 << 10
	// textFollowEvery is how often a followed file is checked.
	textFollowEvery = time.Second
)

// textLog reads a plain syslog file. It knows only time, program and text: the
// priority of every line is 6 (info), so a level filter below 6 finds nothing.
type textLog struct {
	path string
}

func (t *textLog) permissionError() error {
	return &protocol.Error{Code: protocol.CodeSyslogPermission,
		Message: "代理没有读取 " + t.path + " 的权限。在服务器上执行：sudo usermod -aG adm <代理运行的用户>，然后重启代理。"}
}

func (t *textLog) open() (*os.File, error) {
	f, err := os.Open(t.path)
	if errors.Is(err, fs.ErrPermission) {
		return nil, t.permissionError()
	}
	return f, rpcutil.FromOS(err)
}

var (
	// Sep 29 10:00:01 host prog[123]: text
	classicRe = regexp.MustCompile(`^([A-Z][a-z]{2}) +(\d{1,2}) (\d\d:\d\d:\d\d) \S+ ([^\s:\[]+)(?:\[(\d+)\])?: ?(.*)$`)
	// 2026-09-29T10:00:01.123456+08:00 host prog[123]: text
	isoRe = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[+-]\d\d:\d\d)) \S+ ([^\s:\[]+)(?:\[(\d+)\])?: ?(.*)$`)
)

// parseTextLine reads one syslog line. now is used to guess the year of the
// classic format, which has none.
func parseTextLine(line string, now time.Time, loc *time.Location) (protocol.SyslogEntry, bool) {
	line = strings.TrimRight(line, "\r")
	var e protocol.SyslogEntry
	e.Priority = 6
	if m := isoRe.FindStringSubmatch(line); m != nil {
		ts, err := time.Parse(time.RFC3339Nano, m[1])
		if err != nil {
			return e, false
		}
		e.Time, e.Unit, e.Message = ts.UTC(), m[2], m[4]
		e.PID, _ = strconv.Atoi(m[3])
		e.Message = clipMessage(e.Message)
		return e, true
	}
	m := classicRe.FindStringSubmatch(line)
	if m == nil {
		return e, false
	}
	year := now.In(loc).Year()
	ts, err := time.ParseInLocation("2006 Jan _2 15:04:05", strconv.Itoa(year)+" "+m[1]+" "+m[2]+" "+m[3], loc)
	if err != nil {
		return e, false
	}
	if ts.After(now.Add(24 * time.Hour)) {
		ts = ts.AddDate(-1, 0, 0) // a December line read in January
	}
	e.Time, e.Unit, e.Message = ts.UTC(), m[4], clipMessage(m[6])
	e.PID, _ = strconv.Atoi(m[5])
	return e, true
}

// scanBack calls fn with the lines of r from the end towards the start; start
// is the byte offset of the line. fn returns false to stop. At most maxBytes
// are read. It reports whether the start of the file was reached.
func scanBack(r io.ReaderAt, end, maxBytes int64, fn func(line string, start int64) bool) (bool, error) {
	pos, read := end, int64(0)
	var carry []byte // the beginning of a line whose start is further back
	for pos > 0 {
		if read >= maxBytes {
			return false, nil
		}
		from := max(0, pos-textChunk)
		buf := make([]byte, pos-from, pos-from+int64(len(carry)))
		if _, err := r.ReadAt(buf, from); err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		read += pos - from
		buf = append(buf, carry...)
		// buf starts at file offset from. Its first segment may be cut.
		cut := from > 0
		lineEnd := len(buf)
		for i := len(buf) - 1; i >= 0; i-- {
			if buf[i] != '\n' {
				continue
			}
			if line := buf[i+1 : lineEnd]; len(line) > 0 && !fn(string(line), from+int64(i+1)) {
				return false, nil
			}
			lineEnd = i
		}
		if cut {
			carry = append(carry[:0], buf[:lineEnd]...)
		} else {
			if lineEnd > 0 && !fn(string(buf[:lineEnd]), from) {
				return false, nil
			}
			carry = nil
		}
		pos = from
	}
	return true, nil
}

func (t *textLog) query(ctx context.Context, p protocol.SyslogQueryParams) (protocol.SyslogPage, error) {
	f, err := t.open()
	if err != nil {
		return protocol.SyslogPage{}, err
	}
	defer f.Close()
	end := int64(-1)
	if p.Cursor != "" {
		off, ok := strings.CutPrefix(p.Cursor, "off:")
		n, perr := strconv.ParseInt(off, 10, 64)
		if !ok || perr != nil || n < 0 {
			return protocol.SyslogPage{}, rpcutil.BadParams("invalid cursor")
		}
		end = n
	}
	if end < 0 {
		st, err := f.Stat()
		if err != nil {
			return protocol.SyslogPage{}, rpcutil.FromOS(err)
		}
		end = st.Size()
	}
	if p.Priority != nil && *p.Priority < 6 {
		return protocol.SyslogPage{Items: []protocol.SyslogEntry{}}, nil
	}
	now, loc := time.Now(), time.Local
	type hit struct {
		e     protocol.SyslogEntry
		start int64
	}
	var hits []hit // newest first
	var lastStart int64 = end
	reachedStart, err := scanBack(f, end, textScanLimit, func(line string, start int64) bool {
		if ctx.Err() != nil {
			return false
		}
		lastStart = start
		e, ok := parseTextLine(line, now, loc)
		if !ok {
			return true
		}
		if p.Until != nil && e.Time.After(*p.Until) {
			return true
		}
		if p.Since != nil && e.Time.Before(*p.Since) {
			lastStart = -1 // everything before this is older still
			return false
		}
		if (p.Unit != "" && e.Unit != p.Unit) || !matchGrep(p.Grep, e.Message) {
			return true
		}
		hits = append(hits, hit{e, start})
		return len(hits) <= p.Limit
	})
	if err != nil {
		return protocol.SyslogPage{}, rpcutil.FromOS(err)
	}
	if err := ctx.Err(); err != nil {
		return protocol.SyslogPage{}, err
	}
	page := protocol.SyslogPage{Items: []protocol.SyslogEntry{}}
	switch {
	case len(hits) > p.Limit:
		hits = hits[:p.Limit]
		page.Cursor = "off:" + strconv.FormatInt(hits[len(hits)-1].start, 10)
	case !reachedStart && lastStart >= 0:
		// The scan hit its byte limit: continue from where it stopped.
		page.Cursor = "off:" + strconv.FormatInt(lastStart, 10)
	}
	for i := len(hits) - 1; i >= 0; i-- {
		page.Items = append(page.Items, hits[i].e)
	}
	return page, nil
}

func (t *textLog) units(ctx context.Context) ([]string, error) {
	f, err := t.open()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, rpcutil.FromOS(err)
	}
	now, loc := time.Now(), time.Local
	since := now.Add(-7 * 24 * time.Hour)
	counts := map[string]int{}
	n := 0
	_, err = scanBack(f, st.Size(), textScanLimit, func(line string, _ int64) bool {
		e, ok := parseTextLine(line, now, loc)
		if !ok {
			return true
		}
		if e.Time.Before(since) {
			return false
		}
		counts[e.Unit]++
		n++
		return n < unitScan && ctx.Err() == nil
	})
	if err != nil {
		return nil, rpcutil.FromOS(err)
	}
	return countUnits(counts), nil
}

func (t *textLog) follow(ctx context.Context, p protocol.SyslogFollowParams, emit func([]protocol.SyslogEntry) error) error {
	if p.Priority != nil && *p.Priority < 6 {
		<-ctx.Done() // nothing has a level below 6 here
		return nil
	}
	f, err := t.open()
	if err != nil {
		return err
	}
	st, err := f.Stat()
	f.Close()
	if err != nil {
		return rpcutil.FromOS(err)
	}
	offset := st.Size()
	var partial string
	tick := time.NewTicker(textFollowEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		batch, next, rest, err := t.readNew(offset, partial, p)
		if err != nil {
			return err
		}
		offset, partial = next, rest
		if len(batch) > 0 {
			if err := emit(batch); err != nil {
				return err
			}
		}
	}
}

// readNew reads what was added to the file after offset. A file that got
// smaller was rotated and is read from the start.
func (t *textLog) readNew(offset int64, partial string, p protocol.SyslogFollowParams) (batch []protocol.SyslogEntry, next int64, rest string, err error) {
	f, err := t.open()
	if err != nil {
		return nil, offset, partial, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, offset, partial, rpcutil.FromOS(err)
	}
	if st.Size() < offset {
		offset, partial = 0, ""
	}
	if st.Size() == offset {
		return nil, offset, partial, nil
	}
	buf := make([]byte, min(st.Size()-offset, 4<<20))
	n, rerr := f.ReadAt(buf, offset)
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return nil, offset, partial, rpcutil.FromOS(rerr)
	}
	text := partial + string(buf[:n])
	next = offset + int64(n)
	cut := strings.LastIndexByte(text, '\n')
	if cut < 0 {
		return nil, next, text, nil
	}
	rest = text[cut+1:]
	now, loc := time.Now(), time.Local
	for _, line := range strings.Split(text[:cut], "\n") {
		e, ok := parseTextLine(line, now, loc)
		if !ok || (p.Unit != "" && e.Unit != p.Unit) || !matchGrep(p.Grep, e.Message) {
			continue
		}
		batch = append(batch, e)
	}
	if len(batch) > batchLines*5 {
		batch = batch[len(batch)-batchLines*5:]
	}
	return batch, next, rest, nil
}
