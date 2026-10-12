package hosts

import (
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestGeoDownloadLookupRetainsExistingOnFailure(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "GeoIP2-Country-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	var bad atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := gzip.NewWriter(w)
		defer writer.Close()
		if bad.Load() {
			_, _ = writer.Write([]byte("invalid database"))
		} else {
			_, _ = writer.Write(raw)
		}
	}))
	defer server.Close()
	g := newGeoResolver(t.TempDir())
	defer g.close()
	g.url = server.URL
	month := time.Now().UTC().Format("2006-01")
	if err := g.download(context.Background(), month); err != nil {
		t.Fatal(err)
	}
	if got := g.country(context.Background(), "81.2.69.142"); got != "GB" {
		t.Fatalf("country %q", got)
	}
	if countryName("JP") != "日本" || countryName("US") != "美国" {
		t.Fatal("Chinese country names")
	}
	bad.Store(true)
	if err := g.download(context.Background(), "2099-01"); err == nil {
		t.Fatal("invalid database accepted")
	}
	if got := g.country(context.Background(), "81.2.69.142"); got != "GB" {
		t.Fatalf("existing database lost %q", got)
	}
	g.close()
	bad.Store(false)
	if err := g.download(context.Background(), month); err == nil {
		t.Fatal("closed resolver reopened")
	}
}

func TestHostAddressesNormalizePublicAndPrivate(t *testing.T) {
	input := []protocol.HostAddress{{IP: "127.0.0.1"}, {IP: "fe80::1"}, {IP: "2001:db8::1", Public: true}, {IP: "100.64.0.1", Public: true}, {IP: "10.0.0.2"}, {IP: "8.8.8.8"}, {IP: "::ffff:8.8.8.8"}, {IP: "2001:4860::8888"}, {IP: "invalid"}}
	addresses := normalizeHostAddresses(input)
	if len(addresses) != 5 || addresses[0].IP != "8.8.8.8" || addresses[1].IP != "2001:4860::8888" {
		t.Fatalf("addresses %+v", addresses)
	}
	for _, a := range addresses[2:] {
		if a.Public {
			t.Fatalf("reserved marked public %+v", a)
		}
	}
}

func TestGeoIPv6Lookup(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("testdata", "GeoIP2-Country-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "geo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geo", "country.mmdb"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	g := newGeoResolver(dir)
	defer g.close()
	if got := g.country(context.Background(), "2001:218::"); got != "JP" {
		t.Fatalf("IPv6 country %q", got)
	}
}

func TestGeoRetriesFailedDownload(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "GeoIP2-Country-Test.mmdb"))
	if err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer := gzip.NewWriter(w)
		defer writer.Close()
		_, _ = writer.Write(raw)
	}))
	defer server.Close()
	g := newGeoResolver(t.TempDir())
	defer g.close()
	g.url = server.URL
	g.country(context.Background(), "81.2.69.142")
	wait := func(ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			g.mu.RLock()
			ok := ready()
			g.mu.RUnlock()
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("国家库下载未结束")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(func() bool { return !g.fetching && !g.retryAt.IsZero() })
	g.country(context.Background(), "81.2.69.142")
	if hits.Load() != 1 {
		t.Fatal("重试间隔内重复下载")
	}
	// 推进重试时间，不等待一小时。
	g.mu.Lock()
	g.retryAt = time.Now().Add(-time.Second)
	g.mu.Unlock()
	g.country(context.Background(), "81.2.69.142")
	wait(func() bool { return !g.fetching && g.reader != nil })
	if got := g.country(context.Background(), "81.2.69.142"); got != "GB" || hits.Load() != 2 {
		t.Fatalf("重试后国家 %q，请求数 %d", got, hits.Load())
	}
}
