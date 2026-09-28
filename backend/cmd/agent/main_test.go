package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/agent/config"
)

func TestRevokedAgentExitsThree(t *testing.T) {
	if os.Getenv("XC_TEST_REVOKED_AGENT") == "1" {
		os.Args = []string{"x-console-agent", "run", "--config", os.Getenv("XC_TEST_AGENT_CONFIG")}
		main()
		return
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "agent.json")
	if err := config.Save(path, config.Config{Server: srv.URL, AgentID: "revoked", Token: "invalid"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRevokedAgentExitsThree$")
	cmd.Env = append(os.Environ(), "XC_TEST_REVOKED_AGENT=1", "XC_TEST_AGENT_CONFIG="+path)
	err := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 {
		t.Fatalf("exit code = %v, want 3", err)
	}
}
