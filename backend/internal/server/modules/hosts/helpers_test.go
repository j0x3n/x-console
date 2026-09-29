package hosts_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// setup starts a full server (hosts is registered in app/modules.go) and
// returns the hosts module for direct calls such as evaluateAlerts.
func setup(t *testing.T) (*testutil.Env, *hosts.Module) {
	t.Helper()
	env := testutil.New(t)
	h, ok := module.Lookup[contracts.Hosts](env.App.Deps.Registry, contracts.HostsKey)
	if !ok {
		t.Fatal("contracts.Hosts not provided")
	}
	return env, h.(*hosts.Module)
}

// startAgent pairs and runs an agent like testutil.Env.Agent, but also
// returns a stop function so tests can take the agent offline.
func startAgent(t *testing.T, env *testutil.Env, name, kind string, caps []string, register func(c *conn.Client)) (string, *conn.Client, func()) {
	t.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]string{"name": name, "kind": kind}, &pc)
	hello := protocol.Hello{AgentVersion: "test", OS: "linux", Arch: "amd64", Hostname: name + "-host", Capabilities: caps}
	if kind == "desktop" {
		hello.OS = "windows"
	}
	id, token, err := conn.Pair(context.Background(), env.Server.URL, pc.Code, hello)
	if err != nil {
		t.Fatal(err)
	}
	client := conn.New(env.Server.URL, token, hello)
	if register != nil {
		register(client)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = client.Run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
		waitFor(t, "agent offline", func() bool { return !env.App.Deps.Agents.Online(id) })
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	waitFor(t, "agent online", func() bool { return env.App.Deps.Agents.Online(id) })
	return id, client, stop
}

// relogin starts a fresh session that is not elevated.
func relogin(t *testing.T, env *testutil.Env) {
	t.Helper()
	env.MustDo(http.MethodPost, "/auth/logout", nil, nil)
	env.MustDo(http.MethodPost, "/auth/login", map[string]string{"username": testutil.Username, "password": testutil.Password, "code": env.Code()}, nil)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// errCode returns the "code" of an error response body.
func errCode(raw []byte) string {
	var e struct{ Code string }
	_ = json.Unmarshal(raw, &e)
	return e.Code
}

func expectStatus(t *testing.T, env *testutil.Env, method, path string, body any, status int, code string) {
	t.Helper()
	got, raw := env.Do(method, path, body, nil)
	if got != status || (code != "" && errCode(raw) != code) {
		t.Fatalf("%s %s: got %d %s, want %d %s", method, path, got, raw, status, code)
	}
}

// rawDo sends a non-JSON request (uploads and downloads).
func rawDo(t *testing.T, env *testutil.Env, method, path string, body io.Reader, size int64) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, env.URL(path), body)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = size
	req.Header.Set("X-Requested-With", "x-console")
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

func sample(cpu float64, memUsed, memTotal uint64, diskUsed uint64) protocol.MetricsSample {
	return protocol.MetricsSample{CPU: cpu, CPUPerCore: []float64{cpu, cpu}, MemUsed: memUsed, MemTotal: memTotal,
		Disks:     []protocol.DiskUsage{{Mount: "/", FSType: "ext4", Used: diskUsed, Total: 100}, {Mount: "/data", FSType: "ext4", Used: 10, Total: 100}},
		NetRxRate: 1000, NetTxRate: 500, Load1: 0.5, UptimeSeconds: 3600, Procs: 99}
}

// ---- fake SSH server ----

type fakeSSH struct {
	addr     string
	password string
	key      ssh.Signer
	mu       sync.Mutex
	commands []string
}

func startFakeSSH(t *testing.T, password string) *fakeSSH {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSSH{password: password, key: signer}
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
		if c.User() == "root" && string(pw) == f.password {
			return nil, nil
		}
		return nil, fmt.Errorf("denied")
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	f.addr = ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c, cfg)
		}
	}()
	return f
}

func (f *fakeSSH) serve(c net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		c.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, in, err := nc.Accept()
		if err != nil {
			continue
		}
		go f.session(ch, in)
	}
}

func exitStatus(ch ssh.Channel, code uint32) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, code)
	_, _ = ch.SendRequest("exit-status", false, b)
	ch.Close()
}

func (f *fakeSSH) session(ch ssh.Channel, in <-chan *ssh.Request) {
	for req := range in {
		switch req.Type {
		case "exec":
			var p struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &p)
			_ = req.Reply(true, nil)
			f.mu.Lock()
			f.commands = append(f.commands, p.Command)
			f.mu.Unlock()
			switch {
			case strings.Contains(p.Command, "@@stat1"):
				_, _ = io.WriteString(ch, hosts.CannedProc)
				exitStatus(ch, 0)
			case p.Command == "fail":
				_, _ = io.WriteString(ch.Stderr(), "boom\n")
				exitStatus(ch, 7)
			default:
				_, _ = io.WriteString(ch, "ran: "+p.Command+"\n")
				exitStatus(ch, 0)
			}
			return
		case "pty-req", "window-change", "env":
			_ = req.Reply(true, nil)
		case "shell":
			_ = req.Reply(true, nil)
			go func() {
				buf := make([]byte, 1024)
				for {
					n, err := ch.Read(buf)
					if n > 0 {
						_, _ = ch.Write(bytes.ToUpper(buf[:n]))
					}
					if err != nil {
						exitStatus(ch, 0)
						return
					}
				}
			}()
		default:
			_ = req.Reply(false, nil)
		}
	}
}

// auditCount counts audit entries of one action.
func auditCount(t *testing.T, env *testutil.Env, action string) int {
	t.Helper()
	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
