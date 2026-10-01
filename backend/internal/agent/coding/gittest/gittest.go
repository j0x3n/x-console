// Package gittest serves bare repositories over HTTP with git http-backend
// and basic auth, for tests of cloning and pushing with a token (B47).
package gittest

import (
	"encoding/base64"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// Server serves the bare repositories under root at /<name>.git. Requests
// must carry basic auth user:token. Auth records every Authorization header.
type Server struct {
	*httptest.Server
	mu   sync.Mutex
	Auth []string
}

// New starts the server, or skips the test when git http-backend is missing.
func New(t testing.TB, root, user, token string) *Server {
	t.Helper()
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skip("git not installed")
	}
	backend := filepath.Join(strings.TrimSpace(string(out)), "git-http-backend")
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+token))
	s := &Server{}
	h := &cgi.Handler{Path: backend, Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=" + user}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		s.mu.Lock()
		s.Auth = append(s.Auth, got)
		s.mu.Unlock()
		if got != want {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}
