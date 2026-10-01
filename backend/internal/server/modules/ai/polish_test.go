package ai_test

import (
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestPolishPromptScenes(t *testing.T) {
	for _, scene := range []string{"note", "card", "comment", "task", "reminder", "event", "general"} {
		p, ok := ai.PolishPromptForTest(scene, "")
		if !ok || !strings.Contains(p, "原样保留") {
			t.Fatalf("scene %s: ok=%v prompt=%q", scene, ok, p)
		}
	}
	card, _ := ai.PolishPromptForTest("card", "")
	note, _ := ai.PolishPromptForTest("note", "")
	if card == note || !strings.Contains(card, "专业") {
		t.Fatalf("card prompt should differ from note and ask for a professional tone: %q", card)
	}
	withAsk, _ := ai.PolishPromptForTest("note", " 改成要点列表 ")
	if !strings.HasSuffix(withAsk, "改成要点列表") {
		t.Fatalf("user request not appended: %q", withAsk)
	}
	if _, ok := ai.PolishPromptForTest("poem", ""); ok {
		t.Fatal("unknown scene accepted")
	}
}

func TestPolishTextValidates(t *testing.T) {
	env := testutil.New(t)
	for _, body := range []map[string]any{
		{"text": "   ", "scene": "note"},
		{"text": strings.Repeat("字", 20001), "scene": "note"},
		{"text": "hello", "scene": "poem"},
	} {
		if status, _ := env.Do("POST", "/ai/polish", body, nil); status != 400 {
			t.Fatalf("body %v: status %d, want 400", body["scene"], status)
		}
	}
	// 没配置模型时回 409，前端提示去设置
	if status, _ := env.Do("POST", "/ai/polish", map[string]any{"text": "hello", "scene": "card"}, nil); status != 409 {
		t.Fatalf("not configured: status %d, want 409", status)
	}
}
