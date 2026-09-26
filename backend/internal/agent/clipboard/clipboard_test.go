package clipboard

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestClean(t *testing.T) {
	got, err := Clean("a\x00b\nc\r\nd")
	if err != nil || got != "ab\r\nc\r\nd" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := Clean(strings.Repeat("x", MaxText+1)); err == nil {
		t.Fatal("oversized text accepted")
	}
}

func TestUnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has a clipboard")
	}
	var pe *protocol.Error
	if _, err := get(); !errors.As(err, &pe) || pe.Code != protocol.CodeUnsupported {
		t.Fatalf("get: %v", err)
	}
	if Available() {
		t.Fatal("available off windows")
	}
}
