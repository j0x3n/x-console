package credentials_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials"
	"github.com/j0x3n/x-console/backend/internal/server/modules/credentials/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// clockDay is the fixed "today" of the tests: 2026-10-10 in Asia/Shanghai.
var clockDay = time.Date(2026, 10, 10, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

func setup(t *testing.T) (*testutil.Env, *credentials.Module) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*credentials.Module](env.App.Deps.Registry, credentials.ServiceKey)
	if !ok {
		t.Fatal("credentials module not registered")
	}
	credentials.SetNow(m, func() time.Time { return clockDay })
	return env, m
}

type listOut struct {
	Items   []api.Credential      `json:"items"`
	Summary api.CredentialSummary `json:"summary"`
}

func create(t *testing.T, env *testutil.Env, body map[string]any) api.Credential {
	t.Helper()
	var c api.Credential
	env.MustDo(http.MethodPost, "/credentials", body, &c)
	return c
}

func days(n int) string { return clockDay.AddDate(0, 0, n).Format("2006-01-02") }

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type notice struct{ Kind, Title, Body, Link string }

func notices(t *testing.T, env *testutil.Env, kind string) []notice {
	t.Helper()
	var out struct{ Items []notice }
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == kind {
			got = append(got, n)
		}
	}
	return got
}

func TestCreateListSortAndStatus(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "ssh_key", "name": "没有日期的密钥"})
	create(t, env, map[string]any{"kind": "api_key", "name": "OpenAI 主密钥", "platform": "OpenAI", "account": "me@example.com",
		"usedBy": []string{"服务器 hk-1", "项目 x-console", "服务器 hk-1", " "}, "scopes": "chat", "hint": "a1b2", "createdOn": days(-30), "expiresOn": days(200)})
	create(t, env, map[string]any{"kind": "access_token", "name": "GitHub 部署", "platform": "GitHub", "expiresOn": days(20)})
	create(t, env, map[string]any{"kind": "access_token", "name": "旧令牌", "expiresOn": days(-3)})
	create(t, env, map[string]any{"kind": "signing_key", "name": "久未更换", "createdOn": days(-400), "rotatedOn": days(-100), "rotateEveryDays": 90})
	create(t, env, map[string]any{"kind": "api_key", "name": "刚更换", "rotatedOn": days(-10), "rotateEveryDays": 90})

	var out listOut
	env.MustDo(http.MethodGet, "/credentials", nil, &out)
	var names []string
	for _, c := range out.Items {
		names = append(names, c.Name)
	}
	if got := strings.Join(names, ","); got != "久未更换,旧令牌,GitHub 部署,刚更换,OpenAI 主密钥,没有日期的密钥" {
		t.Fatalf("order: %s", got)
	}
	want := map[string]api.CredentialStatus{"旧令牌": api.Expired, "久未更换": api.Stale, "GitHub 部署": api.Soon, "刚更换": api.Ok, "OpenAI 主密钥": api.Ok, "没有日期的密钥": api.None}
	for _, c := range out.Items {
		if c.Status != want[c.Name] {
			t.Errorf("%s status = %s, want %s", c.Name, c.Status, want[c.Name])
		}
	}
	stale := out.Items[0]
	if stale.RotateDueIn == nil || *stale.RotateDueIn != -10 || stale.ExpiresIn != nil {
		t.Fatalf("stale due: %+v", stale)
	}
	main := out.Items[4]
	if len(main.UsedBy) != 2 || main.UsedBy[0] != "服务器 hk-1" || main.Hint != "a1b2" || main.ExpiresIn == nil || *main.ExpiresIn != 200 {
		t.Fatalf("fields: %+v", main)
	}
	if got := main.RemindDays; len(got) != 2 || got[0] != 30 || got[1] != 7 {
		t.Fatalf("default remind days: %v", got)
	}
	if out.Summary.Total != 6 || out.Summary.Expired != 1 || out.Summary.Soon != 1 || out.Summary.Stale != 1 || out.Summary.None != 1 {
		t.Fatalf("summary: %+v", out.Summary)
	}

	// 筛选和搜索只改列表，不改概要
	env.MustDo(http.MethodGet, "/credentials?kind=signing_key", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Name != "久未更换" || out.Summary.Total != 6 {
		t.Fatalf("kind filter: %+v %+v", out.Items, out.Summary)
	}
	// 按“用在哪里”找
	env.MustDo(http.MethodGet, "/credentials?q=HK-1", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Name != "OpenAI 主密钥" {
		t.Fatalf("q by usedBy: %+v", out.Items)
	}
	env.MustDo(http.MethodGet, "/credentials?q=github", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Name != "GitHub 部署" {
		t.Fatalf("q by platform: %+v", out.Items)
	}
}

func TestValidation(t *testing.T) {
	env, _ := setup(t)
	bad := []map[string]any{
		{"kind": "api_key", "name": "  "},
		{"kind": "nope", "name": "x"},
		{"kind": "api_key", "name": strings.Repeat("长", 101)},
		{"kind": "api_key", "name": "x", "hint": strings.Repeat("a", 17)},
		{"kind": "api_key", "name": "x", "expiresOn": "2026/10/10"},
		{"kind": "api_key", "name": "x", "createdOn": "2026-10-10", "expiresOn": "2026-10-01"},
		{"kind": "api_key", "name": "x", "createdOn": "2026-10-10", "rotatedOn": "2026-10-01"},
		{"kind": "api_key", "name": "x", "rotateEveryDays": -1},
		{"kind": "api_key", "name": "x", "rotateEveryDays": 90}, // 没有创建日期和更换日期
		{"kind": "api_key", "name": "x", "rotateEveryDays": 4000, "createdOn": "2026-01-01"},
		{"kind": "api_key", "name": "x", "remindDays": []int{0}},
		{"kind": "api_key", "name": "x", "remindDays": []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"kind": "api_key", "name": "x", "usedBy": make([]string, 0)},
	}
	bad = bad[:len(bad)-1] // 最后一项是合法的空数组，单独放在下面
	for i, body := range bad {
		if status, raw := env.Do(http.MethodPost, "/credentials", body, nil); status != http.StatusBadRequest {
			t.Errorf("case %d %v: status %d %s", i, body, status, raw)
		}
	}
	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = "服务器 " + itoa(int64(i))
	}
	if status, _ := env.Do(http.MethodPost, "/credentials", map[string]any{"kind": "api_key", "name": "x", "usedBy": tooMany}, nil); status != http.StatusBadRequest {
		t.Errorf("21 places: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/credentials?kind=nope", nil, nil); status != http.StatusBadRequest {
		t.Errorf("kind filter: %d", status)
	}
	var out listOut
	env.MustDo(http.MethodGet, "/credentials", nil, &out)
	if len(out.Items) != 0 {
		t.Fatalf("a refused request created something: %+v", out.Items)
	}
}

func TestRefusesWhatLooksLikeASecret(t *testing.T) {
	env, _ := setup(t)
	for _, s := range []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----",
		"sk-proj-abcdefghijklmnopqrstuv",
		"ghp_" + strings.Repeat("aB3", 12),
		"github_pat_11ABCDEFG0123456789_abcdefghij",
		"xc_" + strings.Repeat("0a", 16),
		"AKIAIOSFODNN7EXAMPLE",
		"xoxb-123456789012-abcdefghijkl",
		"AIza" + strings.Repeat("Ab1_", 9),
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc",
		"Zm9vYmFyQmF6MTIzNDU2Nzg5MGFiY2RlZkdISUo=",
	} {
		for field, body := range map[string]map[string]any{
			"name":   {"kind": "api_key", "name": s},
			"notes":  {"kind": "api_key", "name": "ok", "notes": "token 是 " + s + " 这个"},
			"usedBy": {"kind": "api_key", "name": "ok", "usedBy": []string{s}},
			"hint":   {"kind": "api_key", "name": "ok", "hint": s[:min(len(s), 16)] + "x"},
		} {
			status, raw := env.Do(http.MethodPost, "/credentials", body, nil)
			if field == "hint" {
				continue // 尾号太长本身就会被拒绝，上面的校验已覆盖
			}
			if status != http.StatusBadRequest || !strings.Contains(string(raw), "密钥本身") {
				t.Errorf("%s %q: status %d %s", field, s, status, raw)
			}
		}
	}
	// 更新也要拒绝
	c := create(t, env, map[string]any{"kind": "api_key", "name": "正常"})
	if status, _ := env.Do(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"notes": "ghp_" + strings.Repeat("aB3", 12)}, nil); status != http.StatusBadRequest {
		t.Fatalf("patch: %d", status)
	}
	var raw string
	env.App.Deps.DB.QueryRow("SELECT count(*) || group_concat(notes) FROM credentials").Scan(&raw)
	if strings.Contains(raw, "ghp_") {
		t.Fatalf("a secret reached the database: %s", raw)
	}
}

func TestLooksLikeSecretAllowsOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"GitHub 部署令牌", "prod-api-gateway-eu-west-1-key-2026-10", "123e4567-e89b-12d3-a456-426614174000",
		"SHA256:uNiVzdSG8Ui5S2Rg6Ia0eT9mJd7pQwXyZ1AbCdEfGhI", "md5:aa:bb:cc:dd:ee:ff:00:11:22:33", "尾号 a1b2", "https://platform.openai.com/api-keys",
		"/home/deploy/.ssh/id_ed25519_github_actions_2026", "repo, workflow, read:org", "",
	} {
		if credentials.LooksLikeSecret(s) {
			t.Errorf("%q refused", s)
		}
	}
}

func TestArchiveHidesFromList(t *testing.T) {
	env, _ := setup(t)
	c := create(t, env, map[string]any{"kind": "api_key", "name": "k", "expiresOn": days(10)})
	var u api.Credential
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"archived": true}, &u)
	if !u.Archived {
		t.Fatal("not archived")
	}
	var out listOut
	env.MustDo(http.MethodGet, "/credentials", nil, &out)
	if len(out.Items) != 0 || out.Summary.Total != 0 {
		t.Fatalf("archived shown: %+v", out)
	}
	env.MustDo(http.MethodGet, "/credentials?archived=true", nil, &out)
	if len(out.Items) != 1 {
		t.Fatalf("archived=true: %+v", out)
	}
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"archived": false}, &u)
	if u.Archived {
		t.Fatal("still archived")
	}
}

func TestDeleteNeedsElevation(t *testing.T) {
	env, _ := setup(t)
	c := create(t, env, map[string]any{"kind": "other", "name": "x"})
	if status, _ := env.Do(http.MethodDelete, "/credentials/"+itoa(c.Id), nil, nil); status != 403 {
		t.Fatalf("delete without elevation: %d", status)
	}
	env.Elevate()
	if status, raw := env.Do(http.MethodDelete, "/credentials/"+itoa(c.Id), nil, nil); status != 204 {
		t.Fatalf("delete: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodGet, "/credentials/"+itoa(c.Id), nil, nil); status != 404 {
		t.Fatalf("after delete: %d", status)
	}
}

func TestExpiryReminderFiresOnce(t *testing.T) {
	env, m := setup(t)
	create(t, env, map[string]any{"kind": "access_token", "name": "部署", "platform": "GitHub", "hint": "z9y8",
		"usedBy": []string{"服务器 hk-1", "服务器 hk-2", "服务器 hk-3", "项目 x-console"}, "expiresOn": days(20)})
	got := notices(t, env, "credentials.expiring")
	if len(got) != 1 {
		t.Fatalf("20 days left: want 1 notification, got %+v", got)
	}
	if got[0].Title != "访问令牌 GitHub「部署」还有 20 天到期" || got[0].Link != "/credentials" {
		t.Fatalf("notification: %+v", got[0])
	}
	if strings.Contains(got[0].Body, "z9y8") || !strings.Contains(got[0].Body, days(20)) || !strings.Contains(got[0].Body, "用在 服务器 hk-1、服务器 hk-2、服务器 hk-3 等 4 处") {
		t.Fatalf("body: %q", got[0].Body)
	}
	for i := 0; i < 3; i++ {
		if err := credentials.RemindAll(m, t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(notices(t, env, "credentials.expiring")); n != 1 {
		t.Fatalf("repeat checks: %d notifications", n)
	}
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 14) }) // 剩 6 天，命中 7
	_ = credentials.RemindAll(m, t.Context())
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 25) }) // 过期 5 天
	_ = credentials.RemindAll(m, t.Context())
	_ = credentials.RemindAll(m, t.Context())
	if n := len(notices(t, env, "credentials.expiring")); n != 3 {
		t.Fatalf("7 day and expired reminders: %d", n)
	}
	if n := len(notices(t, env, "credentials.stale")); n != 0 {
		t.Fatalf("no rotation period, no stale reminder: %d", n)
	}
}

func TestStaleReminderRepeatsEveryThirtyDays(t *testing.T) {
	env, m := setup(t)
	c := create(t, env, map[string]any{"kind": "api_key", "name": "OpenAI", "platform": "OpenAI", "createdOn": days(-300), "rotatedOn": days(-100), "rotateEveryDays": 90})
	got := notices(t, env, "credentials.stale")
	if len(got) != 1 || got[0].Title != "API 密钥 OpenAI「OpenAI」超过 90 天没更换" || !strings.Contains(got[0].Body, "上次更换 "+days(-100)) {
		t.Fatalf("created already stale: %+v", got)
	}
	for i := 0; i < 3; i++ {
		_ = credentials.RemindAll(m, t.Context())
	}
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 29) })
	_ = credentials.RemindAll(m, t.Context())
	if n := len(notices(t, env, "credentials.stale")); n != 1 {
		t.Fatalf("within 30 days: %d", n)
	}
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 31) })
	_ = credentials.RemindAll(m, t.Context())
	_ = credentials.RemindAll(m, t.Context())
	if n := len(notices(t, env, "credentials.stale")); n != 2 {
		t.Fatalf("after 30 days: %d", n)
	}

	// 更换以后状态回到 ok，不再提醒；周期到了又重新开始
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 40) })
	var r api.Credential
	env.MustDo(http.MethodPost, "/credentials/"+itoa(c.Id)+"/rotate", map[string]any{}, &r)
	if r.Status != api.Ok || r.RotatedOn != days(40) {
		t.Fatalf("rotated: %+v", r)
	}
	_ = credentials.RemindAll(m, t.Context())
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 40+95) })
	_ = credentials.RemindAll(m, t.Context())
	if n := len(notices(t, env, "credentials.stale")); n != 3 {
		t.Fatalf("a new overdue round: %d", n)
	}
}

func TestNoReminderForArchivedOrWithoutDates(t *testing.T) {
	env, m := setup(t)
	c := create(t, env, map[string]any{"kind": "ssh_key", "name": "无日期"})
	a := create(t, env, map[string]any{"kind": "ssh_key", "name": "归档的"})
	var u api.Credential
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(a.Id), map[string]any{"archived": true}, &u)
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(a.Id), map[string]any{"expiresOn": days(3), "createdOn": days(-400), "rotateEveryDays": 30}, &u)
	_ = credentials.RemindAll(m, t.Context())
	_ = c
	if got := append(notices(t, env, "credentials.expiring"), notices(t, env, "credentials.stale")...); len(got) != 0 {
		t.Fatalf("no reminders expected: %+v", got)
	}
}

func TestExpiryDateChangeStartsOver(t *testing.T) {
	env, m := setup(t)
	c := create(t, env, map[string]any{"kind": "access_token", "name": "t", "expiresOn": days(20)})
	var u api.Credential
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"expiresOn": days(400)}, &u)
	if n := len(notices(t, env, "credentials.expiring")); n != 1 {
		t.Fatalf("renewed far away: %d", n)
	}
	credentials.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 380) })
	_ = credentials.RemindAll(m, t.Context())
	if n := len(notices(t, env, "credentials.expiring")); n != 2 {
		t.Fatalf("after renewal: %d", n)
	}
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"notes": "x"}, &u)
	if n := len(notices(t, env, "credentials.expiring")); n != 2 {
		t.Fatalf("patch without date change: %d", n)
	}
}

func TestRotate(t *testing.T) {
	env, _ := setup(t)
	c := create(t, env, map[string]any{"kind": "api_key", "name": "k", "hint": "old1", "createdOn": days(-200), "expiresOn": days(-5), "rotateEveryDays": 90})
	if c.Status != api.Expired {
		t.Fatalf("before: %+v", c)
	}
	var r api.Credential
	env.MustDo(http.MethodPost, "/credentials/"+itoa(c.Id)+"/rotate", map[string]any{"expiresOn": days(360), "hint": "new2", "rotatedOn": days(-1)}, &r)
	if r.RotatedOn != days(-1) || r.ExpiresOn != days(360) || r.Hint != "new2" || r.CreatedOn != days(-200) || r.Status != api.Ok {
		t.Fatalf("rotated: %+v", r)
	}
	// 没给的字段保持原样，更换日默认今天
	env.MustDo(http.MethodPost, "/credentials/"+itoa(c.Id)+"/rotate", map[string]any{}, &r)
	if r.RotatedOn != days(0) || r.ExpiresOn != days(360) || r.Hint != "new2" {
		t.Fatalf("rotated with defaults: %+v", r)
	}
	for name, body := range map[string]map[string]any{
		"future":         {"rotatedOn": days(1)},
		"bad date":       {"rotatedOn": "yesterday"},
		"bad expiry":     {"expiresOn": "2026-13-01"},
		"before created": {"rotatedOn": days(-300)},
		"secret":         {"hint": "ghp_" + strings.Repeat("aB3", 12)},
	} {
		if status, _ := env.Do(http.MethodPost, "/credentials/"+itoa(c.Id)+"/rotate", body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, status)
		}
	}
	if status, _ := env.Do(http.MethodPost, "/credentials/999/rotate", map[string]any{}, nil); status != http.StatusNotFound {
		t.Errorf("missing: %d", status)
	}
}

func TestHiddenModuleGate(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "api_key", "name": "k", "expiresOn": days(10)})
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"credentials"}}, nil)
	if status, _ := env.Do(http.MethodGet, "/credentials", nil, nil); status != 200 {
		t.Fatalf("unlocked: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/credentials", nil, nil); status != 404 {
		t.Fatalf("locked list: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/credentials", map[string]any{"kind": "other", "name": "x"}, nil); status != 404 {
		t.Fatalf("locked create: %d", status)
	}
	if got := notices(t, env, "credentials.expiring"); len(got) != 0 {
		t.Fatalf("locked notifications must be hidden: %+v", got)
	}
	var avail struct{ Modules []string }
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, id := range avail.Modules {
		if id == "credentials" {
			t.Fatalf("credentials still available: %v", avail.Modules)
		}
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "credentials.list", json.RawMessage(`{}`)); err == nil {
		t.Fatal("the AI action must be hidden while the module is locked")
	}
}

func TestAIAction(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "api_key", "name": "OpenAI", "usedBy": []string{"服务器 hk-1"}, "hint": "a1b2", "expiresOn": days(100)})
	create(t, env, map[string]any{"kind": "ssh_key", "name": "部署钥匙", "usedBy": []string{"服务器 us-2"}})
	out, err := env.App.Deps.Actions.Run(context.Background(), "credentials.list", json.RawMessage(`{"q":"hk-1"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "OpenAI") || strings.Contains(string(raw), "部署钥匙") || !strings.Contains(string(raw), `"hint":"a1b2"`) {
		t.Fatalf("action output: %s", raw)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "credentials.list", json.RawMessage(`{"kind":"nope"}`)); err == nil {
		t.Fatal("bad kind accepted")
	}
}

func TestStoredSecretIsSealedAndNeedsElevation(t *testing.T) {
	env, _ := setup(t)
	token := "ghp_" + strings.Repeat("aB3", 12)
	c := create(t, env, map[string]any{"kind": "access_token", "name": "部署令牌", "secret": token})
	if !c.HasSecret {
		t.Fatalf("hasSecret: %+v", c)
	}
	// 数据库里是密文，列表和详情不带内容
	var stored string
	env.App.Deps.DB.QueryRow("SELECT secret_enc FROM credentials WHERE id = ?", c.Id).Scan(&stored)
	if stored == "" || strings.Contains(stored, token) {
		t.Fatalf("secret not sealed: %q", stored)
	}
	status, raw := env.Do(http.MethodGet, "/credentials/"+itoa(c.Id), nil, nil)
	if status != 200 || strings.Contains(string(raw), token) {
		t.Fatalf("detail leaks the secret: %d %s", status, raw)
	}
	status, raw = env.Do(http.MethodGet, "/credentials", nil, nil)
	if status != 200 || strings.Contains(string(raw), token) {
		t.Fatalf("list leaks the secret: %d %s", status, raw)
	}

	// 取出来要提升权限
	if status, _ := env.Do(http.MethodGet, "/credentials/"+itoa(c.Id)+"/secret", nil, nil); status != http.StatusForbidden {
		t.Fatalf("reveal without elevation: %d", status)
	}
	env.Elevate()
	var out struct{ Secret string }
	env.MustDo(http.MethodGet, "/credentials/"+itoa(c.Id)+"/secret", nil, &out)
	if out.Secret != token {
		t.Fatalf("revealed: %q", out.Secret)
	}

	// 改其他字段不动密钥，换密钥和清掉都可以
	var u api.Credential
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"name": "新名字"}, &u)
	env.MustDo(http.MethodGet, "/credentials/"+itoa(c.Id)+"/secret", nil, &out)
	if !u.HasSecret || out.Secret != token {
		t.Fatalf("patch kept: %+v %q", u, out.Secret)
	}
	env.MustDo(http.MethodPost, "/credentials/"+itoa(c.Id)+"/rotate", map[string]any{"secret": "new-value"}, &u)
	env.MustDo(http.MethodGet, "/credentials/"+itoa(c.Id)+"/secret", nil, &out)
	if out.Secret != "new-value" {
		t.Fatalf("rotated secret: %q", out.Secret)
	}
	env.MustDo(http.MethodPatch, "/credentials/"+itoa(c.Id), map[string]any{"secret": ""}, &u)
	if u.HasSecret {
		t.Fatalf("cleared: %+v", u)
	}
	if status, _ := env.Do(http.MethodGet, "/credentials/"+itoa(c.Id)+"/secret", nil, nil); status != http.StatusNotFound {
		t.Fatalf("reveal after clear: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/credentials", map[string]any{"kind": "other", "name": "x", "secret": strings.Repeat("长", 20001)}, nil); status != http.StatusBadRequest {
		t.Fatalf("too long: %d", status)
	}
}
