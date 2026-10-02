package notes_test

import (
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
)

func TestPlainTextHidesHiddenBlocks(t *testing.T) {
	md := "开头 ==重点==\n\n:::hidden 密码\nroot / 123456\n:::\n\n:::tip 提示\n提示内容\n:::\n\n| a | b |\n| --- | --- |\n| 1 | 2 |"
	got := notes.PlainText(md)
	want := "开头 重点 提示内容 a b 1 2"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
