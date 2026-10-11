package protocol

import (
	"errors"
	"testing"
)

func TestNormalizeBlockDomain(t *testing.T) {
	good := map[string]string{
		"youtube.com":                          "youtube.com",
		"  YouTube.COM ":                       "youtube.com",
		"https://www.youtube.com/watch?v=abc":  "www.youtube.com",
		"http://user:pw@bilibili.com:8080/a#b": "bilibili.com",
		"weibo.com.":                           "weibo.com",
		"space.bilibili.com":                   "space.bilibili.com",
		"bbc.co.uk":                            "bbc.co.uk",
		"例子.中国":                                "xn--fsqu00a.xn--fiqs8s",
		"a-b.example.org":                      "a-b.example.org",
	}
	for in, want := range good {
		got, err := NormalizeBlockDomain(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	bad := map[string]error{
		"":                ErrBlockEmpty,
		"   ":             ErrBlockEmpty,
		"localhost":       ErrBlockDomainOne,
		"com":             ErrBlockDomainOne,
		"127.0.0.1":       ErrBlockDomainIP,
		"http://10.0.0.5": ErrBlockDomainIP,
		"[::1]":           ErrBlockDomainIP,
		"1.2.3":           ErrBlockDomainIP,
		"a b.com":         ErrBlockDomainFormat,
		"-a.com":          ErrBlockDomainFormat,
		"a..com":          ErrBlockDomainFormat,
		"a_b.com":         ErrBlockDomainFormat,
		"*.example.com":   ErrBlockDomainFormat,
	}
	for in, want := range bad {
		if got, err := NormalizeBlockDomain(in); !errors.Is(err, want) {
			t.Errorf("%q: got %q, %v; want %v", in, got, err, want)
		}
	}
}

func TestBlockHostNames(t *testing.T) {
	for in, want := range map[string][]string{
		"youtube.com":     {"youtube.com", "www.youtube.com", "m.youtube.com"},
		"www.youtube.com": {"youtube.com", "www.youtube.com", "m.youtube.com"},
	} {
		got := BlockHostNames(in)
		if len(got) != len(want) {
			t.Fatalf("%s: %v", in, got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: %v", in, got)
			}
		}
	}
}

func TestNormalizeBlockApp(t *testing.T) {
	for in, want := range map[string]string{
		"Steam.exe": "steam", " steam ": "steam", "Battle.net": "battle.net", "Google Chrome": "google chrome",
		"cloudmusic.EXE": "cloudmusic",
	} {
		got, err := NormalizeBlockApp(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]error{
		"": ErrBlockEmpty, `C:\Program Files\Steam\steam.exe`: ErrBlockAppPath, "/usr/bin/steam": ErrBlockAppPath,
		"c:steam": ErrBlockAppPath, "*": ErrBlockAppFormat, "st?am": ErrBlockAppFormat, "..": ErrBlockAppFormat,
		".exe": ErrBlockAppFormat,
	} {
		if got, err := NormalizeBlockApp(in); !errors.Is(err, want) {
			t.Errorf("%q: got %q, %v; want %v", in, got, err, want)
		}
	}
}

func TestIsProtectedProcess(t *testing.T) {
	for _, n := range []string{"System", "csrss.exe", "WinLogon.EXE", "explorer", "Explorer.exe", "lsass", "svchost.exe",
		"systemd", "systemd-journald", "init", "sshd", "dbus-daemon", "x-console-agent", "x-console-agent.exe", "kworker/0:1", "bash"} {
		if !IsProtectedProcess(n) {
			t.Errorf("%q should be protected", n)
		}
	}
	for _, n := range []string{"steam", "chrome.exe", "spotify", "notepad.exe", "cloudmusic", "vlc"} {
		if IsProtectedProcess(n) {
			t.Errorf("%q should not be protected", n)
		}
	}
}
