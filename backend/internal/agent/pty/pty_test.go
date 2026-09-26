package pty

import (
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

func TestParseFrame(t *testing.T) {
	kind, data, _, ok := ParseFrame(DataFrame([]byte("ls\r")))
	if !ok || kind != protocol.PTYFrameData || string(data) != "ls\r" {
		t.Fatal(kind, data, ok)
	}
	kind, _, size, ok := ParseFrame(ResizeFrame(120, 32))
	if !ok || kind != protocol.PTYFrameResize || size.Cols != 120 || size.Rows != 32 {
		t.Fatal(kind, size, ok)
	}
	if _, _, _, ok := ParseFrame([]byte{9, 1}); ok {
		t.Fatal("unknown frame accepted")
	}
	if _, _, _, ok := ParseFrame(nil); ok {
		t.Fatal("empty frame accepted")
	}
	if _, _, _, ok := ParseFrame([]byte{1, '{'}); ok {
		t.Fatal("bad resize accepted")
	}
}

func TestClampSize(t *testing.T) {
	if c, r := ClampSize(0, 0); c != 80 || r != 24 {
		t.Fatal(c, r)
	}
	if c, r := ClampSize(5000, 5000); c != 1000 || r != 500 {
		t.Fatal(c, r)
	}
}

func TestWindowsCommandLine(t *testing.T) {
	cases := map[string]string{
		"":                                  "powershell.exe -NoLogo",
		"PowerShell":                        "powershell.exe -NoLogo",
		"pwsh":                              "pwsh.exe -NoLogo",
		"cmd":                               "cmd.exe",
		`C:\Program Files\Git\bin\bash.exe`: `"C:\Program Files\Git\bin\bash.exe"`,
		`wsl.exe`:                           "wsl.exe",
	}
	for in, want := range cases {
		if got := WindowsCommandLine(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
