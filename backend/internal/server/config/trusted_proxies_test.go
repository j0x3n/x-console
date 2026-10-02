package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestTrustedProxiesEnvironment(t *testing.T) {
	t.Setenv("XC_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("XC_TRUSTED_PROXIES", "127.0.0.1,2001:db8::/32")
	cfg, err := FromEnv()
	if err != nil || len(cfg.TrustedProxies) != 2 {
		t.Fatalf("trusted proxies %+v %v", cfg.TrustedProxies, err)
	}
	t.Setenv("XC_TRUSTED_PROXIES", "invalid")
	if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), "XC_TRUSTED_PROXIES") {
		t.Fatalf("invalid configuration %v", err)
	}
	t.Setenv("XC_TRUSTED_PROXIES", "")
	cfg, err = FromEnv()
	if err != nil || len(cfg.TrustedProxies) != 0 {
		t.Fatalf("default proxies %+v %v", cfg.TrustedProxies, err)
	}
}
