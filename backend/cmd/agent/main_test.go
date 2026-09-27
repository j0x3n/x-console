package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/config"
)

func TestRevokedAgentExitCode(t *testing.T) {
	if os.Getenv("XC_REVOKED_TEST_HELPER") == "1" {
		os.Args = []string{"x-console-agent", "run", "--config", os.Getenv("XC_REVOKED_TEST_CONFIG")}
		main()
		return
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.Save(path, config.Config{Server: srv.URL, AgentID: "revoked", Token: "revoked"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRevokedAgentExitCode$")
	cmd.Env = append(os.Environ(), "XC_REVOKED_TEST_HELPER=1", "XC_REVOKED_TEST_CONFIG="+path)
	if err := cmd.Run(); err == nil {
		t.Fatal("agent exited successfully after 401")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
		t.Fatalf("agent exit: %v", err)
	}
}
