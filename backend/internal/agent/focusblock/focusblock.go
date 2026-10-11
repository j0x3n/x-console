// Package focusblock blocks distracting sites and programs while the panel's
// pomodoro runs (B131). Sites are pointed to this computer in the hosts file,
// inside one marked block. Listed programs are ended, and ended again when
// they start again. Everything is restored when the server says so, and by
// the agent itself at a deadline the server gave it. See docs/specs/B131.md.
package focusblock

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/j0x3n/x-console/backend/internal/agent/config"
	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// Registrar is the part of conn.Client the handlers need.
type Registrar interface {
	Handle(method string, h rpc.Handler)
}

// stateDir holds focusblock-state.json. cmd/agent sets it to the directory of
// the agent's own config file.
var stateDir = filepath.Dir(config.DefaultPath())

// SetStateDir moves the state file next to the agent's config file.
func SetStateDir(dir string) { stateDir = dir }

// Available reports whether this system can do it. The hosts file and the
// process list exist on Windows and Linux.
func Available() bool { return runtime.GOOS == "windows" || runtime.GOOS == "linux" }

// Register adds focusblock.apply, focusblock.clear and focusblock.status, and
// takes care of a block left over from before the agent started. serverURL is
// the panel's address, which is never blocked.
func Register(c Registrar, serverURL string) {
	if !Available() {
		return
	}
	b := New(DefaultHostsPath(), filepath.Join(stateDir, "focusblock-state.json"))
	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		b.Reserved = []string{u.Hostname()}
	}
	b.Recover()
	Handlers(c, b)
}

// Handlers registers the methods for b.
func Handlers(c Registrar, b *Blocker) {
	c.Handle(protocol.MethodFocusBlockApply, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.FocusBlockApplyParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		st, err := b.Apply(p)
		if err == nil {
			slog.Info("focus block: applied", "session", p.SessionID, "hosts", st.Hosts, "sites", st.Sites)
		}
		return st, err
	})
	c.Handle(protocol.MethodFocusBlockClear, func(context.Context, json.RawMessage) (any, error) {
		return b.Clear(), nil
	})
	c.Handle(protocol.MethodFocusBlockStatus, func(context.Context, json.RawMessage) (any, error) {
		return b.Status(), nil
	})
}
