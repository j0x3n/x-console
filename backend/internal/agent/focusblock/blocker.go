package focusblock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const (
	defaultTick       = 5 * time.Second  // how often the program list is checked
	defaultHostsCheck = 30 * time.Second // how often the hosts block is checked
	defaultRetryClean = 30 * time.Second // how often a failed clean-up is tried again
	maxLastKilled     = 10
	truncatedName     = 15 // Linux cuts process names to 15 characters
)

// Blocker holds the one block that may be active on this computer.
type Blocker struct {
	HostsPath string
	StatePath string
	Procs     Procs
	Now       func() time.Time
	// Tick, HostsCheck and RetryClean are the check intervals. Tests shorten them.
	Tick       time.Duration
	HostsCheck time.Duration
	RetryClean time.Duration
	FlushDNS   func()
	// Reserved are host names that are never blocked, such as the address of
	// the panel the agent talks to. Blocking it would cut the agent off.
	Reserved []string

	root  context.Context
	close context.CancelFunc

	mu       sync.Mutex
	cur      *run
	cleanErr error // the last failed attempt to take the block out of the hosts file
	retrying bool
}

// run is one active block.
type run struct {
	sessionID int64
	domains   []string
	names     []string
	apps      map[string]bool
	ipv6      bool
	deadline  time.Time
	refused   []string
	cancel    context.CancelFunc

	hosts        string
	hostsErr     string
	checkedHosts time.Time
	killed       int
	last         []string
	asked        map[int32]bool
}

// saved is what survives an agent restart.
type saved struct {
	SessionID int64     `json:"sessionId"`
	Deadline  time.Time `json:"deadline"`
	Domains   []string  `json:"domains,omitempty"`
	Apps      []string  `json:"apps,omitempty"`
	IPv6      bool      `json:"ipv6,omitempty"`
	// Cleanup marks a block that is over but could not be taken out of the
	// hosts file yet.
	Cleanup bool `json:"cleanup,omitempty"`
}

// New builds a blocker for a hosts file and a state file.
func New(hostsPath, statePath string) *Blocker {
	root, cancel := context.WithCancel(context.Background())
	return &Blocker{HostsPath: hostsPath, StatePath: statePath, Procs: systemProcs{}, Now: time.Now,
		Tick: defaultTick, HostsCheck: defaultHostsCheck, RetryClean: defaultRetryClean, FlushDNS: flushDNS,
		root: root, close: cancel}
}

// Close stops the background work. It leaves the hosts file as it is.
func (b *Blocker) Close() { b.close() }

func badEntry(kind, entry string, err error) string {
	return fmt.Sprintf("%s (%s: %v)", entry, kind, err)
}

// Apply starts a block, or replaces the active one.
func (b *Blocker) Apply(p protocol.FocusBlockApplyParams) (protocol.FocusBlockStatus, error) {
	if p.TTLSeconds < 1 || p.TTLSeconds > protocol.FocusBlockMaxTTLSeconds {
		return protocol.FocusBlockStatus{}, rpcutil.BadParams("ttlSeconds must be between 1 and %d", protocol.FocusBlockMaxTTLSeconds)
	}
	if len(p.Domains) > protocol.FocusBlockMaxDomains || len(p.Apps) > protocol.FocusBlockMaxApps {
		return protocol.FocusBlockStatus{}, rpcutil.BadParams("too many entries")
	}
	r := &run{sessionID: p.SessionID, ipv6: p.IPv6, apps: map[string]bool{}, asked: map[int32]bool{}}
	seenName := map[string]bool{}
	for _, d := range p.Domains {
		domain, err := protocol.NormalizeBlockDomain(d)
		if err != nil {
			r.refused = append(r.refused, badEntry("domain", d, err))
			continue
		}
		names := protocol.BlockHostNames(domain)
		if b.reserved(names) {
			r.refused = append(r.refused, badEntry("domain", d, errors.New("address of the panel")))
			continue
		}
		r.domains = append(r.domains, domain)
		for _, n := range names {
			if !seenName[n] {
				seenName[n] = true
				r.names = append(r.names, n)
			}
		}
	}
	for _, a := range p.Apps {
		app, err := protocol.NormalizeBlockApp(a)
		if err == nil && protocol.IsProtectedProcess(app) {
			err = protocol.ErrBlockProtected
		}
		if err != nil {
			r.refused = append(r.refused, badEntry("program", a, err))
			continue
		}
		r.apps[app] = true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(r.names) == 0 && len(r.apps) == 0 {
		st := b.clearLocked()
		st.Refused = r.refused
		return st, nil
	}
	r.deadline = b.Now().Add(time.Duration(p.TTLSeconds) * time.Second)
	b.startLocked(r)
	return b.statusLocked(), nil
}

func (b *Blocker) reserved(names []string) bool {
	for _, n := range names {
		for _, h := range b.Reserved {
			if strings.EqualFold(n, h) {
				return true
			}
		}
	}
	return false
}

// startLocked makes r the active block.
func (b *Blocker) startLocked(r *run) {
	if b.cur != nil {
		b.cur.cancel()
	}
	var ctx context.Context
	ctx, r.cancel = context.WithCancel(b.root)
	b.cur = r
	b.syncHostsLocked(r)
	b.saveStateLocked(saved{SessionID: r.sessionID, Deadline: r.deadline, Domains: r.domains, Apps: r.appList(), IPv6: r.ipv6})
	b.killPassLocked(ctx, r)
	go b.loop(ctx, r)
}

func (r *run) appList() []string {
	out := make([]string, 0, len(r.apps))
	for a := range r.apps {
		out = append(out, a)
	}
	return out
}

// Clear ends the block and restores the hosts file.
func (b *Blocker) Clear() protocol.FocusBlockStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.clearLocked()
}

func (b *Blocker) clearLocked() protocol.FocusBlockStatus {
	if b.cur != nil {
		b.cur.cancel()
		b.cur = nil
	}
	b.cleanLocked()
	return b.statusLocked()
}

// cleanLocked takes the block out of the hosts file. When that fails the
// state file says so and a retry runs, so the block never stays for good.
func (b *Blocker) cleanLocked() {
	if err := b.removeHostsLocked(); err != nil {
		slog.Warn("focus block: cannot restore the hosts file", "err", err)
		b.saveStateLocked(saved{Cleanup: true})
		b.startRetryLocked()
		return
	}
	b.removeStateLocked()
}

func (b *Blocker) removeHostsLocked() error {
	wrote, err := syncHosts(b.HostsPath, nil, false)
	b.cleanErr = err
	if wrote && err == nil {
		b.FlushDNS()
	}
	return err
}

func (b *Blocker) syncHostsLocked(r *run) {
	wrote, err := syncHosts(b.HostsPath, r.names, r.ipv6)
	r.checkedHosts = time.Now()
	r.hosts = hostsResult(err)
	r.hostsErr = ""
	if err != nil {
		r.hostsErr = err.Error()
		slog.Warn("focus block: cannot write the hosts file", "err", err)
	} else if len(r.names) == 0 {
		r.hosts = protocol.FocusBlockHostsNone
	}
	if wrote && err == nil {
		b.FlushDNS()
	}
}

func (b *Blocker) startRetryLocked() {
	if b.retrying {
		return
	}
	b.retrying = true
	go func() {
		t := time.NewTicker(b.RetryClean)
		defer t.Stop()
		for {
			select {
			case <-b.root.Done():
				return
			case <-t.C:
			}
			b.mu.Lock()
			if b.cur != nil { // a new block took over the hosts file
				b.retrying = false
				b.mu.Unlock()
				return
			}
			if err := b.removeHostsLocked(); err == nil {
				b.removeStateLocked()
				b.retrying = false
				b.mu.Unlock()
				slog.Info("focus block: the hosts file is restored")
				return
			}
			b.mu.Unlock()
		}
	}()
}

// Status says what the agent is doing now.
func (b *Blocker) Status() protocol.FocusBlockStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.statusLocked()
}

func (b *Blocker) statusLocked() protocol.FocusBlockStatus {
	r := b.cur
	if r == nil {
		st := protocol.FocusBlockStatus{Hosts: hostsResult(b.cleanErr)}
		if b.cleanErr != nil {
			st.HostsError = b.cleanErr.Error()
		}
		return st
	}
	left := int(r.deadline.Sub(b.Now()) / time.Second)
	return protocol.FocusBlockStatus{Active: true, SessionID: r.sessionID, RemainingSeconds: max(left, 0),
		Hosts: r.hosts, HostsError: r.hostsErr, Sites: len(r.names), Killed: r.killed,
		LastKilled: append([]string(nil), r.last...), Refused: r.refused}
}

// loop ends programs that were started again, keeps the hosts block in place
// and restores everything when the deadline passes, whether or not the
// server is still there.
func (b *Blocker) loop(ctx context.Context, r *run) {
	t := time.NewTicker(b.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		b.mu.Lock()
		if b.cur != r {
			b.mu.Unlock()
			return
		}
		now := b.Now()
		if !now.Before(r.deadline) {
			slog.Info("focus block: deadline reached, restoring", "session", r.sessionID)
			b.clearLocked()
			b.mu.Unlock()
			return
		}
		b.killPassLocked(ctx, r)
		if len(r.names) > 0 && time.Since(r.checkedHosts) >= b.HostsCheck {
			b.syncHostsLocked(r)
		}
		b.mu.Unlock()
	}
}

// matches reports whether a process name is one of the listed programs. The
// name has to be the same; a path or a part of a name never matches.
func matches(apps map[string]bool, name string) bool {
	n, err := protocol.NormalizeBlockApp(name)
	if err != nil {
		return false
	}
	if apps[n] {
		return true
	}
	if len(n) == truncatedName { // Linux shows only the first 15 characters
		for a := range apps {
			if strings.HasPrefix(a, n) {
				return true
			}
		}
	}
	return false
}

// killPassLocked ends every listed program that is running now.
func (b *Blocker) killPassLocked(ctx context.Context, r *run) {
	if len(r.apps) == 0 {
		return
	}
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	procs, err := b.Procs.List(lctx)
	if err != nil {
		slog.Warn("focus block: cannot list programs", "err", err)
		return
	}
	self, parent := int32(os.Getpid()), int32(os.Getppid())
	alive := map[int32]bool{}
	for _, p := range procs {
		if !matches(r.apps, p.Name) {
			continue
		}
		// The list was checked on the way in. This is the last guard.
		if protocol.IsProtectedProcess(p.Name) || p.PID == self || p.PID == parent || p.PID <= 4 {
			continue
		}
		alive[p.PID] = true
		force := r.asked[p.PID]
		if err := b.Procs.End(lctx, p.PID, force); err != nil {
			slog.Warn("focus block: cannot end a program", "name", p.Name, "pid", p.PID, "err", err)
			continue
		}
		if !force {
			r.asked[p.PID] = true
			r.killed++
			r.last = append(r.last, p.Name)
			if len(r.last) > maxLastKilled {
				r.last = r.last[len(r.last)-maxLastKilled:]
			}
		}
	}
	for pid := range r.asked {
		if !alive[pid] {
			delete(r.asked, pid)
		}
	}
}

// ---- state file ----

func (b *Blocker) saveStateLocked(s saved) {
	if b.StatePath == "" {
		return
	}
	data, err := json.Marshal(s)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(b.StatePath), 0o700)
	}
	if err == nil {
		err = writeAtomic(b.StatePath, data)
	}
	if err != nil {
		// Without the file a restart cleans the hosts file, which is the safe side.
		slog.Warn("focus block: cannot save the state", "err", err)
	}
}

func (b *Blocker) removeStateLocked() {
	if b.StatePath != "" {
		_ = os.Remove(b.StatePath)
	}
}

func (b *Blocker) loadState() (saved, bool) {
	if b.StatePath == "" {
		return saved{}, false
	}
	data, err := os.ReadFile(b.StatePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("focus block: cannot read the state", "err", err)
		}
		return saved{}, false
	}
	var s saved
	if json.Unmarshal(data, &s) != nil {
		return saved{}, false
	}
	return s, true
}

// Recover runs once when the agent starts. A block that is still within its
// deadline carries on. Anything else left in the hosts file is removed: the
// agent was stopped or crashed while a block was active, and nobody knows
// whether the focus session is still running. The server sends the current
// state when the agent connects, so a block that should go on is set again.
func (b *Blocker) Recover() {
	st, ok := b.loadState()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur != nil {
		return
	}
	if ok && !st.Cleanup && st.Deadline.After(b.Now()) {
		r := &run{sessionID: st.SessionID, ipv6: st.IPv6, deadline: st.Deadline, apps: map[string]bool{}, asked: map[int32]bool{}}
		seen := map[string]bool{}
		for _, d := range st.Domains {
			domain, err := protocol.NormalizeBlockDomain(d)
			if err != nil || b.reserved(protocol.BlockHostNames(domain)) {
				continue
			}
			r.domains = append(r.domains, domain)
			for _, n := range protocol.BlockHostNames(domain) {
				if !seen[n] {
					seen[n] = true
					r.names = append(r.names, n)
				}
			}
		}
		for _, a := range st.Apps {
			if app, err := protocol.NormalizeBlockApp(a); err == nil && !protocol.IsProtectedProcess(app) {
				r.apps[app] = true
			}
		}
		if len(r.names) > 0 || len(r.apps) > 0 {
			slog.Info("focus block: carrying on after a restart", "session", r.sessionID)
			b.startLocked(r)
			return
		}
	}
	b.cleanLocked()
}
