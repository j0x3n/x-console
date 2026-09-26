// Package netproxy serves http.proxy and ws.proxy: the server asks the agent
// to reach a service on the agent's own network (M9 Home Assistant). Only
// loopback, private, link-local and CGNAT (Tailscale) addresses are allowed.
// The check runs on the address actually dialed, after DNS resolution, so a
// public name that resolves to a private address works and a name that
// resolves to a public address does not.
package netproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// cgnat is 100.64.0.0/10, used by Tailscale and some ISPs.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// Allowed reports whether the agent may connect to ip.
func Allowed(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip)
}

// Proxy holds the HTTP machinery. The zero value is not usable; use New.
type Proxy struct {
	allow     func(net.IP) bool
	transport *http.Transport
}

// New builds a Proxy that only dials addresses accepted by Allowed.
func New() *Proxy { return newProxy(Allowed) }

func newProxy(allow func(net.IP) bool) *Proxy {
	p := &Proxy{allow: allow}
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: p.control}
	p.transport = &http.Transport{
		Proxy:                 nil, // never go through an environment proxy
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          8,
	}
	return p
}

var std = New()

// HTTP handles protocol.MethodHTTPProxy with the default Proxy.
func HTTP(ctx context.Context, raw json.RawMessage) (any, error) { return std.HTTP(ctx, raw) }

// WS handles protocol.MethodWSProxy with the default Proxy.
func WS(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error { return std.WS(ctx, raw, s) }

// control runs after DNS resolution for every connection.
func (p *Proxy) control(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !p.allow(ip) {
		return forbidden(host)
	}
	return nil
}

func forbidden(host string) error {
	return &protocol.Error{Code: protocol.CodeForbiddenTarget, Message: "代理只转发到内网地址，" + host + " 不是内网地址"}
}

func badParams(msg string) error {
	return &protocol.Error{Code: protocol.CodeBadParams, Message: msg}
}

// checkURL validates the scheme and rejects public IP literals early.
func (p *Proxy) checkURL(raw string, schemes ...string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, badParams("bad url: " + err.Error())
	}
	ok := false
	for _, s := range schemes {
		ok = ok || u.Scheme == s
	}
	if !ok || u.Hostname() == "" {
		return nil, badParams(fmt.Sprintf("url must start with %s", strings.Join(schemes, ":// or ")+"://"))
	}
	if u.User != nil {
		return nil, badParams("url must not contain credentials")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !p.allow(ip) {
		return nil, forbidden(u.Hostname())
	}
	return u, nil
}

// unwrap returns the protocol error hidden in a dial error, if any.
func unwrap(err error) error {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe
	}
	return &protocol.Error{Code: protocol.CodeFailed, Message: err.Error()}
}

// HTTP performs one request.
func (p *Proxy) HTTP(ctx context.Context, raw json.RawMessage) (any, error) {
	var in protocol.HTTPProxyParams
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, badParams(err.Error())
	}
	u, err := p.checkURL(in.URL, "http", "https")
	if err != nil {
		return nil, err
	}
	if in.Method == "" {
		in.Method = http.MethodGet
	}
	timeout := 15 * time.Second
	if in.TimeoutSeconds > 0 {
		timeout = time.Duration(min(in.TimeoutSeconds, 60)) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, in.Method, u.String(), bytes.NewReader(in.Body))
	if err != nil {
		return nil, badParams(err.Error())
	}
	for k, v := range in.Header {
		req.Header[http.CanonicalHeaderKey(k)] = v
	}
	client := &http.Client{Transport: p.transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		return nil, unwrap(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, protocol.HTTPProxyMaxBody+1))
	if err != nil {
		return nil, unwrap(err)
	}
	if len(body) > protocol.HTTPProxyMaxBody {
		return nil, &protocol.Error{Code: protocol.CodeFailed, Message: "response body too large"}
	}
	return protocol.HTTPProxyResult{Status: resp.StatusCode, Header: resp.Header, Body: body}, nil
}

// WS connects to the target and pumps messages until either side ends.
func (p *Proxy) WS(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
	var in protocol.WSProxyParams
	if err := json.Unmarshal(raw, &in); err != nil {
		return badParams(err.Error())
	}
	u, err := p.checkURL(in.URL, "ws", "wss")
	if err != nil {
		return err
	}
	dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	c, _, err := websocket.Dial(dctx, u.String(), &websocket.DialOptions{
		HTTPClient: &http.Client{Transport: p.transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
		HTTPHeader: in.Header,
	})
	cancel()
	if err != nil {
		return unwrap(err)
	}
	defer c.CloseNow()
	c.SetReadLimit(protocol.WSProxyMaxMessage)

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	upstream := make(chan error, 1)
	go func() {
		for {
			_, msg, err := c.Read(ctx)
			if err != nil {
				upstream <- err
				return
			}
			for _, frame := range protocol.WSProxySplit(msg) {
				if err := s.Send(ctx, frame); err != nil {
					upstream <- nil // the stream is gone; the other loop reports it
					return
				}
			}
		}
	}()
	down := make(chan error, 1)
	go func() {
		var j protocol.WSProxyJoiner
		for {
			frame, err := s.Recv(ctx)
			if err != nil {
				down <- err
				return
			}
			msg, done, err := j.Add(frame)
			if err != nil {
				down <- badParams(err.Error())
				return
			}
			if done {
				if err := c.Write(ctx, websocket.MessageText, msg); err != nil {
					down <- unwrap(err)
					return
				}
			}
		}
	}()
	select {
	case err := <-upstream:
		if err == nil {
			return nil
		}
		return &protocol.Error{Code: protocol.CodeFailed, Message: "target closed the connection: " + err.Error()}
	case err := <-down:
		if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
			_ = c.Close(websocket.StatusNormalClosure, "")
			return nil
		}
		return err
	}
}
