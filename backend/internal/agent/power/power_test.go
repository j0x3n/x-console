package power

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestValidate(t *testing.T) {
	for _, a := range []string{"lock", "sleep", "shutdown", "restart"} {
		if err := ValidateAction(a); err != nil {
			t.Fatal(a, err)
		}
	}
	if ValidateAction("hibernate") == nil {
		t.Fatal("unknown action accepted")
	}
	if ValidateOpen(protocol.AppOpenParams{Target: "https://example.com"}) != nil {
		t.Fatal("url rejected")
	}
	for _, bad := range []string{"", "  ", "a\nb", strings.Repeat("x", 3000)} {
		if ValidateOpen(protocol.AppOpenParams{Target: bad}) == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}

func TestShutdownArgs(t *testing.T) {
	if a := ShutdownArgs("shutdown"); a[0] != "/s" || !slices.Contains(a, "/t") {
		t.Fatal(a)
	}
	if a := ShutdownArgs("restart"); a[0] != "/r" {
		t.Fatal(a)
	}
}

func TestUnsupportedOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	var pe *protocol.Error
	if err := doPower("lock"); !errors.As(err, &pe) || pe.Code != protocol.CodeUnsupported {
		t.Fatal(err)
	}
	if err := open("notepad", ""); !errors.As(err, &pe) || pe.Code != protocol.CodeUnsupported {
		t.Fatal(err)
	}
}
