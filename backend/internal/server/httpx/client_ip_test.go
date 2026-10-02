package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestTrustedClientIP(t *testing.T) {
	trusted, err := ParseTrustedProxies("127.0.0.1/32,::1/128")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ remote, header, want string }{{"192.0.2.1:42", "8.8.8.8", "192.0.2.1"}, {"127.0.0.1:42", "bad, 8.8.8.8, 1.1.1.1", "1.1.1.1"}, {"127.0.0.1:42", "6.6.6.6, 8.8.8.8", "8.8.8.8"}, {"127.0.0.1:42", "8.8.8.8, 127.0.0.1", "8.8.8.8"}, {"127.0.0.1:42", "8.8.8.8, bad, 127.0.0.1", "127.0.0.1"}, {"[::1]:42", "::ffff:8.8.8.8", "8.8.8.8"}, {"127.0.0.1:42", "bad", "127.0.0.1"}, {"unparseable", "8.8.8.8", ""}} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		r.Header.Set("X-Forwarded-For", tc.header)
		if got := ClientIP(r, trusted); got != tc.want {
			t.Fatalf("%s %s: %s", tc.remote, tc.header, got)
		}
	}
	if _, err := ParseTrustedProxies("garbage"); err == nil {
		t.Fatal("invalid trusted proxy accepted")
	}
}
