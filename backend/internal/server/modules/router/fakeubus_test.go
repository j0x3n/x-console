package router_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// fakeUbus answers uhttpd's /ubus JSON-RPC like an OpenWrt router.
type fakeUbus struct {
	srv *httptest.Server

	mu       sync.Mutex
	sessions map[string]bool
	logins   int
	wanUp    bool
	rx, tx   uint64
	calls    []string // object.method of every call after login
	denied   map[string]bool
	leases   []map[string]any
	hints    map[string]any
	// B93: a router without LuCI, odhcpd leases, files rpcd-mod-file can read
	noLuci bool
	odhcpd []map[string]any
	files  map[string]string
}

func newFakeUbus(t *testing.T) *fakeUbus {
	f := &fakeUbus{sessions: map[string]bool{}, wanUp: true, rx: 1000, tx: 500, denied: map[string]bool{},
		leases: []map[string]any{
			{"expires": 3600, "hostname": "nas", "macaddr": "aa:bb:cc:00:00:02", "ipaddr": "192.168.1.20"},
			{"expires": 600, "hostname": "phone", "macaddr": "aa:bb:cc:00:00:03", "ipaddr": "192.168.1.3"},
		},
		hints: map[string]any{
			"AA:BB:CC:00:00:02": map[string]any{"name": "nas", "ipaddrs": []string{"192.168.1.20"}, "ip6addrs": []string{"fd00::20"}},
			"AA:BB:CC:00:00:09": map[string]any{"name": "printer", "ipaddrs": []string{"192.168.1.100"}},
			"AA:BB:CC:00:00:0A": map[string]any{"name": "old"},
		}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeUbus) URL() string { return f.srv.URL }

// expire drops every session, as if rpcd restarted.
func (f *fakeUbus) expire() {
	f.mu.Lock()
	f.sessions = map[string]bool{}
	f.mu.Unlock()
}

func (f *fakeUbus) set(fn func(f *fakeUbus)) {
	f.mu.Lock()
	fn(f)
	f.mu.Unlock()
}

func (f *fakeUbus) called(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == name {
			return true
		}
	}
	return false
}

func (f *fakeUbus) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ubus" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		ID     int               `json:"id"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Params) != 4 {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	var session, object, method string
	_ = json.Unmarshal(req.Params[0], &session)
	_ = json.Unmarshal(req.Params[1], &object)
	_ = json.Unmarshal(req.Params[2], &method)
	reply := func(v ...any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": v})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if object == "session" && method == "login" {
		var args struct{ Username, Password string }
		_ = json.Unmarshal(req.Params[3], &args)
		if args.Username != "xconsole" || args.Password != "secret" {
			reply(6)
			return
		}
		f.logins++
		id := fmt.Sprintf("%032d", f.logins)
		f.sessions[id] = true
		reply(0, map[string]any{"ubus_rpc_session": id, "timeout": 300, "expires": 300})
		return
	}
	if !f.sessions[session] {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32002, "message": "Access denied"}})
		return
	}
	name := object + "." + method
	f.calls = append(f.calls, name)
	if f.denied[name] {
		reply(6)
		return
	}
	if f.noLuci && object == "luci-rpc" {
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32000, "message": "Object not found"}})
		return
	}
	switch name {
	case "dhcp.ipv4leases":
		if f.odhcpd == nil {
			reply(4)
			return
		}
		reply(0, map[string]any{"device": map[string]any{"br-lan": map[string]any{"leases": f.odhcpd}}})
		return
	case "file.read":
		var args struct{ Path string }
		_ = json.Unmarshal(req.Params[3], &args)
		data, ok := f.files[args.Path]
		if !ok {
			reply(4)
			return
		}
		reply(0, map[string]any{"data": data})
		return
	case "system.board":
		reply(0, map[string]any{"hostname": "OpenWrt", "model": "Xiaomi AX3600",
			"release": map[string]any{"distribution": "OpenWrt", "version": "24.10.0", "description": "OpenWrt 24.10.0 r28427"}})
	case "system.info":
		reply(0, map[string]any{"uptime": 3600, "load": []int{65536, 32768, 0},
			"memory": map[string]any{"total": 512 << 20, "free": 100 << 20, "available": 300 << 20}})
	case "network.interface.dump":
		wan := map[string]any{"interface": "wan", "up": f.wanUp, "proto": "pppoe", "device": "eth1", "l3_device": "pppoe-wan"}
		if f.wanUp {
			wan["uptime"] = 1200
			wan["ipv4-address"] = []map[string]any{{"address": "100.64.1.2", "mask": 32}}
		}
		reply(0, map[string]any{"interface": []map[string]any{
			{"interface": "lan", "up": true, "uptime": 3600, "proto": "static", "device": "br-lan", "l3_device": "br-lan",
				"ipv4-address": []map[string]any{{"address": "192.168.1.1", "mask": 24}}},
			{"interface": "loopback", "up": true, "proto": "static", "device": "lo"},
			wan,
		}})
	case "network.device.status":
		var args struct{ Name string }
		_ = json.Unmarshal(req.Params[3], &args)
		if args.Name != "pppoe-wan" {
			reply(4)
			return
		}
		reply(0, map[string]any{"up": true, "statistics": map[string]any{"rx_bytes": f.rx, "tx_bytes": f.tx}})
	case "luci-rpc.getDHCPLeases":
		reply(0, map[string]any{"dhcp_leases": f.leases})
	case "luci-rpc.getHostHints":
		reply(0, f.hints)
	case "network.interface.wan.down", "network.interface.wan.up", "system.reboot":
		reply(0)
	default:
		reply(3)
	}
}
