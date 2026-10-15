package egress

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

func TestAllowed(t *testing.T) {
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "114.114.114.114", "100.128.0.1", "172.32.0.1", "2001:4860:4860::8888", "::ffff:8.8.8.8"} {
		if !Allowed(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
	for _, ip := range []string{
		"127.0.0.1", "127.9.9.9", "::1", // loopback
		"10.1.2.3", "172.16.0.9", "192.168.1.20", "fd00::5", // private
		"169.254.1.1", "fe80::1", // link-local
		"100.64.0.1", "100.101.102.103", "100.127.255.255", // CGNAT
		"224.0.0.1", "239.255.255.250", "ff02::1", // multicast
		"0.0.0.0", "::", "0.1.2.3", // unspecified
		"255.255.255.255", "240.0.0.1", "192.0.2.1", "198.18.0.1",
		"::ffff:10.0.0.1", "::ffff:127.0.0.1", // v4 inside v6
	} {
		if Allowed(net.ParseIP(ip)) {
			t.Errorf("%s should be rejected", ip)
		}
	}
	if Allowed(nil) {
		t.Error("nil should be rejected")
	}
}

// open runs d.Handle on one end of an in-memory connection and returns the
// server's stream.
func open(t *testing.T, d *Dialer, params any) (*rpc.Stream, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	a, b := rpc.Pipe()
	server, agent := rpc.NewPeer(a, "s"), rpc.NewPeer(b, "a")
	agent.HandleStream(protocol.MethodNetDial, d.Handle)
	go server.Run(ctx)
	go agent.Run(ctx)
	s, err := server.Open(ctx, protocol.MethodNetDial, params)
	if err != nil {
		t.Fatal(err)
	}
	return s, ctx
}

func code(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func listen(t *testing.T) (host string, port int, accepted chan net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	accepted = make(chan net.Conn, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted <- c
		}
	}()
	h, p, _ := net.SplitHostPort(ln.Addr().String())
	n, _ := strconv.Atoi(p)
	return h, n, accepted
}

func TestRejectsLocalTargets(t *testing.T) {
	host, port, accepted := listen(t)
	d := New(Allowed)
	for _, h := range []string{host, "localhost", "127.0.0.2", "10.0.0.1"} {
		s, ctx := open(t, d, protocol.NetDialParams{Host: h, Port: port})
		_, err := s.Recv(ctx)
		if code(err) != protocol.CodeForbiddenTarget {
			t.Errorf("%s: want forbidden_target, got %v", h, err)
		}
	}
	select {
	case <-accepted:
		t.Fatal("the target was reached")
	default:
	}
}

func TestBadParams(t *testing.T) {
	d := New(func(net.IP) bool { return true })
	for _, p := range []protocol.NetDialParams{{Host: "", Port: 80}, {Host: "8.8.8.8", Port: 0}, {Host: "8.8.8.8", Port: 65536}, {Host: "8.8.8.8", Port: -1}} {
		s, ctx := open(t, d, p)
		if _, err := s.Recv(ctx); code(err) != protocol.CodeBadParams {
			t.Errorf("%+v: want bad_params, got %v", p, err)
		}
	}
}

func TestCannotConnect(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close() // nothing listens here now
	s, ctx := open(t, New(func(net.IP) bool { return true }), protocol.NetDialParams{Host: "127.0.0.1", Port: port})
	if _, err := s.Recv(ctx); code(err) != protocol.CodeFailed {
		t.Fatalf("want failed, got %v", err)
	}
}

func TestForwardsBothWays(t *testing.T) {
	host, port, accepted := listen(t)
	s, ctx := open(t, New(func(ip net.IP) bool { return ip.IsLoopback() }), protocol.NetDialParams{Host: host, Port: port})
	ready, err := s.Recv(ctx)
	if err != nil || len(ready) != 0 {
		t.Fatalf("ready frame: %q %v", ready, err)
	}
	target := <-accepted
	defer target.Close()

	if err := s.Send(ctx, []byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(target, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("target got %q %v", buf, err)
	}
	if _, err := target.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	got, err := s.Recv(ctx)
	if err != nil || string(got) != "pong" {
		t.Fatalf("server got %q %v", got, err)
	}

	// A large body goes through in order.
	big := make([]byte, 300<<10)
	for i := range big {
		big[i] = byte(i * 7)
	}
	go func() { _, _ = target.Write(big) }()
	var recv []byte
	for len(recv) < len(big) {
		b, err := s.Recv(ctx)
		if err != nil {
			t.Fatal(err)
		}
		recv = append(recv, b...)
	}
	for i := range big {
		if recv[i] != big[i] {
			t.Fatalf("byte %d differs", i)
		}
	}

	// The target closing ends the stream.
	target.Close()
	if _, err := s.Recv(ctx); !errors.Is(err, io.EOF) {
		t.Fatalf("want EOF after the target closed, got %v", err)
	}
}

func TestServerClosingClosesTarget(t *testing.T) {
	host, port, accepted := listen(t)
	s, ctx := open(t, New(func(ip net.IP) bool { return ip.IsLoopback() }), protocol.NetDialParams{Host: host, Port: port})
	if _, err := s.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	target := <-accepted
	defer target.Close()
	s.Close(nil)
	_ = target.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := target.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("target should see the close, got %v", err)
	}
}

func TestDisabledRefuses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, b := rpc.Pipe()
	server, agent := rpc.NewPeer(a, "s"), rpc.NewPeer(b, "a")
	agent.HandleStream(protocol.MethodNetDial, Disabled)
	go server.Run(ctx)
	go agent.Run(ctx)
	s, err := server.Open(ctx, protocol.MethodNetDial, protocol.NetDialParams{Host: "8.8.8.8", Port: 80})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Recv(ctx); code(err) != protocol.CodeUnsupported {
		t.Fatalf("want unsupported, got %v", err)
	}
}

func TestRegisterCapability(t *testing.T) {
	on := conn.New("https://x.invalid", "t", protocol.Hello{})
	Register(on, false)
	Register(on, false) // announced once
	if n := count(on.Hello.Capabilities, protocol.CapEgress); n != 1 {
		t.Fatalf("egress announced %d times", n)
	}
	off := conn.New("https://x.invalid", "t", protocol.Hello{})
	Register(off, true)
	if count(off.Hello.Capabilities, protocol.CapEgress) != 0 {
		t.Fatal("egress announced although disabled")
	}
}

func count(list []string, v string) int {
	n := 0
	for _, s := range list {
		if s == v {
			n++
		}
	}
	return n
}
