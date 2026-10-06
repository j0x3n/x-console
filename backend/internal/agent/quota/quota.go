// Package quota reads how much of an AI subscription is used, for the agent:
// Claude Code, Codex and Grok, each signed in in a directory of its own so one
// machine can hold several accounts. See docs/specs/B110.md.
//
// The tokens never leave this package. Codex's sign-in is renewed in place in
// its own auth.json, as the Codex CLI does, so the CLI keeps working.
package quota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/rpcutil"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
	"github.com/j0x3n/x-console/backend/pkg/rpc"
)

var (
	lookPath   = exec.LookPath
	httpClient = &http.Client{Timeout: 20 * time.Second}
	now        = time.Now
)

// Registrar is the part of conn.Client the handlers need.
type Registrar interface {
	Handle(method string, h rpc.Handler)
}

// Register adds quota.read.
func Register(c Registrar) {
	c.Handle(protocol.MethodQuotaRead, func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p protocol.QuotaReadParams
		if err := rpcutil.Decode(raw, &p); err != nil {
			return nil, err
		}
		return Read(ctx, p)
	})
}

// Available reports whether this machine has Claude Code, Codex or Grok on
// the PATH, or a sign-in directory of one. cmd/agent announces
// protocol.CapQuota only then.
func Available() bool {
	for _, kind := range kinds {
		if _, err := lookPath(kind); err == nil {
			return true
		}
		if dir, err := defaultHome(kind); err == nil {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				return true
			}
		}
	}
	return false
}

var kinds = []string{protocol.QuotaKindClaude, protocol.QuotaKindCodex, protocol.QuotaKindGrok}

// Read answers quota.read.
func Read(ctx context.Context, p protocol.QuotaReadParams) (protocol.QuotaReading, error) {
	home, err := resolveHome(p.Kind, p.Home)
	if err != nil {
		return protocol.QuotaReading{}, err
	}
	var r protocol.QuotaReading
	switch p.Kind {
	case protocol.QuotaKindCodex:
		r, err = readCodex(ctx, home)
	case protocol.QuotaKindGrok:
		r, err = readGrok(ctx, home)
	case protocol.QuotaKindClaude:
		r, err = readClaude(ctx, p.Home == "", home)
	}
	if r.Windows == nil {
		r.Windows = []protocol.QuotaWindow{}
	}
	return r, err
}

// defaultHome is where the CLI keeps its sign-in when nothing says otherwise.
func defaultHome(kind string) (string, error) {
	env, dot := "", ""
	switch kind {
	case protocol.QuotaKindClaude:
		env, dot = "CLAUDE_CONFIG_DIR", ".claude"
	case protocol.QuotaKindCodex:
		env, dot = "CODEX_HOME", ".codex"
	case protocol.QuotaKindGrok:
		env, dot = "GROK_HOME", ".grok"
	default:
		return "", rpcutil.BadParams("unknown kind %q", kind)
	}
	if v := strings.TrimSpace(os.Getenv(env)); v != "" {
		return v, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", rpcutil.Failed("cannot find the home directory: %v", err)
	}
	return filepath.Join(h, dot), nil
}

// resolveHome checks the directory the server asked for. It must be absolute
// (or start with "~/") and hold no ".." part, so a request cannot point the
// agent at somewhere it was not meant to look.
func resolveHome(kind, home string) (string, error) {
	if kind != protocol.QuotaKindClaude && kind != protocol.QuotaKindCodex && kind != protocol.QuotaKindGrok {
		return "", rpcutil.BadParams("unknown kind %q", kind)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return defaultHome(kind)
	}
	if home == "~" || strings.HasPrefix(home, "~/") || strings.HasPrefix(home, `~\`) {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", rpcutil.Failed("cannot find the home directory: %v", err)
		}
		home = filepath.Join(h, home[1:])
	}
	for _, part := range strings.FieldsFunc(home, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", rpcutil.BadParams("home must not contain \"..\"")
		}
	}
	if !filepath.IsAbs(home) {
		return "", rpcutil.BadParams("home must be an absolute path")
	}
	return filepath.Clean(home), nil
}

func signedOut(format string, args ...any) error {
	return &protocol.Error{Code: protocol.CodeQuotaSignedOut, Message: fmt.Sprintf(format, args...)}
}

func unavailable(format string, args ...any) error {
	return &protocol.Error{Code: protocol.CodeQuotaUnavailable, Message: fmt.Sprintf(format, args...)}
}

type statusError struct{ status int }

func (e *statusError) Error() string { return http.StatusText(e.status) }

// getJSON asks url with a bearer token and decodes the answer into dst. A
// vendor refusal comes back as *statusError.
func getJSON(ctx context.Context, url, token string, headers map[string]string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return scrubURLError(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &statusError{status: res.StatusCode}
	}
	return json.Unmarshal(b, dst)
}

// scrubURLError keeps the reason of a failed request and drops the URL,
// which net/http puts in the message.
func scrubURLError(err error) error {
	var ue interface{ Unwrap() error }
	if errors.As(err, &ue) && ue.Unwrap() != nil {
		return ue.Unwrap()
	}
	return err
}

// readJSONFile reads a small JSON file.
func readJSONFile(path string, dst any) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(b, dst) == nil
}

func durationName(secs int64) string {
	switch {
	case secs <= 0:
		return "额度"
	case secs%86400 == 0:
		return strconv.FormatInt(secs/86400, 10) + " 天"
	case secs%3600 == 0:
		return strconv.FormatInt(secs/3600, 10) + " 小时"
	default:
		return strconv.FormatInt((secs+59)/60, 10) + " 分钟"
	}
}
