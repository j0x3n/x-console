package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseName(t *testing.T) {
	for _, tc := range []struct {
		path string
		code string
		ok   bool
	}{
		{`C:\Users\jo\Downloads\x-console-agent-setup-K7QM-3XHP.exe`, "K7QM-3XHP", true},
		{`/tmp/x-console-agent-setup-k7qm-3xhp.exe`, "K7QM-3XHP", true},
		{`x-console-agent-setup-K7QM-3XHP (1).exe`, "K7QM-3XHP", true},
		{`x-console-agent-setup.exe`, "", true},
		{`x-console-agent-setup (2).exe`, "", true},
		{`x-console-agent.exe`, "", false},
		{`x-console-agent-setup-K7QM.exe`, "", false},
		{`x-console-agent-setup-K7QM-3XHP-EXTRA.exe`, "", false},
		{`my-x-console-agent-setup-K7QM-3XHP.exe`, "", false},
		{`x-console-agent-setup-K7QM-3XHP.exe.txt`, "", false},
		{``, "", false},
	} {
		code, ok := ParseName(tc.path)
		if code != tc.code || ok != tc.ok {
			t.Errorf("%q: got %q %v, want %q %v", tc.path, code, ok, tc.code, tc.ok)
		}
	}
}

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x-console-agent-setup-K7QM-3XHP.exe")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadTrailer(t *testing.T) {
	program := "MZ" + strings.Repeat("\x00binary\n", 3000) // more than the tail that is searched
	path := write(t, program+Marker+`{"server":"https://console.example.com"}`)
	tr, ok, err := ReadTrailer(path)
	if err != nil || !ok || tr.Server != "https://console.example.com" || tr.Size != int64(len(program)) {
		t.Fatalf("trailer: %+v %v %v", tr, ok, err)
	}
	// The marker text inside the program does not fool it: the last one counts.
	path = write(t, program+Marker+`{"server":"http://a"}`+"\n"+Marker+`{"server":"http://b:8080"}`)
	if tr, ok, _ := ReadTrailer(path); !ok || tr.Server != "http://b:8080" {
		t.Fatalf("last marker: %+v %v", tr, ok)
	}
	for name, content := range map[string]string{
		"no marker":    program,
		"empty":        "",
		"not json":     program + Marker + "nonsense",
		"bad server":   program + Marker + `{"server":"ftp://x"}`,
		"with a quote": program + Marker + `{"server":"https://x'y"}`,
	} {
		if tr, ok, err := ReadTrailer(write(t, content)); ok || err != nil {
			t.Errorf("%s: %+v %v %v", name, tr, ok, err)
		}
	}
	if _, _, err := ReadTrailer(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file: no error")
	}
}

func TestCopyProgramDropsTheTrailer(t *testing.T) {
	program := "MZ-program-bytes"
	src := write(t, program+Marker+`{"server":"https://console.example.com"}`)
	dst := filepath.Join(t.TempDir(), "sub", "x-console-agent.exe")
	if err := CopyProgram(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != program {
		t.Fatalf("copy: %q", got)
	}
	// Copying again replaces the file, and a program without a trailer is copied whole.
	plain := write(t, "plain program")
	if err := CopyProgram(plain, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "plain program" {
		t.Fatalf("second copy: %q", got)
	}
	if _, err := os.Stat(dst + ".new"); err == nil {
		t.Fatal("temporary file left behind")
	}
}
