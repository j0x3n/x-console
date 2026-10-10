package documents_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// clockDay is the fixed "today" of the tests: 2026-10-10 in Asia/Shanghai.
var clockDay = time.Date(2026, 10, 10, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))

func setup(t *testing.T) (*testutil.Env, *documents.Module) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*documents.Module](env.App.Deps.Registry, documents.ServiceKey)
	if !ok {
		t.Fatal("documents module not registered")
	}
	documents.SetNow(m, func() time.Time { return clockDay })
	return env, m
}

type listOut struct {
	Items   []api.Document      `json:"items"`
	Summary api.DocumentSummary `json:"summary"`
}

func create(t *testing.T, env *testutil.Env, body map[string]any) api.Document {
	t.Helper()
	var d api.Document
	env.MustDo(http.MethodPost, "/documents", body, &d)
	return d
}

func days(n int) string { return clockDay.AddDate(0, 0, n).Format("2006-01-02") }

type notice struct{ Kind, Title, Body, Link string }

func notices(t *testing.T, env *testutil.Env) []notice {
	t.Helper()
	var out struct{ Items []notice }
	env.MustDo(http.MethodGet, "/notifications", nil, &out)
	var got []notice
	for _, n := range out.Items {
		if n.Kind == "documents.expiring" {
			got = append(got, n)
		}
	}
	return got
}

func TestCreateListSortAndStatus(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "contract", "name": "永久合同"})
	create(t, env, map[string]any{"kind": "passport", "name": "护照", "holder": "李四", "number": "E12345678", "expiresOn": days(200)})
	create(t, env, map[string]any{"kind": "visa", "name": "美国签证", "expiresOn": days(20)})
	create(t, env, map[string]any{"kind": "insurance", "name": "旧保险", "expiresOn": days(-3)})
	create(t, env, map[string]any{"kind": "item", "name": "笔记本电脑", "serial": "SN-42", "price": 8999.5, "currency": "CNY", "issuedOn": days(-300), "expiresOn": days(65)})

	var out listOut
	env.MustDo(http.MethodGet, "/documents", nil, &out)
	var names []string
	for _, d := range out.Items {
		names = append(names, d.Name)
	}
	if got := strings.Join(names, ","); got != "旧保险,美国签证,笔记本电脑,护照,永久合同" {
		t.Fatalf("order: %s", got)
	}
	want := map[string]api.DocumentStatus{"旧保险": api.Expired, "美国签证": api.Soon, "笔记本电脑": api.Soon, "护照": api.Ok, "永久合同": api.None}
	for _, d := range out.Items {
		if d.Status != want[d.Name] {
			t.Errorf("%s status = %s, want %s", d.Name, d.Status, want[d.Name])
		}
	}
	if out.Items[0].DaysLeft == nil || *out.Items[0].DaysLeft != -3 || out.Items[1].DaysLeft == nil || *out.Items[1].DaysLeft != 20 {
		t.Fatalf("daysLeft: %+v", out.Items[:2])
	}
	if out.Items[4].DaysLeft != nil {
		t.Fatal("no expiry date must have no daysLeft")
	}
	if out.Summary.Total != 5 || out.Summary.Expired != 1 || out.Summary.Soon != 2 || out.Summary.None != 1 {
		t.Fatalf("summary: %+v", out.Summary)
	}
	if got := out.Items[3].RemindDays; len(got) != 3 || got[0] != 90 || got[2] != 7 {
		t.Fatalf("default remind days: %v", got)
	}

	// 筛选和搜索只改列表，不改概要
	env.MustDo(http.MethodGet, "/documents?kind=visa", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Name != "美国签证" || out.Summary.Total != 5 {
		t.Fatalf("kind filter: %+v %+v", out.Items, out.Summary)
	}
	env.MustDo(http.MethodGet, "/documents?q=sn-42", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Kind != api.Item || out.Items[0].Price == nil || *out.Items[0].Price != 8999.5 {
		t.Fatalf("q filter: %+v", out.Items)
	}
}

func TestNumberIsEncryptedAtRest(t *testing.T) {
	env, _ := setup(t)
	d := create(t, env, map[string]any{"kind": "passport", "name": "护照", "number": "E12345678"})
	if d.Number != "E12345678" {
		t.Fatalf("number in response: %q", d.Number)
	}
	var raw string
	if err := env.App.Deps.DB.QueryRow("SELECT number FROM documents WHERE id=?", d.Id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "" || strings.Contains(raw, "E12345678") {
		t.Fatalf("number stored in plain text: %q", raw)
	}
	// 不改编号的修改不会动密文
	var u api.Document
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"holder": "李四"}, &u)
	var again string
	_ = env.App.Deps.DB.QueryRow("SELECT number FROM documents WHERE id=?", d.Id).Scan(&again)
	if again != raw || u.Number != "E12345678" || u.Holder != "李四" {
		t.Fatalf("patch changed number: %q vs %q, %+v", again, raw, u)
	}
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"number": ""}, &u)
	if u.Number != "" {
		t.Fatalf("number not cleared: %q", u.Number)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestValidation(t *testing.T) {
	env, _ := setup(t)
	bad := []map[string]any{
		{"kind": "passport", "name": "  "},
		{"kind": "boat", "name": "x"},
		{"kind": "passport", "name": "x", "expiresOn": "2026/10/10"},
		{"kind": "passport", "name": "x", "issuedOn": "2026-10-10", "expiresOn": "2026-10-01"},
		{"kind": "passport", "name": "x", "remindDays": []int{0}},
		{"kind": "passport", "name": "x", "remindDays": []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"kind": "item", "name": "x", "price": -1},
		{"kind": "passport", "name": strings.Repeat("长", 101)},
	}
	for i, body := range bad {
		if status, raw := env.Do(http.MethodPost, "/documents", body, nil); status != 400 {
			t.Errorf("case %d: status %d: %s", i, status, raw)
		}
	}
	if status, _ := env.Do(http.MethodGet, "/documents/9999", nil, nil); status != 404 {
		t.Errorf("missing id: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, "/documents?kind=boat", nil, nil); status != 400 {
		t.Errorf("bad kind filter: %d", status)
	}
}

func TestArchiveHidesFromList(t *testing.T) {
	env, _ := setup(t)
	d := create(t, env, map[string]any{"kind": "visa", "name": "签证", "expiresOn": days(10)})
	var u api.Document
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"archived": true}, &u)
	if !u.Archived {
		t.Fatal("not archived")
	}
	var out listOut
	env.MustDo(http.MethodGet, "/documents", nil, &out)
	if len(out.Items) != 0 || out.Summary.Total != 0 {
		t.Fatalf("archived shown: %+v", out)
	}
	env.MustDo(http.MethodGet, "/documents?archived=true", nil, &out)
	if len(out.Items) != 1 {
		t.Fatalf("archived=true: %+v", out)
	}
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"archived": false}, &u)
	if u.Archived {
		t.Fatal("still archived")
	}
}

func TestDeleteNeedsElevation(t *testing.T) {
	env, _ := setup(t)
	d := create(t, env, map[string]any{"kind": "other", "name": "x"})
	if status, _ := env.Do(http.MethodDelete, "/documents/"+itoa(d.Id), nil, nil); status != 403 {
		t.Fatalf("delete without elevation: %d", status)
	}
	env.Elevate()
	if status, raw := env.Do(http.MethodDelete, "/documents/"+itoa(d.Id), nil, nil); status != 204 {
		t.Fatalf("delete: %d %s", status, raw)
	}
	if status, _ := env.Do(http.MethodGet, "/documents/"+itoa(d.Id), nil, nil); status != 404 {
		t.Fatalf("after delete: %d", status)
	}
}

func TestFiles(t *testing.T) {
	env, _ := setup(t)
	d := create(t, env, map[string]any{"kind": "contract", "name": "租房合同"})
	var u api.Document
	env.MustDo(http.MethodPost, "/documents/"+itoa(d.Id)+"/files", map[string]any{"driveId": 7, "name": "合同.pdf"}, &u)
	env.MustDo(http.MethodPost, "/documents/"+itoa(d.Id)+"/files", map[string]any{"driveId": 8, "name": "发票.jpg"}, &u)
	env.MustDo(http.MethodPost, "/documents/"+itoa(d.Id)+"/files", map[string]any{"driveId": 7, "name": "合同-新.pdf"}, &u)
	if len(u.Files) != 2 || u.Files[0].Name != "合同-新.pdf" {
		t.Fatalf("files: %+v", u.Files)
	}
	env.MustDo(http.MethodDelete, "/documents/"+itoa(d.Id)+"/files/7", nil, &u)
	if len(u.Files) != 1 || u.Files[0].DriveId != 8 {
		t.Fatalf("after remove: %+v", u.Files)
	}
	if status, _ := env.Do(http.MethodPost, "/documents/"+itoa(d.Id)+"/files", map[string]any{"driveId": 0, "name": "x"}, nil); status != 400 {
		t.Fatalf("bad drive id: %d", status)
	}
	for i := int64(100); i < 120; i++ {
		env.Do(http.MethodPost, "/documents/"+itoa(d.Id)+"/files", map[string]any{"driveId": i, "name": "f"}, nil)
	}
	env.MustDo(http.MethodGet, "/documents/"+itoa(d.Id), nil, &u)
	if len(u.Files) != 20 {
		t.Fatalf("limit: %d files", len(u.Files))
	}
}

// ---- 提醒 ----

func TestReminderFiresOncePerDay(t *testing.T) {
	env, m := setup(t)
	create(t, env, map[string]any{"kind": "passport", "name": "护照", "holder": "李四", "number": "E12345678", "expiresOn": days(20)})
	got := notices(t, env)
	if len(got) != 1 {
		t.Fatalf("created with 20 days left: want 1 notification, got %+v", got)
	}
	if got[0].Title != "护照「护照」还有 20 天到期" || got[0].Link != "/documents" {
		t.Fatalf("notification: %+v", got[0])
	}
	if strings.Contains(got[0].Body, "E12345678") || !strings.Contains(got[0].Body, days(20)) {
		t.Fatalf("body: %q", got[0].Body)
	}
	// 重复检查不重复发；90 和 30 天都算已提醒
	for i := 0; i < 3; i++ {
		if err := documents.RemindAll(m, t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(notices(t, env)); n != 1 {
		t.Fatalf("repeat checks: %d notifications", n)
	}
	// 过了 14 天（还剩 6 天）命中 7 天
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 14) })
	_ = documents.RemindAll(m, t.Context())
	got = notices(t, env)
	if len(got) != 2 || !strings.Contains(got[0].Title+got[1].Title, "还有 6 天到期") {
		t.Fatalf("7 day reminder: %+v", got)
	}
	// 到期当天
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 20) })
	_ = documents.RemindAll(m, t.Context())
	// 过期后一次
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 25) })
	_ = documents.RemindAll(m, t.Context())
	_ = documents.RemindAll(m, t.Context())
	got = notices(t, env)
	if len(got) != 3 {
		t.Fatalf("expiry day and after are one reminder: %+v", got)
	}
	titles := got[0].Title + got[1].Title + got[2].Title
	if !strings.Contains(titles, "今天到期") && !strings.Contains(titles, "已过期 5 天") {
		t.Fatalf("expiry notification: %+v", got)
	}
}

func TestExpiryDateChangeStartsOver(t *testing.T) {
	env, m := setup(t)
	d := create(t, env, map[string]any{"kind": "visa", "name": "签证", "expiresOn": days(20)})
	if n := len(notices(t, env)); n != 1 {
		t.Fatalf("first: %d", n)
	}
	var u api.Document
	// 续期：新的到期日还很远，不提醒
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"expiresOn": days(400)}, &u)
	if n := len(notices(t, env)); n != 1 {
		t.Fatalf("renewed far away: %d", n)
	}
	// 过了一年，又到 20 天内：重新提醒
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 380) })
	_ = documents.RemindAll(m, t.Context())
	if n := len(notices(t, env)); n != 2 {
		t.Fatalf("after renewal: %d", n)
	}
	// 同一个到期日再保存一次不重复
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"notes": "x"}, &u)
	if n := len(notices(t, env)); n != 2 {
		t.Fatalf("patch without date change: %d", n)
	}
	// 到期日换成别的值就重新算：现在离新日期 21 天，命中 30 天
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"expiresOn": days(401)}, &u)
	if n := len(notices(t, env)); n != 3 {
		t.Fatalf("moved to 21 days left: %d", n)
	}
}

func TestNoReminderForArchivedOrWithoutDate(t *testing.T) {
	env, m := setup(t)
	d := create(t, env, map[string]any{"kind": "contract", "name": "永久合同"})
	create(t, env, map[string]any{"kind": "visa", "name": "归档签证", "expiresOn": days(500)})
	var u api.Document
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"archived": true}, &u)
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 495) })
	// 归档后改成快到期
	env.MustDo(http.MethodPatch, "/documents/"+itoa(d.Id), map[string]any{"expiresOn": days(499)}, &u)
	_ = documents.RemindAll(m, t.Context())
	got := notices(t, env)
	if len(got) != 1 || !strings.Contains(got[0].Title, "归档签证") {
		t.Fatalf("only the visa should remind: %+v", got)
	}
}

func TestCustomRemindDays(t *testing.T) {
	env, m := setup(t)
	create(t, env, map[string]any{"kind": "insurance", "name": "车险", "expiresOn": days(40), "remindDays": []int{14, 60, 14}})
	got := notices(t, env)
	if len(got) != 1 {
		t.Fatalf("60 days is already past: %+v", got)
	}
	_ = documents.RemindAll(m, t.Context())
	documents.SetNow(m, func() time.Time { return clockDay.AddDate(0, 0, 30) }) // 剩 10 天
	_ = documents.RemindAll(m, t.Context())
	if n := len(notices(t, env)); n != 2 {
		t.Fatalf("14 day reminder: %d", n)
	}
	// 清空提醒天数：只剩到期当天
	d := create(t, env, map[string]any{"kind": "insurance", "name": "意外险", "expiresOn": days(35), "remindDays": []int{}})
	if len(d.RemindDays) != 0 {
		t.Fatalf("remind days: %v", d.RemindDays)
	}
	before := len(notices(t, env))
	_ = documents.RemindAll(m, t.Context())
	if len(notices(t, env)) != before {
		t.Fatal("no remind days means only the expiry day")
	}
}

// ---- 隐藏内容和 AI 动作 ----

func TestHiddenModuleGate(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "passport", "name": "护照", "expiresOn": days(10)})
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret-one"}, nil)
	env.MustDo(http.MethodPut, "/vault/modules", map[string]any{"hidden": []string{"documents"}}, nil)
	// 解锁时能用
	if status, _ := env.Do(http.MethodGet, "/documents", nil, nil); status != 200 {
		t.Fatalf("unlocked: %d", status)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	if status, _ := env.Do(http.MethodGet, "/documents", nil, nil); status != 404 {
		t.Fatalf("locked list: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/documents", map[string]any{"kind": "other", "name": "x"}, nil); status != 404 {
		t.Fatalf("locked create: %d", status)
	}
	if got := notices(t, env); len(got) != 0 {
		t.Fatalf("locked notifications must be hidden: %+v", got)
	}
	var avail struct{ Modules []string }
	env.MustDo(http.MethodGet, "/app/modules", nil, &avail)
	for _, id := range avail.Modules {
		if id == "documents" {
			t.Fatalf("documents still available: %v", avail.Modules)
		}
	}
}

func TestAIActionHidesNumber(t *testing.T) {
	env, _ := setup(t)
	create(t, env, map[string]any{"kind": "passport", "name": "护照", "number": "E12345678", "expiresOn": days(100)})
	out, err := env.App.Deps.Actions.Run(context.Background(), "documents.list", json.RawMessage(`{"kind":"passport"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "E12345678") || !strings.Contains(string(raw), "护照") {
		t.Fatalf("action output: %s", raw)
	}
}
