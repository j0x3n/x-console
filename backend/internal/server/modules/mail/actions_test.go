package mail_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestAIMailActions(t *testing.T) {
	env := testutil.New(t)
	password, err := env.App.Deps.Secrets.Seal("隐藏密码")
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.App.Deps.DB.Exec("INSERT INTO mail_accounts(id,name,email,provider,imap_host,username,password_enc,created_at) VALUES(1,'测试邮箱','a@example.com','other','localhost','a',?,?)", password, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Repeat("中文正文", 700)
	body, _ := json.Marshal(map[string]any{"text": text, "to": []any{}, "cc": []any{}, "html": "秘密 HTML", "attachments": []any{}})
	for i := 1; i <= 3; i++ {
		_, err = env.App.Deps.DB.Exec("INSERT INTO mail_messages(id,account_id,uid,subject,date,unread,body_json) VALUES(?,1,?,?,?, ?,?)", i, i, fmt.Sprintf("测试邮件 %d", i), time.Now().Add(time.Duration(i)*time.Minute), i%2, string(body))
		if err != nil {
			t.Fatal(err)
		}
	}
	accounts := callAIAction(t, env, "mail.accounts", `{"limit":1}`).(map[string]any)
	account := accounts["items"].([]any)[0].(map[string]any)
	if account["unread"] != float64(2) || account["passwordEnc"] != nil || account["username"] != nil {
		t.Fatalf("账号状态: %+v", account)
	}
	page := callAIAction(t, env, "mail.list", `{"accountId":1,"unread":true,"limit":1}`).(map[string]any)
	if len(page["items"].([]any)) != 1 || page["nextBefore"] == nil {
		t.Fatalf("分页: %+v", page)
	}
	search := callAIAction(t, env, "mail.search", `{"q":"邮件 2"}`).(map[string]any)
	if len(search["items"].([]any)) != 1 {
		t.Fatalf("搜索: %+v", search)
	}
	detail := callAIAction(t, env, "mail.get", `{"id":1}`).(map[string]any)
	if len([]rune(detail["text"].(string))) > 2000 || detail["textTruncated"] != true || detail["html"] != nil {
		t.Fatalf("正文未精简: %+v", detail)
	}
	if _, err := env.App.Deps.Actions.Run(context.Background(), "mail.get", json.RawMessage(`{"id":999}`)); err == nil {
		t.Fatal("不存在的邮件应返回错误")
	}
	checkAIActionInputs(t, env, "mail.accounts", "mail.list", "mail.search", "mail.get")
	checkAIHiddenModule(t, env, "mail", "mail.accounts", "mail.list", "mail.search", "mail.get")
}

func callAIAction(t *testing.T, env *testutil.Env, name, input string) any {
	t.Helper()
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{Username: "jo", VaultUntil: &until})
	result, err := env.App.Deps.Actions.Run(ctx, name, json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func checkAIActionInputs(t *testing.T, env *testutil.Env, names ...string) {
	t.Helper()
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{VaultUntil: &until})
	for _, name := range names {
		a, ok := env.App.Deps.Actions.Get(ctx, name)
		if !ok {
			t.Fatalf("操作未注册: %s", name)
		}
		wantEffect := actions.Read
		if name == "backup.export" {
			wantEffect = actions.Dangerous
		}
		if a.Effect != wantEffect {
			t.Fatalf("%s 的 Effect 不正确: %s", name, a.Effect)
		}
		for _, raw := range []string{`{"unexpected":true}`, `null`, `{} {}`, `{"limit":51}`, `{"limit":0}`, `{"limit":-1}`} {
			_, err := a.Run(ctx, json.RawMessage(raw))
			var failure *httpx.Error
			if !errors.As(err, &failure) || failure.Status != 400 {
				t.Fatalf("%s 无效参数未返回 400: %s %v", name, raw, err)
			}
		}
	}
}

type aiHiddenModules map[string]bool

func (h aiHiddenModules) Hidden(ctx context.Context, name string) bool {
	return h[name] && !auth.VaultUnlocked(ctx)
}

func checkAIHiddenModule(t *testing.T, env *testutil.Env, side string, names ...string) {
	t.Helper()
	module.Provide[contracts.HiddenModules](env.App.Deps.Registry, contracts.HiddenModulesKey, aiHiddenModules{side: true})
	until := time.Now().Add(time.Hour)
	ctx := auth.WithSession(context.Background(), &auth.Session{VaultUntil: &until})
	for _, name := range names {
		a, ok := env.App.Deps.Actions.Get(ctx, name)
		if !ok {
			t.Fatalf("找不到操作 %s", name)
		}
		raw := json.RawMessage(`{}`)
		var spec struct {
			Required []string `json:"required"`
		}
		_ = json.Unmarshal(a.Input, &spec)
		fields := map[string]any{}
		for _, key := range spec.Required {
			if key == "q" {
				fields[key] = "测试"
			} else {
				fields[key] = 1
			}
		}
		if len(fields) > 0 {
			raw, _ = json.Marshal(fields)
		}
		_, err := a.Run(ctx, raw)
		var failure *httpx.Error
		if !errors.As(err, &failure) || failure.Status != 404 {
			t.Fatalf("%s 隐藏内容未被阻止: %v", name, err)
		}
	}
}
