package coding_test

import (
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/coding"
)

// B42：Claude Code 的输入不含缓存，要加回去；Codex 的输入已经包含缓存。
func TestParseExecutorUsage(t *testing.T) {
	claude := []byte(`{"code":"result","costUsd":0.42,"usage":{"input_tokens":10,"cache_read_input_tokens":900,"cache_creation_input_tokens":90,"output_tokens":50}}`)
	in, cached, write, out, cost, ok := coding.ParseExecutorUsageForTest("claude", claude)
	if !ok || in != 1000 || cached != 900 || write != 90 || out != 50 || cost == nil || *cost != 0.42 {
		t.Fatalf("claude: %d %d %d %d %v %v", in, cached, write, out, cost, ok)
	}
	codex := []byte(`{"code":"result","usage":{"input_tokens":1000,"cached_input_tokens":600,"output_tokens":40}}`)
	in, cached, write, out, cost, ok = coding.ParseExecutorUsageForTest("codex", codex)
	if !ok || in != 1000 || cached != 600 || write != 0 || out != 40 || cost != nil {
		t.Fatalf("codex: %d %d %d %d %v %v", in, cached, write, out, cost, ok)
	}
	if _, _, _, _, _, ok := coding.ParseExecutorUsageForTest("codex", []byte(`{"code":"result"}`)); ok {
		t.Fatal("no usage should be skipped")
	}
	if _, _, _, _, _, ok := coding.ParseExecutorUsageForTest("claude", []byte(`{"code":"session_started"}`)); ok {
		t.Fatal("other events should be skipped")
	}
}
