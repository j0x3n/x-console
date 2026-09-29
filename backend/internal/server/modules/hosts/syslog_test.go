package hosts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// fakeSyslog answers syslog.* like the agent package would.
type fakeSyslog struct {
	mu        sync.Mutex
	queries   []protocol.SyslogQueryParams
	unitCalls int
	denied    bool // every call fails with the permission error
	followEnd chan struct{}
}

func (f *fakeSyslog) register(c *conn.Client) {
	deny := &protocol.Error{Code: protocol.CodeSyslogPermission, Message: "代理没有读取系统日志的权限。在服务器上执行：sudo usermod -aG systemd-journal <代理运行的用户>，然后重启代理。"}
	c.Handle(protocol.MethodSyslogQuery, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.SyslogQueryParams
		_ = json.Unmarshal(raw, &p)
		f.mu.Lock()
		f.queries = append(f.queries, p)
		denied := f.denied
		f.mu.Unlock()
		if denied {
			return nil, deny
		}
		page := protocol.SyslogPage{Items: []protocol.SyslogEntry{
			{Time: time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC), Priority: 3, Unit: "ssh.service", PID: 77, Message: "Failed password"},
			{Time: time.Date(2026, 9, 29, 10, 0, 1, 0, time.UTC), Priority: 6, Unit: "cron", Message: "job ran"},
		}}
		if p.Cursor == "" {
			page.Cursor = "older"
		}
		return page, nil
	})
	c.Handle(protocol.MethodSyslogUnits, func(ctx context.Context, raw json.RawMessage) (any, error) {
		f.mu.Lock()
		f.unitCalls++
		f.mu.Unlock()
		return protocol.SyslogUnits{Items: []string{"ssh.service", "cron"}}, nil
	})
	c.HandleStream(protocol.MethodSyslogFollow, func(ctx context.Context, raw json.RawMessage, s *rpc.Stream) error {
		f.mu.Lock()
		denied := f.denied
		f.mu.Unlock()
		if denied {
			return deny
		}
		_ = s.Send(ctx, []byte(`[{"time":"2026-09-29T10:00:02Z","priority":4,"unit":"kernel","message":"live line"}]`))
		<-s.Context().Done()
		close(f.followEnd)
		return nil
	})
}

func TestSyslog(t *testing.T) {
	env, _ := setup(t)
	fake := &fakeSyslog{followEnd: make(chan struct{})}
	caps := append([]string{protocol.CapSyslog}, serverCaps...)
	id, _, _ := startAgent(t, env, "logbox", "server", caps, fake.register)
	plain, _, _ := startAgent(t, env, "plain", "server", serverCaps, nil)
	base := "/hosts/" + id + "/syslog"

	var page api.SyslogPage
	env.MustDo(http.MethodGet, base+"?priority=4&unit=ssh.service&grep=pass&limit=20&since=2026-09-29T09:00:00Z", nil, &page)
	if len(page.Items) != 2 || page.Items[0].Unit != "ssh.service" || *page.Items[0].Pid != 77 || page.Items[1].Pid != nil || page.Cursor == nil || *page.Cursor != "older" {
		t.Fatalf("page: %+v", page)
	}
	fake.mu.Lock()
	q := fake.queries[0]
	fake.mu.Unlock()
	if *q.Priority != 4 || q.Unit != "ssh.service" || q.Grep != "pass" || q.Limit != 20 || q.Since == nil || !q.Since.Equal(time.Date(2026, 9, 29, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("params to the agent: %+v", q)
	}
	// Turning the page passes the cursor and is not audited again.
	before := auditCount(t, env, "host.syslog.read")
	page = api.SyslogPage{}
	env.MustDo(http.MethodGet, base+"?cursor=older", nil, &page)
	if page.Cursor != nil || auditCount(t, env, "host.syslog.read") != before || before != 1 {
		t.Fatalf("second page: %+v, audits %d", page, before)
	}
	expectStatus(t, env, http.MethodGet, base+"?limit=0", nil, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodGet, base+"?priority=9", nil, http.StatusBadRequest, "validation_failed")
	expectStatus(t, env, http.MethodGet, "/hosts/"+plain+"/syslog", nil, http.StatusNotImplemented, "feature_unavailable")
	expectStatus(t, env, http.MethodGet, "/hosts/ssh:1/syslog", nil, http.StatusNotFound, "not_found")

	// The unit list is cached for ten minutes.
	var units struct{ Items []string }
	env.MustDo(http.MethodGet, base+"/units", nil, &units)
	env.MustDo(http.MethodGet, base+"/units", nil, &units)
	if strings.Join(units.Items, ",") != "ssh.service,cron" || fake.unitCalls != 1 {
		t.Fatalf("units: %v, %d calls", units.Items, fake.unitCalls)
	}

	// Follow.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, env.WSURL(base+"/follow?priority=6&grep=live"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err != nil {
		t.Fatal(err)
	}
	typ, b, err := ws.Read(ctx)
	var entries []api.SyslogEntry
	if err != nil || typ != websocket.MessageText || json.Unmarshal(b, &entries) != nil || len(entries) != 1 || entries[0].Message != "live line" || entries[0].Priority != 4 {
		t.Fatalf("follow frame %q: %v", b, err)
	}
	_ = ws.Close(websocket.StatusNormalClosure, "")
	select {
	case <-fake.followEnd:
	case <-ctx.Done():
		t.Fatal("follow stream was not closed on the agent")
	}
	_, resp, err := websocket.Dial(ctx, env.WSURL("/hosts/"+plain+"/syslog/follow"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("follow without the capability: %v %+v", err, resp)
	}

	// No permission: 403 with the code the page looks for, and the way to fix it.
	fake.mu.Lock()
	fake.denied = true
	fake.mu.Unlock()
	code, body := env.Do(http.MethodGet, base, nil, nil)
	if code != http.StatusForbidden || errCode(body) != "syslog_permission" || !strings.Contains(string(body), "usermod -aG systemd-journal") {
		t.Fatalf("denied query: %d %s", code, body)
	}
	_, resp, err = websocket.Dial(ctx, env.WSURL(base+"/follow"), &websocket.DialOptions{HTTPHeader: wsHeader(env)})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("denied follow: %v %+v", err, resp)
	}
}
