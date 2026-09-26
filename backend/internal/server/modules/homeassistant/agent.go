package homeassistant

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

// viaAgent reaches HA through a paired agent on the home network, using the
// agent's http.proxy and ws.proxy methods.
type viaAgent struct {
	hub     *agenthub.Hub
	agentID string
}

func (v *viaAgent) dial(ctx context.Context, u string) (msgConn, error) {
	s, err := v.hub.Open(ctx, v.agentID, protocol.MethodWSProxy, protocol.WSProxyParams{URL: u})
	if err != nil {
		return nil, err
	}
	return &streamConn{s: s}, nil
}

func (v *viaAgent) do(ctx context.Context, method, u string, header http.Header, body []byte) (int, []byte, error) {
	var res protocol.HTTPProxyResult
	err := v.hub.Call(ctx, v.agentID, protocol.MethodHTTPProxy,
		protocol.HTTPProxyParams{Method: method, URL: u, Header: header, Body: body}, &res)
	if err != nil {
		return 0, nil, err
	}
	return res.Status, res.Body, nil
}

// streamConn carries framed WebSocket messages over an agent stream.
type streamConn struct {
	s   *rpc.Stream
	j   protocol.WSProxyJoiner
	wmu sync.Mutex
}

func (c *streamConn) Read(ctx context.Context) ([]byte, error) {
	for {
		frame, err := c.s.Recv(ctx)
		if errors.Is(err, io.EOF) {
			return nil, errors.New("代理关闭了到 Home Assistant 的连接")
		}
		if errors.Is(err, rpc.ErrClosed) {
			return nil, httpx.ErrAgentOffline
		}
		if err != nil {
			return nil, err
		}
		msg, done, err := c.j.Add(frame)
		if err != nil {
			return nil, err
		}
		if done {
			return msg, nil
		}
	}
}

func (c *streamConn) Write(ctx context.Context, msg []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	for _, frame := range protocol.WSProxySplit(msg) {
		if err := c.s.Send(ctx, frame); err != nil {
			return err
		}
	}
	return nil
}

func (c *streamConn) Close() error {
	c.s.Close(nil)
	return nil
}

// checkAgent validates the agent used in agent mode. The capability list is
// the one the agent announced when it last connected.
func (m *Module) checkAgent(ctx context.Context, agentID string) error {
	if agentID == "" {
		return httpx.Invalid("请选择一个代理")
	}
	a, err := m.d.Agents.Get(ctx, agentID)
	if errors.Is(err, httpx.ErrNotFound) {
		return httpx.Invalid("这个代理不存在或已吊销")
	}
	if err != nil {
		return err
	}
	if !a.Has(protocol.CapProxy) {
		return httpx.Invalid("这个代理不支持转发。请把代理程序升级到新版本")
	}
	return nil
}

// watchAgents retries right away when the configured agent comes online,
// instead of waiting for the backoff.
func (m *Module) watchAgents(ctx context.Context) {
	ch, cancel := m.d.Bus.Subscribe("agent.online", 16)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				data, _ := ev.Data.(map[string]string)
				cfg, err := m.loadConfig(ctx)
				if err == nil && cfg.AgentID != "" && data["agentId"] == cfg.AgentID {
					m.c.Wake()
				}
			}
		}
	}()
}
