package sysinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
)

func TestObservedAddressAuthenticatedAndOldServer(t *testing.T) {
	var old atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/whoami" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if old.Load() {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, `{"ip":"8.8.8.8"}`)
	}))
	defer server.Close()
	observedAddress.Lock()
	original := observedAddress.ip
	observedAddress.ip = ""
	observedAddress.Unlock()
	defer func() { observedAddress.Lock(); observedAddress.ip = original; observedAddress.Unlock() }()
	refreshObserved(context.Background(), server.URL, "test-token")
	info, err := SystemInfo(context.Background(), json.RawMessage{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(info)
	var result struct{ Addresses []struct{ IP string } }
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range result.Addresses {
		if a.IP == "8.8.8.8" {
			found = true
		}
	}
	if !found {
		t.Fatalf("observed IP absent %s", raw)
	}
	old.Store(true)
	refreshObserved(context.Background(), server.URL, "test-token")
	observedAddress.RLock()
	ip := observedAddress.ip
	observedAddress.RUnlock()
	if ip != "8.8.8.8" {
		t.Fatal("old server discarded existing address")
	}
}

func TestPublicAddressReservedRanges(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "203.0.113.1", "2001:db8::1"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Fatalf("reserved public %s", raw)
		}
	}
	if !publicAddress(netip.MustParseAddr("8.8.8.8")) || !publicAddress(netip.MustParseAddr("2001:4860::8888")) {
		t.Fatal("public address rejected")
	}
}
