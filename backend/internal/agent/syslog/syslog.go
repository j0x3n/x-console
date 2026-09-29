// Package syslog reads system logs for the agent: the systemd journal, plain
// syslog files or the Windows event log. See docs/specs/B29.md.
package syslog

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	defaultLimit = 500
	maxLimit     = 5000
	maxGrep      = 200
	maxUnits     = 300
	// maxMessage is the longest message sent. Longer ones are cut and end
	// with an ellipsis.
	maxMessage = 16 << 10
	// batchLines and batchEvery bound how much a follow stream collects
	// before it sends a frame.
	batchLines = 200
	batchEvery = 200 * time.Millisecond
)

// backend is one way to read logs.
type backend interface {
	query(ctx context.Context, p protocol.SyslogQueryParams) (protocol.SyslogPage, error)
	units(ctx context.Context) ([]string, error)
	// follow calls emit with batches of new entries until ctx ends.
	follow(ctx context.Context, p protocol.SyslogFollowParams, emit func([]protocol.SyslogEntry) error) error
}

// Available reports whether this system has logs the agent can read. cmd/agent
// announces protocol.CapSyslog only then.
func Available() bool { return detect() != nil }

// Registrar is the part of conn.Client the handlers need.
type Registrar interface {
	Handle(method string, h rpc.Handler)
	HandleStream(method string, h rpc.StreamHandler)
}

// Register adds syslog.query, syslog.units and syslog.follow.
func Register(c Registrar) {
	if b := detect(); b != nil {
		RegisterBackend(c, b)
	}
}

// RegisterBackend is Register with a chosen backend, for tests.
func RegisterBackend(c Registrar, b backend) {
	c.Handle(protocol.MethodSyslogQuery, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SyslogQueryParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		if err := normalize(&p); err != nil {
			return nil, err
		}
		return b.query(ctx, p)
	})
	c.Handle(protocol.MethodSyslogUnits, func(ctx context.Context, _ json.RawMessage) (any, error) {
		items, err := b.units(ctx)
		if err != nil {
			return nil, err
		}
		if items == nil {
			items = []string{}
		}
		return protocol.SyslogUnits{Items: items[:min(len(items), maxUnits)]}, nil
	})
	c.HandleStream(protocol.MethodSyslogFollow, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		var p protocol.SyslogFollowParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return err
		}
		q := protocol.SyslogQueryParams{Priority: p.Priority, Unit: p.Unit, Grep: p.Grep}
		if err := normalize(&q); err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			select {
			case <-s.Context().Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		err := b.follow(ctx, protocol.SyslogFollowParams{Priority: q.Priority, Unit: q.Unit, Grep: q.Grep}, func(batch []protocol.SyslogEntry) error {
			raw, err := json.Marshal(batch)
			if err != nil {
				return err
			}
			return s.Send(ctx, raw)
		})
		if ctx.Err() != nil {
			return nil // the reader went away
		}
		return err
	})
}

// unitRe is what a unit or program name may look like. It has no space, quote
// or option prefix, so it is safe as a command line argument.
var unitRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._@:-]{0,127}$`)

// normalize checks the parameters and fills in defaults.
func normalize(p *protocol.SyslogQueryParams) error {
	if p.Limit <= 0 {
		p.Limit = defaultLimit
	}
	p.Limit = min(p.Limit, maxLimit)
	if p.Priority != nil && (*p.Priority < 0 || *p.Priority > 7) {
		return rpcutil.BadParams("priority must be between 0 and 7")
	}
	if len(p.Grep) > maxGrep {
		return rpcutil.BadParams("grep is longer than %d bytes", maxGrep)
	}
	if strings.ContainsAny(p.Grep, "\x00\r\n") {
		return rpcutil.BadParams("grep has a control character")
	}
	if len(p.Cursor) > 500 {
		return rpcutil.BadParams("invalid cursor")
	}
	return nil
}

// clipMessage cuts a long message at a character border.
func clipMessage(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= maxMessage {
		return s
	}
	cut := maxMessage
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// matchGrep is the case-insensitive keyword test for backends that filter
// by themselves.
func matchGrep(grep, message string) bool {
	return grep == "" || strings.Contains(strings.ToLower(message), strings.ToLower(grep))
}

// batchEmit sends entries from ch in batches: every batchEvery, or as soon as
// batchLines are waiting. It returns when ch closes or ctx ends.
func batchEmit(ctx context.Context, ch <-chan protocol.SyslogEntry, emit func([]protocol.SyslogEntry) error) error {
	tick := time.NewTicker(batchEvery)
	defer tick.Stop()
	var batch []protocol.SyslogEntry
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		out := batch
		batch = nil
		return emit(out)
	}
	for {
		select {
		case <-ctx.Done():
			return flush()
		case e, ok := <-ch:
			if !ok {
				return flush()
			}
			batch = append(batch, e)
			if len(batch) >= batchLines {
				if err := flush(); err != nil {
					return err
				}
			}
		case <-tick.C:
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

// countUnits turns counted names into a list, most entries first.
func countUnits(counts map[string]int) []string {
	names := make([]string, 0, len(counts))
	for name := range counts {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if counts[names[i]] != counts[names[j]] {
			return counts[names[i]] > counts[names[j]]
		}
		return names[i] < names[j]
	})
	return names[:min(len(names), maxUnits)]
}
