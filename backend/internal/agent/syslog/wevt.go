package syslog

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// wevtLogs are the event logs that count as the system log.
var wevtLogs = []string{"System", "Application"}

const (
	wevtPermissionMsg = "代理没有读取事件日志的权限。请用管理员身份运行代理，或者把代理的用户加进 Event Log Readers 组。"
	wevtTime          = "2006-01-02T15:04:05.000Z"
	wevtPreciseTime   = "2006-01-02T15:04:05.0000000Z"
	wevtFollowEvery   = 2 * time.Second
	wevtUnitScan      = 2000
)

// wevt reads the Windows event log through wevtutil. run is wevtutil, so
// tests can replace it.
type wevt struct {
	run func(ctx context.Context, args ...string) ([]byte, error)
}

func newWevt() *wevt { return &wevt{run: execWevtutil} }

func execWevtutil(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "wevtutil", args...)
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil && ctx.Err() == nil {
		msg := stderr.String()
		if strings.Contains(msg, "Access is denied") || strings.Contains(msg, "0x5") {
			return nil, &protocol.Error{Code: protocol.CodeSyslogPermission, Message: wevtPermissionMsg}
		}
		return nil, rpcutil.Failed("wevtutil: %s", strings.TrimSpace(msg+" "+err.Error()))
	}
	return out, err
}

// levelForPriority maps the journal level to the highest Windows Level that
// still counts: 1 Critical, 2 Error, 3 Warning, 4 Information, 5 Verbose.
func levelForPriority(priority int) int {
	switch {
	case priority <= 2:
		return 1
	case priority == 3:
		return 2
	case priority == 4:
		return 3
	case priority <= 6:
		return 4
	}
	return 5
}

// priorityForLevel maps a Windows Level to the journal scale.
func priorityForLevel(level int) int {
	switch level {
	case 1:
		return 2
	case 2:
		return 3
	case 3:
		return 4
	case 5:
		return 7
	}
	return 6 // Information, and LogAlways (0)
}

// sourceOK allows the characters of provider names and nothing that could
// end the quoted value in an XPath query.
func sourceOK(s string) bool {
	return s != "" && len(s) <= 128 && !strings.ContainsAny(s, "'\"<>&\x00\r\n\\") && !strings.HasPrefix(s, "-")
}

// buildXPath makes the /q: query. after is the largest record ID already
// seen (0 for none) and before an exclusive upper time for paging.
func buildXPath(since, until *time.Time, before time.Time, priority *int, source string, afterRecord int64) (string, error) {
	var conds []string
	if priority != nil && *priority < 7 {
		conds = append(conds, "Level<="+strconv.Itoa(levelForPriority(*priority)))
	}
	if since != nil {
		conds = append(conds, "TimeCreated[@SystemTime>='"+since.UTC().Format(wevtTime)+"']")
	}
	if until != nil {
		conds = append(conds, "TimeCreated[@SystemTime<='"+until.UTC().Format(wevtTime)+"']")
	}
	if !before.IsZero() {
		conds = append(conds, "TimeCreated[@SystemTime<'"+before.UTC().Format(wevtPreciseTime)+"']")
	}
	if source != "" {
		if !sourceOK(source) {
			return "", rpcutil.BadParams("invalid source %q", source)
		}
		conds = append(conds, "Provider[@Name='"+source+"']")
	}
	if afterRecord > 0 {
		conds = append(conds, "EventRecordID>"+strconv.FormatInt(afterRecord, 10))
	}
	if len(conds) == 0 {
		return "*", nil
	}
	return "*[System[" + strings.Join(conds, " and ") + "]]", nil
}

type wevtEvent struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     int `xml:"EventID"`
		Level       int `xml:"Level"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		EventRecordID int64 `xml:"EventRecordID"`
		Execution     struct {
			ProcessID int `xml:"ProcessID,attr"`
		} `xml:"Execution"`
	} `xml:"System"`
	EventData struct {
		Data []string `xml:"Data"`
	} `xml:"EventData"`
	RenderingInfo struct {
		Message string `xml:"Message"`
	} `xml:"RenderingInfo"`
}

type wevtRow struct {
	entry  protocol.SyslogEntry
	record int64
}

// parseEvents reads the output of wevtutil /f:RenderedXml: Event elements
// one after the other without a root.
func parseEvents(out []byte) ([]wevtRow, error) {
	dec := xml.NewDecoder(bytes.NewReader(out))
	var rows []wevtRow
	for {
		var ev wevtEvent
		err := dec.Decode(&ev)
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		ts, err := time.Parse(time.RFC3339Nano, ev.System.TimeCreated.SystemTime)
		if err != nil {
			continue
		}
		msg := strings.TrimSpace(ev.RenderingInfo.Message)
		if msg == "" {
			var parts []string
			for _, d := range ev.EventData.Data {
				if d = strings.TrimSpace(d); d != "" {
					parts = append(parts, d)
				}
			}
			msg = strings.Join(parts, " ")
		}
		if msg == "" {
			msg = "事件 " + strconv.Itoa(ev.System.EventID)
		}
		rows = append(rows, wevtRow{record: ev.System.EventRecordID, entry: protocol.SyslogEntry{
			Time: ts.UTC(), Priority: priorityForLevel(ev.System.Level), Unit: ev.System.Provider.Name,
			PID: ev.System.Execution.ProcessID, Message: clipMessage(msg)}})
	}
}

// readLog runs one wevtutil query. newestFirst is true for paging back.
func (w *wevt) readLog(ctx context.Context, log, xpath string, count int, newestFirst bool) ([]wevtRow, error) {
	args := []string{"qe", log, "/f:RenderedXml", "/c:" + strconv.Itoa(count), "/rd:" + strconv.FormatBool(newestFirst), "/q:" + xpath}
	out, err := w.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseEvents(toUTF8(out, outputCodePage()))
}

func (w *wevt) query(ctx context.Context, p protocol.SyslogQueryParams) (protocol.SyslogPage, error) {
	var before time.Time
	if p.Cursor != "" {
		t, ok := strings.CutPrefix(p.Cursor, "t:")
		ts, err := time.Parse(time.RFC3339Nano, t)
		if !ok || err != nil {
			return protocol.SyslogPage{}, rpcutil.BadParams("invalid cursor")
		}
		before = ts
	}
	xpath, err := buildXPath(p.Since, p.Until, before, p.Priority, p.Unit, 0)
	if err != nil {
		return protocol.SyslogPage{}, err
	}
	want := p.Limit + 1
	fetch := want
	if p.Grep != "" {
		fetch = min(want*grepScan, 5000)
	}
	var all []wevtRow
	full := false
	for _, log := range wevtLogs {
		rows, err := w.readLog(ctx, log, xpath, fetch, true)
		if err != nil {
			return protocol.SyslogPage{}, err
		}
		full = full || len(rows) >= fetch
		all = append(all, rows...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].entry.Time.After(all[j].entry.Time) })
	kept := all[:0]
	for _, r := range all {
		if matchGrep(p.Grep, r.entry.Message) {
			kept = append(kept, r)
		}
	}
	page := protocol.SyslogPage{Items: []protocol.SyslogEntry{}}
	switch {
	case len(kept) > p.Limit:
		kept = kept[:p.Limit]
		page.Cursor = "t:" + kept[len(kept)-1].entry.Time.Format(time.RFC3339Nano)
	case full && len(kept) > 0:
		page.Cursor = "t:" + kept[len(kept)-1].entry.Time.Format(time.RFC3339Nano)
	}
	for i := len(kept) - 1; i >= 0; i-- {
		page.Items = append(page.Items, kept[i].entry)
	}
	return page, nil
}

func (w *wevt) units(ctx context.Context) ([]string, error) {
	since := time.Now().Add(-7 * 24 * time.Hour)
	xpath, _ := buildXPath(&since, nil, time.Time{}, nil, "", 0)
	counts := map[string]int{}
	for _, log := range wevtLogs {
		rows, err := w.readLog(ctx, log, xpath, wevtUnitScan, true)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			counts[r.entry.Unit]++
		}
	}
	return countUnits(counts), nil
}

func (w *wevt) follow(ctx context.Context, p protocol.SyslogFollowParams, emit func([]protocol.SyslogEntry) error) error {
	// Start after the newest record of each log.
	last := map[string]int64{}
	for _, log := range wevtLogs {
		rows, err := w.readLog(ctx, log, "*", 1, true)
		if err != nil {
			return err
		}
		if len(rows) > 0 {
			last[log] = rows[0].record
		}
	}
	tick := time.NewTicker(wevtFollowEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		var batch []wevtRow
		for _, log := range wevtLogs {
			xpath, err := buildXPath(nil, nil, time.Time{}, p.Priority, p.Unit, last[log])
			if err != nil {
				return err
			}
			rows, err := w.readLog(ctx, log, xpath, 500, false)
			if err != nil {
				return err
			}
			for _, r := range rows {
				last[log] = max(last[log], r.record)
				if matchGrep(p.Grep, r.entry.Message) {
					batch = append(batch, r)
				}
			}
		}
		if len(batch) == 0 {
			continue
		}
		sort.SliceStable(batch, func(i, j int) bool { return batch[i].entry.Time.Before(batch[j].entry.Time) })
		out := make([]protocol.SyslogEntry, len(batch))
		for i, r := range batch {
			out[i] = r.entry
		}
		for len(out) > 0 {
			n := min(len(out), batchLines)
			if err := emit(out[:n]); err != nil {
				return err
			}
			out = out[n:]
		}
	}
}
