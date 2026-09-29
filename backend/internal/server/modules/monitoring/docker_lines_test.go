package monitoring

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestTextLinesFromOldAgent(t *testing.T) {
	var tl textLines
	if got := tl.add("first\nsec"); len(got) != 1 {
		t.Fatalf("frames: %q", got)
	}
	if got := tl.add("ond\r\nthi"); len(got) != 1 {
		t.Fatalf("frames: %q", got)
	}
	last := tl.finish()
	if len(last) != 1 {
		t.Fatalf("finish: %q", last)
	}
	var lines []protocol.DockerLogLine
	if err := json.Unmarshal(last[0], &lines); err != nil || len(lines) != 1 || lines[0].Text != "thi" {
		t.Fatalf("last line: %q", last[0])
	}
	if tl.finish() != nil {
		t.Fatal("finish twice")
	}

	// 450 lines in one chunk: 200 + 200 + 50.
	var text strings.Builder
	for i := 0; i < 450; i++ {
		fmt.Fprintf(&text, "l%d\n", i)
	}
	sizes := []int{}
	for _, f := range tl.add(text.String()) {
		var part []protocol.DockerLogLine
		if err := json.Unmarshal(f, &part); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(part))
	}
	if fmt.Sprint(sizes) != "[200 200 50]" {
		t.Fatalf("sizes: %v", sizes)
	}
}
