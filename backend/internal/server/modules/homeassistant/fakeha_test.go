package homeassistant_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	goodToken = "good-long-lived-token-1234"
	haVersion = "2026.9.1"
)

// fakeHA implements the parts of Home Assistant the module uses: REST
// /api/config and the WebSocket API (auth, subscribe_events, get_states,
// call_service, ping).
type fakeHA struct {
	srv *httptest.Server

	mu       sync.Mutex
	states   map[string]map[string]any
	conns    map[*websocket.Conn]int64 // subscription id, 0 when not subscribed
	calls    []map[string]any
	authOK   int
	authFail int
}

func newFakeHA(t *testing.T) *fakeHA {
	f := &fakeHA{states: map[string]map[string]any{}, conns: map[*websocket.Conn]int64{}}
	f.set("light.kitchen", "off", map[string]any{"friendly_name": "Kitchen light"})
	f.set("switch.fan", "on", map[string]any{"friendly_name": "Fan"})
	f.set("sensor.temperature", "21.5", map[string]any{"friendly_name": "Living room temperature", "unit_of_measurement": "°C"})
	f.set("lock.front_door", "locked", map[string]any{"friendly_name": "Front door"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", f.serveConfig)
	mux.HandleFunc("/api/websocket", f.serveWS)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(func() {
		f.dropAll()
		f.srv.CloseClientConnections()
		f.srv.Close()
	})
	return f
}

func (f *fakeHA) URL() string { return f.srv.URL }

func rawState(id, state string, attrs map[string]any) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return map[string]any{"entity_id": id, "state": state, "attributes": attrs, "last_changed": now, "last_updated": now, "context": map[string]any{"id": "x"}}
}

// set changes a state without telling anyone (as if it changed while the
// module was disconnected).
func (f *fakeHA) set(id, state string, attrs map[string]any) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if attrs == nil {
		if old, ok := f.states[id]; ok {
			attrs = old["attributes"].(map[string]any)
		} else {
			attrs = map[string]any{}
		}
	}
	s := rawState(id, state, attrs)
	f.states[id] = s
	return s
}

// push changes a state and sends state_changed to subscribers.
func (f *fakeHA) push(id, state string) {
	s := f.set(id, state, nil)
	f.mu.Lock()
	conns := map[*websocket.Conn]int64{}
	for c, sub := range f.conns {
		if sub != 0 {
			conns[c] = sub
		}
	}
	f.mu.Unlock()
	for c, sub := range conns {
		_ = wsjson.Write(context.Background(), c, map[string]any{"id": sub, "type": "event", "event": map[string]any{
			"event_type": "state_changed", "data": map[string]any{"entity_id": id, "new_state": s}, "origin": "LOCAL"}})
	}
}

func (f *fakeHA) dropAll() {
	f.mu.Lock()
	conns := f.conns
	f.conns = map[*websocket.Conn]int64{}
	f.mu.Unlock()
	for c := range conns {
		_ = c.Close(websocket.StatusGoingAway, "restarting")
	}
}

func (f *fakeHA) counts() (ok, fail int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authOK, f.authFail
}

func (f *fakeHA) lastCall() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func (f *fakeHA) serveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+goodToken {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("401: Unauthorized"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"version": haVersion, "location_name": "Home", "time_zone": "Asia/Shanghai"})
}

func (f *fakeHA) serveWS(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	ctx := context.Background()
	defer c.CloseNow()
	if wsjson.Write(ctx, c, map[string]any{"type": "auth_required", "ha_version": haVersion}) != nil {
		return
	}
	var authMsg struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if wsjson.Read(ctx, c, &authMsg) != nil {
		return
	}
	f.mu.Lock()
	good := authMsg.Type == "auth" && authMsg.AccessToken == goodToken
	if good {
		f.authOK++
		f.conns[c] = 0
	} else {
		f.authFail++
	}
	f.mu.Unlock()
	if !good {
		_ = wsjson.Write(ctx, c, map[string]any{"type": "auth_invalid", "message": "Invalid access token or password"})
		_ = c.Close(websocket.StatusNormalClosure, "")
		return
	}
	_ = wsjson.Write(ctx, c, map[string]any{"type": "auth_ok", "ha_version": haVersion})
	defer func() {
		f.mu.Lock()
		delete(f.conns, c)
		f.mu.Unlock()
	}()
	for {
		var msg map[string]any
		if err := wsjson.Read(ctx, c, &msg); err != nil {
			return
		}
		id := msg["id"]
		reply := map[string]any{"id": id, "type": "result", "success": true, "result": nil}
		switch msg["type"] {
		case "subscribe_events":
			f.mu.Lock()
			if _, ok := f.conns[c]; ok {
				f.conns[c] = int64(id.(float64))
			}
			f.mu.Unlock()
		case "get_states":
			f.mu.Lock()
			list := make([]map[string]any, 0, len(f.states))
			for _, s := range f.states {
				list = append(list, s)
			}
			f.mu.Unlock()
			reply["result"] = list
		case "ping":
			reply = map[string]any{"id": id, "type": "pong"}
		case "call_service":
			f.mu.Lock()
			f.calls = append(f.calls, msg)
			f.mu.Unlock()
			domain, _ := msg["domain"].(string)
			service, _ := msg["service"].(string)
			target, _ := msg["target"].(map[string]any)
			entity, _ := target["entity_id"].(string)
			if data, ok := msg["service_data"].(map[string]any); ok && entity == "" {
				entity, _ = data["entity_id"].(string)
			}
			switch {
			case service == "turn_on" || service == "turn_off" || service == "unlock" || service == "lock":
				_ = wsjson.Write(ctx, c, reply)
				if entity != "" {
					f.push(entity, map[string]string{"turn_on": "on", "turn_off": "off", "unlock": "unlocked", "lock": "locked"}[service])
				}
				continue
			case strings.HasPrefix(service, "missing"):
				reply = map[string]any{"id": id, "type": "result", "success": false,
					"error": map[string]any{"code": "not_found", "message": "Service " + domain + "." + service + " not found."}}
			}
		default:
			reply = map[string]any{"id": id, "type": "result", "success": false, "error": map[string]any{"code": "unknown_command", "message": "Unknown command."}}
		}
		if wsjson.Write(ctx, c, reply) != nil {
			return
		}
	}
}
