package router

// B227: instead of pasting a long script, the user runs one command on the
// router. It downloads the install script from a link that works once.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

const (
	// installPrefix is the public path of the download. The code follows.
	installPrefix = "/router/install"
	// installTTL is how long a download code works.
	installTTL = 15 * time.Minute
	// installGone is the body of the 404. wget fails on it, so the sh behind
	// it in the one line command does not run.
	installGone = "安装链接失效了，回面板重新生成"
)

// installCode is the one live download code. Only the hash of the code is
// kept, the script is the one with the token filled in. Nothing is stored on
// disk, so a restart of the panel ends it.
type installCode struct {
	hash    [sha256.Size]byte
	script  string
	expires time.Time
}

// newInstall makes a download code for script and forgets the previous one.
// It returns the link and when the link stops working.
func (m *Module) newInstall(r *http.Request, script string) (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	code := hex.EncodeToString(raw)
	expires := m.now().Add(installTTL)
	m.mu.Lock()
	m.install = installCode{hash: sha256.Sum256([]byte(code)), script: script, expires: expires}
	m.mu.Unlock()
	return m.apiBase(r) + installPrefix + "/" + code, expires, nil
}

// takeInstall returns the script for code and ends the code. A wrong, used or
// expired code gets nothing.
func (m *Module) takeInstall(code string, now time.Time) (string, bool) {
	sum := sha256.Sum256([]byte(code))
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.install
	if c.script == "" || subtle.ConstantTimeCompare(sum[:], c.hash[:]) != 1 {
		return "", false
	}
	m.install = installCode{}
	if now.After(c.expires) {
		return "", false
	}
	return c.script, true
}

// installCommand is the line to run on the router.
func installCommand(installURL string) string {
	quoted := strings.ReplaceAll(installURL, "'", `'\''`)
	return "wget -qO /tmp/xc-install.sh '" + quoted + "' && sh /tmp/xc-install.sh"
}

// GetRouterInstall serves the install script once. The code is the proof, the
// router has no session.
func (m *Module) GetRouterInstall(w http.ResponseWriter, r *http.Request, code string) {
	script, ok := m.takeInstall(code, m.now())
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(installGone + "\n"))
		return
	}
	m.d.Audit.Record(r.Context(), "router.install.download", "", map[string]any{"ip": httpx.ClientIP(r, m.d.Config.TrustedProxies)}, nil)
	_, _ = w.Write([]byte(script))
}
