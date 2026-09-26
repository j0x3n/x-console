package svc

import (
	"testing"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const unitsOut = `  cron.service                 loaded    active   running Regular background program processing daemon
● nginx.service                loaded    failed   failed  A high performance web server
ssh.service                    loaded    inactive dead    OpenBSD Secure Shell server
systemd-journald.socket        loaded    active   running Journal Socket
`

const filesOut = `cron.service     enabled  enabled
nginx.service    disabled enabled
ssh.service      enabled-runtime enabled
getty@.service   enabled  enabled
docker.service   disabled enabled
sshd.service     alias    -
`

func TestParseAndMerge(t *testing.T) {
	units := ParseUnits(unitsOut)
	if len(units) != 3 {
		t.Fatalf("units: %+v", units)
	}
	if units[1].Name != "nginx.service" || units[1].State != "failed" || units[0].Description != "Regular background program processing daemon" {
		t.Fatalf("parse: %+v", units)
	}
	all := MergeUnitFiles(units, ParseUnitFiles(filesOut))
	byName := map[string]protocol.ServiceInfo{}
	for _, u := range all {
		byName[u.Name] = u
	}
	if len(all) != 4 {
		t.Fatalf("merged: %+v", all)
	}
	if !byName["cron.service"].Enabled || byName["nginx.service"].Enabled || !byName["ssh.service"].Enabled {
		t.Fatalf("enabled: %+v", byName)
	}
	if d := byName["docker.service"]; d.State != "stopped" || d.StartType != "disabled" {
		t.Fatalf("unloaded unit: %+v", d)
	}
	if _, ok := byName["getty@.service"]; ok {
		t.Fatal("template unit listed")
	}
}

func TestValidate(t *testing.T) {
	ok := []protocol.SvcActionParams{{Name: "nginx.service", Action: "restart"}, {Name: "Spooler", Action: "stop"}, {Name: "getty@tty1.service", Action: "start"}}
	for _, p := range ok {
		if err := ValidateAction(p); err != nil {
			t.Fatalf("%+v: %v", p, err)
		}
	}
	bad := []protocol.SvcActionParams{{Name: "--now", Action: "stop"}, {Name: "x;rm", Action: "stop"}, {Name: "nginx", Action: "mask"}, {Name: "", Action: "start"}}
	for _, p := range bad {
		if err := ValidateAction(p); err == nil {
			t.Fatalf("%+v accepted", p)
		}
	}
}

func TestWindowsMapping(t *testing.T) {
	if s, _ := WindowsState(winRunning); s != "running" {
		t.Fatal(s)
	}
	if s, _ := WindowsState(winStopped); s != "stopped" {
		t.Fatal(s)
	}
	if s, sub := WindowsState(99); s != "other" || sub != "unknown" {
		t.Fatal(s, sub)
	}
	if n, en := WindowsStartType(winAutoStart, true); n != "auto_delayed" || !en {
		t.Fatal(n, en)
	}
	if n, en := WindowsStartType(winDisabled, false); n != "disabled" || en {
		t.Fatal(n, en)
	}
	if n, en := WindowsStartType(winDemandStart, false); n != "manual" || en {
		t.Fatal(n, en)
	}
}

func TestSystemdState(t *testing.T) {
	cases := map[string]string{"active": "running", "inactive": "stopped", "failed": "failed", "activating": "starting", "weird": "other"}
	for in, want := range cases {
		if got := SystemdState(in); got != want {
			t.Fatalf("%s: %s", in, got)
		}
	}
}
