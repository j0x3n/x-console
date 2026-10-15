package watchers_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// The AI sets the format once. Later checks put every record into the
// template without AI, skip the header, and fall back to the plain text
// when the page no longer fits.
func TestFormatIsSetOnceAndReused(t *testing.T) {
	f := setup(t, true)
	f.site.set(func(s *site) {
		s.business = [][3]string{
			{"个体工商户注销", "东海县聪聪幕墙装饰经营部", "不予登记"},
			{"个体工商户开业", "东海云中月电子商务工作室〔个体工商户〕", "登记中（已入内网）"},
		}
	})
	var fields []map[string]any
	var refs []string
	wrong := do("finish", func(p string) map[string]any {
		refs = []string{ref(p, "] 个体工商户注销"), ref(p, "登记中")}
		fields = []map[string]any{
			{"name": "类型", "ref": ref(p, "] 个体工商户注销")},
			{"name": "名称", "ref": ref(p, "聪聪")},
			{"name": "状态", "ref": ref(p, "不予登记")},
		}
		return map[string]any{"refs": refs, "note": "我的业务", "fields": fields, "template": "{类型}（{名称}）于{日期}"}
	})
	right := do("finish", func(string) map[string]any {
		return map[string]any{"refs": refs, "note": "我的业务", "fields": fields, "template": "{类型}（{名称}）最新状态为“{状态}”"}
	})
	f.chat.script(wrong, right)
	w := f.create(t, map[string]any{"name": "个体户", "url": f.site.URL + "/business", "goal": "看我的业务办理状态"})
	got := f.until(t, w.Id, api.WatcherStatusOk)
	watchers.WaitRuns(f.m)
	want := "个体工商户注销（东海县聪聪幕墙装饰经营部）最新状态为“不予登记”；" +
		"个体工商户开业（东海云中月电子商务工作室〔个体工商户〕）最新状态为“登记中（已入内网）”"
	if got.CurrentValue != want {
		t.Fatalf("value %q", got.CurrentValue)
	}
	if !f.chat.saw("{日期} 没有在 fields 里定义") || !f.chat.saw("按格式读到 2 条记录") {
		t.Fatal("the model was not told about the template or the records")
	}

	// A status changes and a record is added. No AI is called.
	calls := f.chat.count()
	f.site.set(func(s *site) {
		s.business[1][2] = "已办结"
		s.business = append(s.business, [3]string{"个体工商户变更", "东海县某某店", "待受理"})
	})
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if f.chat.count() != calls {
		t.Fatal("a check called AI")
	}
	lines := strings.Split(got.CurrentValue, "；")
	if len(lines) != 3 || lines[1] != "个体工商户开业（东海云中月电子商务工作室〔个体工商户〕）最新状态为“已办结”" ||
		lines[2] != "个体工商户变更（东海县某某店）最新状态为“待受理”" {
		t.Fatalf("after check: %q", got.CurrentValue)
	}

	// The site is redesigned: the plain text is shown and the run says so.
	f.site.set(func(s *site) { s.flat = true })
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if got.Status != api.WatcherStatusOk || !strings.Contains(got.CurrentValue, "个体工商户注销 东海县聪聪幕墙装饰经营部 不予登记") {
		t.Fatalf("after redesign: %s %q", got.Status, got.CurrentValue)
	}
	if !strings.Contains(got.Runs[0].Detail, "格式对不上") {
		t.Fatalf("run detail %q", got.Runs[0].Detail)
	}
}

func TestFormatNeedsBothFieldsAndTemplate(t *testing.T) {
	f := setup(t, true)
	f.site.set(func(s *site) { s.business = [][3]string{{"个体工商户注销", "聪聪经营部", "不予登记"}} })
	var refs []string
	half := do("finish", func(p string) map[string]any {
		refs = []string{ref(p, "] 个体工商户注销")}
		return map[string]any{"refs": refs, "note": "我的业务", "fields": []map[string]any{{"name": "类型", "ref": refs[0]}}}
	})
	plain := do("finish", func(string) map[string]any { return map[string]any{"refs": refs, "note": "我的业务"} })
	f.chat.script(half, plain)
	w := f.create(t, map[string]any{"name": "个体户", "url": f.site.URL + "/business", "goal": "看状态"})
	got := f.until(t, w.Id, api.WatcherStatusOk)
	if !f.chat.saw("填了 fields 就要填 template") {
		t.Fatal("the model was not told to add a template")
	}
	if got.CurrentValue != "个体工商户注销" {
		t.Fatalf("value %q", got.CurrentValue)
	}
}
