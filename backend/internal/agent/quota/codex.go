package quota

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Codex's own OAuth client and endpoints, as the Codex CLI uses them. Vars so
// tests can point them at a fake server.
var (
	codexClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexTokenURL = "https://auth.openai.com/oauth/token"
	codexBase     = "https://chatgpt.com/backend-api"
)

const (
	codexSignedOut = "Codex 登录已失效，请在这台机器上运行 codex login"
	// tokenMargin is how close to its end an access token is renewed.
	tokenMargin = 5 * time.Minute
)

// codexMu keeps two reads from renewing the same sign-in at once: the refresh
// token changes with every renewal, and a second use of the old one fails.
var codexMu sync.Mutex

type codexAuth struct {
	Tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		AccountID    string `json:"account_id"`
	} `json:"tokens"`
}

func readCodex(ctx context.Context, home string) (protocol.QuotaReading, error) {
	path := filepath.Join(home, "auth.json")
	auth, err := codexSignIn(ctx, path)
	if err != nil {
		return protocol.QuotaReading{}, err
	}
	claims := jwtClaims(auth.Tokens.IDToken)
	accountID := auth.Tokens.AccountID
	if accountID == "" {
		accountID = claimString(claims, "https://api.openai.com/auth", "chatgpt_account_id")
	}
	var data struct {
		PlanType string `json:"plan_type"`
		Credits  *struct {
			Has       bool   `json:"has_credits"`
			Unlimited bool   `json:"unlimited"`
			Balance   string `json:"balance"`
		} `json:"credits"`
		RateLimit struct {
			Primary   *codexWindow `json:"primary_window"`
			Secondary *codexWindow `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	headers := map[string]string{}
	if accountID != "" {
		headers["chatgpt-account-id"] = accountID
	}
	if err := getJSON(ctx, codexBase+"/wham/usage", auth.Tokens.AccessToken, headers, &data); err != nil {
		var se *statusError
		if errors.As(err, &se) && (se.status == http.StatusUnauthorized || se.status == http.StatusForbidden) {
			return protocol.QuotaReading{}, signedOut("%s", codexSignedOut)
		}
		if errors.As(err, &se) {
			return protocol.QuotaReading{}, unavailable("Codex 额度接口返回 %d", se.status)
		}
		return protocol.QuotaReading{}, unavailable("读取 Codex 额度失败：%v", err)
	}
	r := protocol.QuotaReading{Plan: data.PlanType, Windows: []protocol.QuotaWindow{}}
	r.User, _ = claims["email"].(string)
	if c := data.Credits; c != nil && c.Has && !c.Unlimited {
		if n, err := strconv.ParseFloat(strings.TrimSpace(c.Balance), 64); err == nil && n > 0 {
			r.Credits = strconv.FormatFloat(math.Round(n*100)/100, 'f', -1, 64) + " 积分"
		}
	}
	for _, w := range []*codexWindow{data.RateLimit.Primary, data.RateLimit.Secondary} {
		if w != nil {
			r.Windows = append(r.Windows, w.window())
		}
	}
	return r, nil
}

type codexWindow struct {
	UsedPercent     float64 `json:"used_percent"`
	LimitWindowSecs int64   `json:"limit_window_seconds"`
	ResetAt         int64   `json:"reset_at"`
	ResetAfterSecs  int64   `json:"reset_after_seconds"`
}

func (w codexWindow) window() protocol.QuotaWindow {
	out := protocol.QuotaWindow{Name: durationName(w.LimitWindowSecs), UsedPercent: w.UsedPercent, SpanSecs: w.LimitWindowSecs}
	switch {
	case w.ResetAt > 0:
		t := time.Unix(w.ResetAt, 0).UTC()
		out.ResetsAt = &t
	case w.ResetAfterSecs > 0:
		t := now().Add(time.Duration(w.ResetAfterSecs) * time.Second).UTC()
		out.ResetsAt = &t
	}
	return out
}

// codexSignIn returns the sign-in in path, renewed first when its access token
// is about to end. The renewed tokens are written back to the same file with
// every other field as the CLI wrote it, so the CLI is not signed out.
func codexSignIn(ctx context.Context, path string) (codexAuth, error) {
	codexMu.Lock()
	defer codexMu.Unlock()
	var a codexAuth
	if !readJSONFile(path, &a) || a.Tokens.AccessToken == "" {
		return a, signedOut("%s", codexSignedOut)
	}
	if !tokenEnding(a.Tokens.AccessToken) {
		return a, nil
	}
	if a.Tokens.RefreshToken == "" {
		return a, signedOut("%s", codexSignedOut)
	}
	var raw map[string]any
	if !readJSONFile(path, &raw) {
		return a, signedOut("%s", codexSignedOut)
	}
	fresh, err := codexRefresh(ctx, a.Tokens.RefreshToken)
	if err != nil {
		// The CLI may have renewed it between our read and our request. If
		// the file now holds a good token, that one stands.
		var b codexAuth
		if readJSONFile(path, &b) && b.Tokens.AccessToken != "" && !tokenEnding(b.Tokens.AccessToken) {
			return b, nil
		}
		return a, err
	}
	// read again just before writing, so a change made meanwhile is kept
	if !readJSONFile(path, &raw) {
		return a, unavailable("auth.json 在刷新期间不可读")
	}
	toks, _ := raw["tokens"].(map[string]any)
	if toks == nil {
		toks = map[string]any{}
	}
	toks["access_token"] = fresh.AccessToken
	if fresh.RefreshToken != "" {
		toks["refresh_token"] = fresh.RefreshToken
	}
	if fresh.IDToken != "" {
		toks["id_token"] = fresh.IDToken
	}
	raw["tokens"] = toks
	raw["last_refresh"] = now().UTC().Format(time.RFC3339Nano)
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return a, unavailable("auth.json 写回失败")
	}
	if err := writeFileAtomic(path, append(out, '\n')); err != nil {
		return a, unavailable("auth.json 写回失败：%v", err)
	}
	a.Tokens.AccessToken = fresh.AccessToken
	if fresh.RefreshToken != "" {
		a.Tokens.RefreshToken = fresh.RefreshToken
	}
	if fresh.IDToken != "" {
		a.Tokens.IDToken = fresh.IDToken
	}
	return a, nil
}

type codexTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func codexRefresh(ctx context.Context, refresh string) (codexTokens, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": codexClientID, "grant_type": "refresh_token",
		"refresh_token": refresh, "scope": "openid profile email",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenURL, bytes.NewReader(body))
	if err != nil {
		return codexTokens{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return codexTokens{}, unavailable("刷新 Codex 登录失败：%v", scrubURLError(err))
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized {
		return codexTokens{}, signedOut("%s", codexSignedOut)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return codexTokens{}, unavailable("刷新 Codex 登录失败，接口返回 %d", res.StatusCode)
	}
	var t codexTokens
	if json.Unmarshal(b, &t) != nil || t.AccessToken == "" {
		return codexTokens{}, unavailable("刷新 Codex 登录失败，返回内容不对")
	}
	return t, nil
}

// tokenEnding reports whether a JWT access token ends within tokenMargin. A
// token that says nothing of its end counts as good.
func tokenEnding(token string) bool {
	exp, _ := jwtClaims(token)["exp"].(float64)
	if exp == 0 {
		return false
	}
	return time.Until(time.Unix(int64(exp), 0)) <= tokenMargin
}

// jwtClaims reads the payload of a JWT without checking it. The agent only
// reads what the vendor put in its own token.
func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}

// claimString follows keys down nested objects and returns the string there.
func claimString(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = obj[k]
	}
	s, _ := cur.(string)
	return s
}

// writeFileAtomic replaces path with data through a temporary file in the same
// directory, mode 0600, so a reader never sees half a file.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".xc-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(name)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// CreateTemp makes the file 0600 already; Chmod is not supported everywhere.
	_ = tmp.Chmod(0o600)
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}
