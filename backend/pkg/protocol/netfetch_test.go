package protocol_test

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestNetFetchTargetsAndLimits(t *testing.T) {
	for _, u := range []string{"https://music.163.com/api/search", "https://p1.music.126.net/a.jpg", "https://is1-ssl.mzstatic.com/a", "https://ia800.archive.org/a", "https://y.gtimg.cn/a"} {
		if err := protocol.ValidateNetFetchURL(u); err != nil {
			t.Fatalf("allowed %q: %v", u, err)
		}
	}
	for _, u := range []string{"http://music.163.com/a", "https://evil.test/a", "https://music.163.com.evil.test/a", "https://mzstatic.com/a", "https://127.0.0.1/a", "https://10.0.0.1/a", "https://[::1]/a", "https://user:pass@music.163.com/a", "https://music.163.com:1234/a"} {
		if protocol.ValidateNetFetchURL(u) == nil {
			t.Fatalf("accepted %q", u)
		}
	}
	p := protocol.NetFetchParams{URL: "https://lrclib.net/api/search"}
	if err := p.Validate(); err != nil || p.MaxBytes != 8<<20 || p.TimeoutMs != 8000 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	for _, p := range []protocol.NetFetchParams{
		{URL: "https://lrclib.net", MaxBytes: 8<<20 + 1}, {URL: "https://lrclib.net", TimeoutMs: 15001},
		{URL: "https://lrclib.net", Headers: map[string]string{"Authorization": "secret"}},
		{URL: "https://lrclib.net", Headers: map[string]string{"Cookie": "x\r\nHost: evil"}},
	} {
		if p.Validate() == nil {
			t.Fatalf("accepted %+v", p)
		}
	}
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "172.16.0.1", "169.254.1.1", "::1", "fc00::1", "fe80::1", "100.64.0.1", "0.0.0.0"} {
		if protocol.NetFetchIPAllowed(net.ParseIP(ip)) {
			t.Fatalf("accepted IP %s", ip)
		}
	}
	if !protocol.NetFetchIPAllowed(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	data, _ := json.Marshal(protocol.NetFetchResult{Status: 200, Body: []byte("abc")})
	if !strings.Contains(string(data), `"body":"YWJj"`) {
		t.Fatalf("body is not base64: %s", data)
	}
}
