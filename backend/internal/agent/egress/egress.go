// Package egress serves net.dial (B208): the server's headless browser
// reaches the web through this machine. Only public addresses are allowed.
// The check runs on the address actually dialed, after DNS resolution, so a
// name that resolves to the machine's own network is refused and a page
// cannot use the agent to reach devices on the user's network.
package egress

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

const (
	dialTimeout = 10 * time.Second
	// chunk is how much of a connection goes into one data frame.
	chunk = 32 << 10
)

// blocked are ranges that IsGlobalUnicast accepts but that are not the
// public internet.
var blocked = mustCIDRs(
	"0.0.0.0/8",       // "this network"
	"100.64.0.0/10",   // CGNAT, Tailscale
	"192.0.0.0/24",    // IETF protocol assignments
	"192.0.2.0/24",    // documentation
	"198.18.0.0/15",   // benchmarking
	"198.51.100.0/24", // documentation
	"203.0.113.0/24",  // documentation
	"240.0.0.0/4",     // reserved
	"2001:db8::/32",   // documentation
)

func mustCIDRs(list ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(list))
	for _, s := range list {
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// Allowed reports whether the agent may connect to ip: a public unicast
// address. Loopback, private, link-local, CGNAT, multicast, unspecified and
// reserved ranges are refused.
func Allowed(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4 // also covers ::ffff:10.0.0.1
	}
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, n := range blocked {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

// Dialer connects streams to targets. The zero value is not usable; use New.
type Dialer struct {
	allow func(net.IP) bool
}

// New builds a Dialer that only connects to addresses accepted by allow.
// Production code passes Allowed; tests pass a function that also accepts
// 127.0.0.1.
func New(allow func(net.IP) bool) *Dialer { return &Dialer{allow: allow} }

var std = New(Allowed)

// Handle serves protocol.MethodNetDial with the default Dialer.
func Handle(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	return std.Handle(ctx, raw, s)
}

// Disabled answers protocol.MethodNetDial on an agent whose config turned
// egress off.
func Disabled(context.Context, json.RawMessage, *rpc.Stream) error {
	return &protocol.Error{Code: protocol.CodeUnsupported, Message: "这台机器的代理关掉了出口功能"}
}

// Register wires net.dial into the client. Egress is on unless disabled; the
// capability is only announced when it is on, and a server that tries anyway
// is refused.
func Register(c *conn.Client, disabled bool) {
	if disabled {
		c.HandleStream(protocol.MethodNetDial, Disabled)
		return
	}
	for _, v := range c.Hello.Capabilities {
		if v == protocol.CapEgress {
			c.HandleStream(protocol.MethodNetDial, Handle)
			return
		}
	}
	c.Hello.Capabilities = append(c.Hello.Capabilities, protocol.CapEgress)
	c.HandleStream(protocol.MethodNetDial, Handle)
}

// control runs after DNS resolution for every connection attempt.
func (d *Dialer) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !d.allow(ip) {
		return forbidden(host)
	}
	return nil
}

func forbidden(host string) error {
	return &protocol.Error{Code: protocol.CodeForbiddenTarget, Message: "出口只连公网地址，" + host + " 不是公网地址"}
}

// Handle connects to the target in raw and copies bytes both ways until
// either side ends.
func (d *Dialer) Handle(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var in protocol.NetDialParams
	if err := json.Unmarshal(raw, &in); err != nil {
		return &protocol.Error{Code: protocol.CodeBadParams, Message: err.Error()}
	}
	if in.Host == "" || in.Port < 1 || in.Port > 65535 {
		return &protocol.Error{Code: protocol.CodeBadParams, Message: "host or port is missing"}
	}
	dialer := &net.Dialer{Timeout: dialTimeout, Control: d.control}
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	c, err := dialer.DialContext(dctx, "tcp", net.JoinHostPort(in.Host, strconv.Itoa(in.Port)))
	cancel()
	if err != nil {
		return unwrap(err)
	}
	defer c.Close()
	// The server waits for this empty frame before it tells the browser the
	// connection is made.
	if err := s.Send(ctx, nil); err != nil {
		return nil
	}
	return pump(ctx, c, s)
}

// unwrap returns the protocol error hidden in a dial error, if any.
func unwrap(err error) error {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe
	}
	return &protocol.Error{Code: protocol.CodeFailed, Message: err.Error()}
}

// pump copies between the connection and the stream. It returns when either
// direction ends.
func pump(ctx context.Context, c net.Conn, s *rpc.Stream) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = c.Close() })
	defer stop()
	up := make(chan error, 1)
	go func() { // target -> server
		buf := make([]byte, chunk)
		for {
			n, err := c.Read(buf)
			if n > 0 {
				if serr := s.Send(ctx, append([]byte(nil), buf[:n]...)); serr != nil {
					up <- nil // the stream is gone; the other direction reports it
					return
				}
			}
			if err != nil {
				up <- err
				return
			}
		}
	}()
	down := make(chan error, 1)
	go func() { // server -> target
		for {
			b, err := s.Recv(ctx)
			if err != nil {
				down <- err
				return
			}
			if len(b) == 0 {
				continue
			}
			if _, err := c.Write(b); err != nil {
				down <- err
				return
			}
		}
	}()
	select {
	case err := <-up:
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return &protocol.Error{Code: protocol.CodeFailed, Message: err.Error()}
	case err := <-down:
		if err == nil || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}
