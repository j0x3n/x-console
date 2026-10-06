package quota

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// grokBase is the Grok CLI's backend. A var so tests can point it elsewhere.
var grokBase = "https://cli-chat-proxy.grok.com/v1"

const grokSignedOut = "Grok 登录已失效，请在这台机器上运行 grok login"

// grokRefresh has the Grok CLI renew its own sign-in, which it keeps in home.
// The agent never writes that file: the CLI's token changes each time.
var grokRefresh = realGrokRefresh

type grokCredential struct {
	Key       string    `json:"key"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}

// readGrokCredential reads the sign-in the CLI keeps in home: the first of its
// entries, by name, that holds a key.
func readGrokCredential(home string) (grokCredential, bool) {
	var all map[string]grokCredential
	if !readJSONFile(filepath.Join(home, "auth.json"), &all) {
		return grokCredential{}, false
	}
	names := make([]string, 0, len(all))
	for k := range all {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if all[k].Key != "" {
			return all[k], true
		}
	}
	return grokCredential{}, false
}

func readGrok(ctx context.Context, home string) (protocol.QuotaReading, error) {
	c, ok := readGrokCredential(home)
	if !ok {
		return protocol.QuotaReading{}, signedOut("%s", grokSignedOut)
	}
	if !c.ExpiresAt.IsZero() && time.Until(c.ExpiresAt) < tokenMargin {
		grokRefresh(ctx, home)
		if c, ok = readGrokCredential(home); !ok {
			return protocol.QuotaReading{}, signedOut("%s", grokSignedOut)
		}
		if !c.ExpiresAt.IsZero() && time.Until(c.ExpiresAt) <= 0 {
			return protocol.QuotaReading{User: c.Email}, signedOut("%s", grokSignedOut)
		}
	}
	var data struct {
		Config struct {
			CreditUsagePercent float64 `json:"creditUsagePercent"`
			CurrentPeriod      *struct {
				Type string `json:"type"`
				End  string `json:"end"`
			} `json:"currentPeriod"`
			BillingPeriodEnd string `json:"billingPeriodEnd"`
			OnDemandCap      struct {
				Val float64 `json:"val"`
			} `json:"onDemandCap"`
			OnDemandUsed struct {
				Val float64 `json:"val"`
			} `json:"onDemandUsed"`
		} `json:"config"`
	}
	r := protocol.QuotaReading{User: c.Email, Windows: []protocol.QuotaWindow{}}
	if err := getJSON(ctx, grokBase+"/billing?format=credits", c.Key, nil, &data); err != nil {
		var se *statusError
		if errors.As(err, &se) && (se.status == http.StatusUnauthorized || se.status == http.StatusForbidden) {
			return r, signedOut("%s", grokSignedOut)
		}
		if errors.As(err, &se) {
			return r, unavailable("Grok 额度接口返回 %d", se.status)
		}
		return r, unavailable("读取 Grok 额度失败：%v", err)
	}
	cfg := data.Config
	name, span, end := "额度", int64(0), cfg.BillingPeriodEnd
	if p := cfg.CurrentPeriod; p != nil {
		name, span = grokPeriod(p.Type)
		end = p.End
	}
	w := protocol.QuotaWindow{Name: name, UsedPercent: cfg.CreditUsagePercent, SpanSecs: span}
	if t, err := time.Parse(time.RFC3339Nano, end); err == nil {
		t = t.UTC()
		w.ResetsAt = &t
	}
	r.Windows = append(r.Windows, w)
	if cfg.OnDemandCap.Val > 0 {
		r.Windows = append(r.Windows, protocol.QuotaWindow{
			Name: "按量付费", UsedPercent: 100 * cfg.OnDemandUsed.Val / cfg.OnDemandCap.Val,
			ResetsAt: w.ResetsAt, Aside: true,
		})
	}
	return r, nil
}

// grokPeriod names a USAGE_PERIOD_TYPE_* and says how long it runs.
func grokPeriod(t string) (string, int64) {
	switch strings.TrimPrefix(t, "USAGE_PERIOD_TYPE_") {
	case "DAILY":
		return "今日", 86400
	case "WEEKLY":
		return "7 天", 7 * 86400
	case "MONTHLY":
		return "本月", 30 * 86400
	}
	return "额度", 0
}

func realGrokRefresh(ctx context.Context, home string) {
	binary, err := lookPath("grok")
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := commandContext(ctx, binary, "models")
	cmd.Dir = filepath.Dir(home)
	cmd.Env = withEnv(os.Environ(), []string{"GROK_HOME", "GROK_AUTH_PROVIDER_COMMAND", "GROK_AUTH_EXPIRED"}, "GROK_HOME="+home)
	_ = cmd.Run()
}
