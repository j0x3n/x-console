package hosts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func pairInfoHost(t *testing.T, env *testutil.Env, name, kind string, info any) (string, string) {
	t.Helper()
	env.Elevate()
	var pc struct{ Code string }
	env.MustDo(http.MethodPost, "/agents/pairing-codes", map[string]any{"name": name, "kind": kind, "info": info}, &pc)
	id, token, err := conn.Pair(context.Background(), env.Server.URL, pc.Code, protocol.Hello{OS: "linux", Arch: "amd64", Hostname: name, AgentVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func TestHostInfoPasswordEncryptionAndPatch(t *testing.T) {
	env := testutil.New(t, hosts.New)
	id, _ := pairInfoHost(t, env, "客户服务器", "server", map[string]any{"ownership": "client", "client": "客户甲", "username": "operator", "password": "secret-host-password", "note": "备注", "tags": []string{"生产", "生产", " 日本 "}})
	var h api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &h)
	if h.Info.Ownership != api.HostInfoOwnershipClient || !h.Info.HasPassword || h.Info.Client == nil || *h.Info.Client != "客户甲" || len(h.Info.Tags) != 2 {
		t.Fatalf("host info %+v", h.Info)
	}
	var encrypted, sidecar string
	if err := env.App.Deps.DB.QueryRow("SELECT password_enc FROM host_info WHERE host_id=?", id).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if encrypted == "secret-host-password" || strings.Contains(encrypted, "secret-host-password") {
		t.Fatal("plaintext password stored")
	}
	plain, err := env.App.Deps.Secrets.Open(encrypted)
	if err != nil || plain != "secret-host-password" {
		t.Fatalf("encrypted password %q %v", plain, err)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM host_pairing_info").Scan(&sidecar); err != nil || sidecar != "0" {
		t.Fatalf("pairing metadata retained %q %v", sidecar, err)
	}
	if _, err := env.App.Deps.DB.Exec("UPDATE sessions SET elevated_until=NULL"); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do(http.MethodGet, "/hosts/"+id+"/password", nil, nil); status != 403 {
		t.Fatalf("password status %d", status)
	}
	if status, _ := env.Do(http.MethodPatch, "/hosts/"+id, map[string]any{"info": map[string]any{"password": "changed"}}, nil); status != 403 {
		t.Fatalf("update password status %d", status)
	}
	env.MustDo(http.MethodPatch, "/hosts/"+id, map[string]any{"name": "新名称", "info": map[string]any{"note": "新备注"}}, nil)
	var updated api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &updated)
	if updated.Name != "新名称" || !updated.Info.HasPassword || updated.Info.Note == nil || *updated.Info.Note != "新备注" {
		t.Fatalf("patch %+v", updated)
	}
	env.Elevate()
	var password struct{ Password string }
	env.MustDo(http.MethodGet, "/hosts/"+id+"/password", nil, &password)
	if password.Password != "secret-host-password" {
		t.Fatal(password.Password)
	}
	if status, _ := env.Do(http.MethodPatch, "/hosts/"+id, map[string]any{"info": map[string]any{"password": "x", "clearPassword": true}}, nil); status != 400 {
		t.Fatalf("conflicting password %d", status)
	}
	env.MustDo(http.MethodPatch, "/hosts/"+id, map[string]any{"info": map[string]any{"ownership": "own", "clearPassword": true, "tags": []string{}}}, nil)
	var cleared api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &cleared)
	if cleared.Info.HasPassword || cleared.Info.Client != nil || len(cleared.Info.Tags) != 0 {
		t.Fatalf("clear %+v", cleared.Info)
	}
}

func TestHostOrderKindIsolationAndSSHInfo(t *testing.T) {
	env := testutil.New(t, hosts.New)
	first, _ := pairInfoHost(t, env, "A", "server", nil)
	second, _ := pairInfoHost(t, env, "Z", "server", nil)
	desktop, _ := pairInfoHost(t, env, "电脑", "desktop", nil)
	env.MustDo(http.MethodPut, "/hosts/order", map[string]any{"kind": "server", "ids": []string{second, first}}, nil)
	var list []api.HostListItem
	env.MustDo(http.MethodGet, "/hosts?kind=server", nil, &list)
	if len(list) != 2 || list[0].Id != second || list[1].Id != first {
		t.Fatalf("order %+v", list)
	}
	if status, _ := env.Do(http.MethodPut, "/hosts/order", map[string]any{"kind": "server", "ids": []string{desktop, first}}, nil); status != 400 {
		t.Fatalf("cross kind %d", status)
	}
	if status, _ := env.Do(http.MethodPut, "/hosts/order", map[string]any{"kind": "server", "ids": []string{first, first}}, nil); status != 400 {
		t.Fatalf("duplicate %d", status)
	}
	env.MustDo(http.MethodGet, "/hosts?kind=desktop", nil, &list)
	if len(list) != 1 || list[0].Id != desktop || list[0].SortOrder != 0 {
		t.Fatalf("desktop order %+v", list)
	}
	var ssh api.SshHost
	env.MustDo(http.MethodPost, "/ssh-hosts", map[string]any{"name": "SSH客户", "address": "127.0.0.1", "username": "root", "auth": "password", "secret": "ssh-credential", "info": map[string]any{"ownership": "client", "client": "客户乙", "password": "info-password"}}, &ssh)
	var detail api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+ssh.HostId, nil, &detail)
	if detail.SortOrder != 2 || !detail.Info.HasPassword {
		t.Fatalf("ssh metadata %+v", detail)
	}
	env.MustDo(http.MethodPut, "/ssh-hosts/"+jsonID(ssh.Id), map[string]any{"name": "SSH更新", "address": "127.0.0.1", "username": "root", "auth": "password", "info": map[string]any{"note": "独立凭据"}}, nil)
	var secret string
	if err := env.App.Deps.DB.QueryRow("SELECT secret FROM ssh_hosts WHERE id=?", ssh.Id).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	plain, err := env.App.Deps.Secrets.Open(secret)
	if err != nil || !strings.Contains(plain, "ssh-credential") {
		t.Fatalf("ssh credential changed %q %v", plain, err)
	}
}

func TestHostAddressesStoredAcrossListAndDetail(t *testing.T) {
	env := testutil.New(t, hosts.New)
	id := env.Agent("server", []string{protocol.CapSystemInfo}, func(c *conn.Client) {
		c.Handle(protocol.MethodSystemInfo, func(context.Context, json.RawMessage) (any, error) {
			return protocol.SystemInfo{Addresses: []protocol.HostAddress{{IP: "10.0.0.2"}, {IP: "2001:db8::1"}, {IP: "127.0.0.1"}, {IP: "fe80::1"}}}, nil
		})
	})
	var detail api.HostDetail
	env.MustDo(http.MethodGet, "/hosts/"+id, nil, &detail)
	if len(detail.Addresses) != 2 {
		t.Fatalf("detail addresses %+v", detail.Addresses)
	}
	var list []api.HostListItem
	env.MustDo(http.MethodGet, "/hosts?kind=server", nil, &list)
	if len(list) != 1 || len(list[0].Addresses) != 2 {
		t.Fatalf("list addresses %+v", list)
	}
}

func jsonID(id int64) string { raw, _ := json.Marshal(id); return string(raw) }
