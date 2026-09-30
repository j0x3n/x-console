package core

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/core/agentinstall"
	"github.com/j0x3n/x-console/backend/internal/server/core/api"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
)

// The public entrances of B30: install scripts and the agent programs. All
// of them answer 404, and nothing more, to a pairing code that is not valid,
// and are limited to 10 requests a minute per address.
const (
	distRate   = 10
	distWindow = time.Minute
)

var errNoAgents = httpx.NewError(http.StatusNotFound, "not_found", "这个面板没有打包代理")

// distState is the memory of the public entrances: request counts per address
// and the checksums of the agent files.
type distState struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	sums map[string]fileSum
}

type fileSum struct {
	size int64
	mod  time.Time
	sum  string
}

// allow counts a request and reports whether the address is still under the limit.
func (d *distState) allow(key string, now time.Time) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.hits == nil {
		d.hits = map[string][]time.Time{}
	}
	kept := d.hits[key][:0]
	for _, t := range d.hits[key] {
		if now.Sub(t) < distWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) >= distRate {
		d.hits[key] = kept
		return false
	}
	d.hits[key] = append(kept, now)
	if len(d.hits) > 10000 { // addresses that stopped asking
		for k, v := range d.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) >= distWindow {
				delete(d.hits, k)
			}
		}
	}
	return true
}

// checksum returns the SHA-256 of a file, computed again when it changes.
func (d *distState) checksum(path string, st os.FileInfo) (string, error) {
	d.mu.Lock()
	hit, ok := d.sums[path]
	d.mu.Unlock()
	if ok && hit.size == st.Size() && hit.mod.Equal(st.ModTime()) {
		return hit.sum, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	d.mu.Lock()
	if d.sums == nil {
		d.sums = map[string]fileSum{}
	}
	d.sums[path] = fileSum{st.Size(), st.ModTime(), sum}
	d.mu.Unlock()
	return sum, nil
}

func clientIP(r *http.Request) string {
	// Caddy sets X-Forwarded-For; the server only listens on localhost behind it.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// panelURL is the address the panel is reached at: XC_PUBLIC_URL, or what the
// request says. It goes into scripts, so it is checked.
func (h *Handlers) panelURL(r *http.Request) (string, error) {
	server := h.PublicURL
	if server == "" {
		scheme := "http"
		if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" || (proto == "" && r.TLS != nil) {
			scheme = "https"
		}
		server = scheme + "://" + r.Host
	}
	if err := agentinstall.CheckServer(server); err != nil {
		return "", httpx.Invalid("面板地址不正确，请设置 XC_PUBLIC_URL")
	}
	return server, nil
}

// normalCode turns what the user typed into the shape scripts and file names
// use. It returns "" for anything that cannot be a code.
func normalCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) == 8 {
		code = code[:4] + "-" + code[4:]
	}
	if !agentinstall.CodeRe.MatchString(code) {
		return ""
	}
	return code
}

// checkCode applies the limit and the code check of a public entrance. Every
// failure is the same 404.
func (h *Handlers) checkCode(w http.ResponseWriter, r *http.Request, code string) (string, bool) {
	if !h.dist.allow(clientIP(r), time.Now()) {
		httpx.Fail(w, r, httpx.ErrTooManyRequests)
		return "", false
	}
	code = normalCode(code)
	if code != "" {
		ok, err := h.Agents.PairingCodeValid(r.Context(), code)
		if err != nil {
			httpx.Fail(w, r, err)
			return "", false
		}
		if ok {
			return code, true
		}
	}
	httpx.Fail(w, r, httpx.ErrNotFound)
	return "", false
}

func (h *Handlers) sendScript(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// GetAgentInstallScript is GET /agent/install.sh.
func (h *Handlers) GetAgentInstallScript(w http.ResponseWriter, r *http.Request, params api.GetAgentInstallScriptParams) {
	code, ok := h.checkCode(w, r, params.Code)
	if !ok {
		return
	}
	server, err := h.panelURL(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	script, err := agentinstall.Script(server, code)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sendScript(w, "text/x-shellscript; charset=utf-8", script)
}

// GetAgentInstallPowerShell is GET /agent/install.ps1.
func (h *Handlers) GetAgentInstallPowerShell(w http.ResponseWriter, r *http.Request, params api.GetAgentInstallPowerShellParams) {
	code, ok := h.checkCode(w, r, params.Code)
	if !ok {
		return
	}
	server, err := h.panelURL(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	script, err := agentinstall.PowerShell(server, code)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sendScript(w, "text/plain; charset=utf-8", script)
}

// GetAgentUninstallScript is GET /agent/uninstall.sh.
func (h *Handlers) GetAgentUninstallScript(w http.ResponseWriter, r *http.Request) {
	h.sendScript(w, "text/x-shellscript; charset=utf-8", agentinstall.Uninstall())
}

// agentFile finds the packaged agent of a platform.
func (h *Handlers) agentFile(goos, arch string) (string, os.FileInfo, error) {
	if (goos != "linux" && goos != "windows") || (arch != "amd64" && arch != "arm64") {
		return "", nil, httpx.ErrNotFound
	}
	name := "x-console-agent"
	if goos == "windows" {
		name += ".exe"
	}
	path := filepath.Join(h.AgentsDir, goos+"-"+arch, name)
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return "", nil, errNoAgents
	}
	return path, st, nil
}

// DownloadAgent is GET /agent/download/{os}/{arch}. A valid pairing code or a
// login lets it through.
func (h *Handlers) DownloadAgent(w http.ResponseWriter, r *http.Request, goos api.DownloadAgentParamsOs, arch api.DownloadAgentParamsArch, params api.DownloadAgentParams) {
	if auth.FromContext(r.Context()) == nil {
		code := ""
		if params.Code != nil {
			code = *params.Code
		}
		if _, ok := h.checkCode(w, r, code); !ok {
			return
		}
	}
	path, st, err := h.agentFile(string(goos), string(arch))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	sum, err := h.dist.checksum(path, st)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sendFile(w, path, st, filepath.Base(path), sum, nil)
}

// DownloadAgentSetup is GET /agent/setup.exe: the Windows agent with the panel
// address added to its end and the pairing code in its name.
func (h *Handlers) DownloadAgentSetup(w http.ResponseWriter, r *http.Request, params api.DownloadAgentSetupParams) {
	code, ok := h.checkCode(w, r, params.Code)
	if !ok {
		return
	}
	server, err := h.panelURL(r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	trailer, err := agentinstall.SetupTrailer(server)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	path, st, err := h.agentFile("windows", "amd64")
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h.sendFile(w, path, st, "x-console-agent-setup-"+code+".exe", "", trailer)
}

// sendFile streams a file followed by trailer. sum is set in the header when
// the bytes are exactly the file.
func (h *Handlers) sendFile(w http.ResponseWriter, path string, st os.FileInfo, name, sum string, trailer []byte) {
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Cache-Control", "no-store")
	if sum != "" {
		w.Header().Set("X-Checksum-Sha256", sum)
	}
	w.Header().Set("Content-Length", fmt.Sprint(st.Size()+int64(len(trailer))))
	if _, err := io.Copy(w, io.LimitReader(f, st.Size())); err != nil && !errors.Is(err, io.EOF) {
		return
	}
	_, _ = w.Write(trailer)
}
