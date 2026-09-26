package netproxy

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestAllowed(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.9", "192.168.1.20", "100.101.102.103", "169.254.1.1", "fe80::1", "fd00::5"} {
		if !Allowed(net.ParseIP(ip)) {
			t.Errorf("%s should be allowed", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "100.128.0.1", "0.0.0.0", "2001:4860:4860::8888", "172.32.0.1"} {
		if Allowed(net.ParseIP(ip)) {
			t.Errorf("%s should be rejected", ip)
		}
	}
}

func call(t *testing.T, p *Proxy, in protocol.HTTPProxyParams) (protocol.HTTPProxyResult, error) {
	t.Helper()
	raw, _ := json.Marshal(in)
	out, err := p.HTTP(context.Background(), raw)
	if err != nil {
		return protocol.HTTPProxyResult{}, err
	}
	return out.(protocol.HTTPProxyResult), nil
}

func code(err error) string {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestHTTPLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer x" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("X-Test", "1")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	res, err := call(t, New(), protocol.HTTPProxyParams{Method: http.MethodPost, URL: srv.URL + "/api/", Header: map[string][]string{"authorization": {"Bearer x"}}})
	if err != nil || res.Status != http.StatusTeapot || string(res.Body) != "hello" || res.Header["X-Test"][0] != "1" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestHTTPRejectsPublicAndBadURLs(t *testing.T) {
	p := New()
	if _, err := call(t, p, protocol.HTTPProxyParams{URL: "http://8.8.8.8/"}); code(err) != protocol.CodeForbiddenTarget {
		t.Fatalf("public ip: %v", err)
	}
	for _, u := range []string{"ftp://10.0.0.1/", "http:///x", "http://user:pw@10.0.0.1/"} {
		if _, err := call(t, p, protocol.HTTPProxyParams{URL: u}); code(err) != protocol.CodeBadParams {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

// A host name is checked after resolution, when the connection is made.
func TestHTTPChecksResolvedAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	deny := newProxy(func(net.IP) bool { return false })
	_, err := call(t, deny, protocol.HTTPProxyParams{URL: "http://localhost:" + port + "/"})
	if code(err) != protocol.CodeForbiddenTarget {
		t.Fatalf("resolved address not checked: %v", err)
	}
	if _, err := call(t, New(), protocol.HTTPProxyParams{URL: "http://localhost:" + port + "/"}); err != nil {
		t.Fatalf("localhost: %v", err)
	}
}

func TestHTTPDoesNotFollowRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://8.8.8.8/", http.StatusFound)
	}))
	defer srv.Close()
	res, err := call(t, New(), protocol.HTTPProxyParams{URL: srv.URL})
	if err != nil || res.Status != http.StatusFound {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestFraming(t *testing.T) {
	big := []byte(strings.Repeat("a", protocol.WSProxyChunk*2+10))
	for _, msg := range [][]byte{nil, []byte("{}"), big} {
		frames := protocol.WSProxySplit(msg)
		var j protocol.WSProxyJoiner
		for i, f := range frames {
			out, done, err := j.Add(f)
			if err != nil {
				t.Fatal(err)
			}
			if done != (i == len(frames)-1) {
				t.Fatalf("frame %d of %d: done=%v", i, len(frames), done)
			}
			if done && string(out) != string(msg) {
				t.Fatalf("message changed: %d vs %d bytes", len(out), len(msg))
			}
		}
	}
	if len(protocol.WSProxySplit(big)) != 3 {
		t.Fatal("expected 3 frames")
	}
	var j protocol.WSProxyJoiner
	if _, _, err := j.Add([]byte{7, 'x'}); err == nil {
		t.Fatal("bad flag accepted")
	}
}
