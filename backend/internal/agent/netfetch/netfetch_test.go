package netfetch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func fakeFetcher(t *testing.T, handler http.HandlerFunc) *Fetcher {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	f := newFetcher(func(net.IP) bool { return true })
	f.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Local fake server only.
	f.transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(f.transport.CloseIdleConnections)
	return f
}

func TestFetchRedirectsHeadersStatusAndBodyLimit(t *testing.T) {
	f := fakeFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/outside":
			http.Redirect(w, r, "https://evil.test/", 302)
		case "/private":
			http.Redirect(w, r, "https://127.0.0.1/", 302)
		case "/loop":
			http.Redirect(w, r, "https://lrclib.net/loop", 302)
		case "/allowed":
			http.Redirect(w, r, "https://itunes.apple.com/ok", 302)
		case "/big":
			io.WriteString(w, "12345")
		default:
			if r.Method != "GET" {
				t.Errorf("method: %s", r.Method)
			}
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(451)
			io.WriteString(w, r.Header.Get("Accept"))
		}
	})
	call := func(path string, max int) (protocol.NetFetchResult, error) {
		raw, _ := json.Marshal(protocol.NetFetchParams{URL: "https://lrclib.net" + path, Headers: map[string]string{"Accept": "fake"}, MaxBytes: max})
		out, err := f.Fetch(context.Background(), raw)
		if err != nil {
			return protocol.NetFetchResult{}, err
		}
		return out.(protocol.NetFetchResult), nil
	}
	out, err := call("/allowed", 0)
	if err != nil || out.Status != 451 || string(out.Body) != "fake" || out.ContentType != "text/plain" {
		t.Fatalf("response: %+v %v", out, err)
	}
	for _, path := range []string{"/outside", "/private", "/loop", "/big"} {
		if _, err := call(path, 4); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	if _, err := call("/big", 5); err != nil {
		t.Fatalf("exact limit: %v", err)
	}
}

func TestFetchRejectsResolvedPrivateIP(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("private server reached") }))
	defer server.Close()
	f := New()
	defer f.transport.CloseIdleConnections()
	f.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	dialer := &net.Dialer{Control: f.control}
	f.transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, server.Listener.Addr().String())
	}
	raw := protocol.Marshal(protocol.NetFetchParams{URL: "https://lrclib.net/a"})
	if _, err := f.Fetch(context.Background(), raw); err == nil || !strings.Contains(err.Error(), "公网") {
		t.Fatalf("private DNS target: %v", err)
	}
}

func TestFetchHonorsCancellationAndRejectsParameters(t *testing.T) {
	f := fakeFetcher(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Fetch(ctx, protocol.Marshal(protocol.NetFetchParams{URL: "https://lrclib.net"})); err == nil {
		t.Fatal("canceled request succeeded")
	}
	for _, raw := range []string{`{`, `{"url":"http://lrclib.net"}`, `{"url":"https://lrclib.net","headers":{"Host":"evil"}}`} {
		if _, err := f.Fetch(context.Background(), []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	start := time.Now()
	if _, err := f.Fetch(context.Background(), protocol.Marshal(protocol.NetFetchParams{URL: "https://lrclib.net", TimeoutMs: 20})); err == nil || time.Since(start) > time.Second {
		t.Fatalf("timeout limit: %v", err)
	}
}
