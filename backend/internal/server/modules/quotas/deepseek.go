package quotas

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas/db"
)

// fetchDeepSeek reads the balance of a DeepSeek API key. The key is opened
// here and goes nowhere but the Authorization header of this request.
func (m *Module) fetchDeepSeek(ctx context.Context, a db.QuotaAccount) (result, error) {
	key, err := m.d.Secrets.Open(a.ApiKey)
	if err != nil {
		return result{}, &readError{code: codeUnavailable, msg: "API Key 无法解密，请重新填写"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.deepseekURL, nil)
	if err != nil {
		return result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result{}, ctx.Err()
		}
		return result{}, &readError{code: codeUnavailable, msg: "连不上 DeepSeek，请检查服务器的网络"}
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return result{}, &readError{code: codeSignedOut, msg: "DeepSeek 说 API Key 无效，可能已被删除，请重新填写"}
	case res.StatusCode < 200 || res.StatusCode >= 300:
		return result{}, &readError{code: codeUnavailable, msg: fmt.Sprintf("DeepSeek 余额接口返回 %d", res.StatusCode)}
	}
	bal, err := parseDeepSeek(b)
	if err != nil {
		return result{}, &readError{code: codeUnavailable, msg: err.Error()}
	}
	return result{windows: []api.QuotaWindow{}, balances: bal}, nil
}

// parseDeepSeek reads {"is_available":true,"balance_infos":[{"currency":"CNY",
// "total_balance":"110.00",...}]}.
func parseDeepSeek(b []byte) ([]api.QuotaBalance, error) {
	var r struct {
		Infos []struct {
			Currency string `json:"currency"`
			Total    any    `json:"total_balance"`
		} `json:"balance_infos"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("DeepSeek 返回的内容读不懂")
	}
	out := []api.QuotaBalance{}
	for _, in := range r.Infos {
		amount := ""
		switch v := in.Total.(type) {
		case string:
			amount = strings.TrimSpace(v)
		case float64:
			amount = strconv.FormatFloat(v, 'f', -1, 64)
		}
		if _, err := strconv.ParseFloat(amount, 64); err != nil || in.Currency == "" {
			continue
		}
		out = append(out, api.QuotaBalance{Currency: in.Currency, Amount: amount})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("DeepSeek 的返回里没有余额")
	}
	return out, nil
}
