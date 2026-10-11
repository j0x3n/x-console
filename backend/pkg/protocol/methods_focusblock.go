package protocol

import (
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
)

// B131: block distracting sites and programs while a pomodoro runs. The
// server tells the agent what to block and for how long. The agent edits its
// own marked block in the hosts file, ends the listed programs, and restores
// everything by itself when the time is up. See docs/specs/B131.md.
const (
	CapFocusBlock = "focusblock"

	MethodFocusBlockApply  = "focusblock.apply"  // FocusBlockApplyParams -> FocusBlockStatus
	MethodFocusBlockClear  = "focusblock.clear"  // nil -> FocusBlockStatus
	MethodFocusBlockStatus = "focusblock.status" // nil -> FocusBlockStatus
)

// Limits on what the panel may send.
const (
	FocusBlockMaxDomains = 200
	FocusBlockMaxApps    = 100
	// FocusBlockMaxTTLSeconds is the longest block the agent accepts. A pomodoro
	// is at most 3 hours; the rest is the break and the grace time.
	FocusBlockMaxTTLSeconds = 6 * 60 * 60
)

// FocusBlockApplyParams starts or renews a block. Sending it again for the
// same session replaces the lists and the deadline.
type FocusBlockApplyParams struct {
	SessionID int64 `json:"sessionId"`
	// TTLSeconds is how long the agent keeps the block if the server never says
	// to stop. It counts from the moment the agent receives the request, so a
	// wrong clock on either side does not matter.
	TTLSeconds int      `json:"ttlSeconds"`
	Domains    []string `json:"domains,omitempty"`
	// Apps are program names without path. Case and ".exe" do not matter.
	Apps []string `json:"apps,omitempty"`
	// IPv6 also points the names to ::1.
	IPv6 bool `json:"ipv6,omitempty"`
}

// Hosts file results in FocusBlockStatus.Hosts.
const (
	FocusBlockHostsOK         = "ok"         // the marked block is in the hosts file
	FocusBlockHostsNone       = "none"       // no site was asked for, nothing was written
	FocusBlockHostsPermission = "permission" // the agent may not write the hosts file
	FocusBlockHostsFailed     = "failed"     // any other write error
)

// FocusBlockStatus is what the agent says about itself.
type FocusBlockStatus struct {
	Active    bool  `json:"active"`
	SessionID int64 `json:"sessionId,omitempty"`
	// RemainingSeconds is the time left until the agent restores by itself.
	RemainingSeconds int `json:"remainingSeconds,omitempty"`
	// Hosts is one of the FocusBlockHosts* values. When Active is false it
	// describes the clean-up: "ok" means the block is gone.
	Hosts      string `json:"hosts"`
	HostsError string `json:"hostsError,omitempty"`
	// Sites is the number of host names written.
	Sites int `json:"sites"`
	// Killed counts the programs ended since this block started.
	Killed     int      `json:"killed"`
	LastKilled []string `json:"lastKilled,omitempty"`
	// Refused lists entries the agent did not use, with the reason in the
	// same string, for example "explorer (protected program)".
	Refused []string `json:"refused,omitempty"`
}

// ---- names ----

// Errors of the name checks. The server turns them into messages.
var (
	ErrBlockEmpty        = errors.New("empty")
	ErrBlockDomainFormat = errors.New("not a domain name")
	ErrBlockDomainIP     = errors.New("an IP address, not a domain name")
	ErrBlockDomainOne    = errors.New("needs at least two parts like example.com")
	ErrBlockAppFormat    = errors.New("not a program name")
	ErrBlockAppPath      = errors.New("a path, not a program name")
	ErrBlockProtected    = errors.New("protected program")
)

// NormalizeBlockDomain turns what the user typed (a host name or a whole
// address) into a lower-case ASCII host name without scheme, port or path.
func NormalizeBlockDomain(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", ErrBlockEmpty
	}
	if i := strings.Index(s, "://"); i > 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 && !strings.Contains(s[i:], "]") {
		port := s[i+1:]
		if port != "" && strings.Trim(port, "0123456789") == "" {
			s = s[:i]
		}
	}
	s = strings.Trim(s, ".")
	if s == "" {
		return "", ErrBlockDomainFormat
	}
	if _, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return "", ErrBlockDomainIP
	}
	ascii, err := idna.Lookup.ToASCII(s)
	if err != nil {
		return "", ErrBlockDomainFormat
	}
	s = ascii
	if len(s) > 253 {
		return "", ErrBlockDomainFormat
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", ErrBlockDomainFormat
		}
		for _, r := range l {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return "", ErrBlockDomainFormat
			}
		}
	}
	if len(labels) < 2 {
		return "", ErrBlockDomainOne
	}
	// The last part of a real name has a letter, so 1.2.3.4 and 10.0 are out.
	if strings.Trim(labels[len(labels)-1], "0123456789") == "" {
		return "", ErrBlockDomainIP
	}
	return s, nil
}

// BlockHostNames lists the host names written for one domain: the name itself
// and the usual prefixes. A hosts file has no wildcards, so every name is a
// line of its own.
func BlockHostNames(domain string) []string {
	base := strings.TrimPrefix(domain, "www.")
	return []string{base, "www." + base, "m." + base}
}

// NormalizeBlockApp returns the canonical form of a program name: lower case,
// without ".exe". Paths and wildcards are refused, so a name can only match
// the name of a process.
func NormalizeBlockApp(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", ErrBlockEmpty
	}
	if strings.ContainsAny(s, `/\:`) {
		return "", ErrBlockAppPath
	}
	s = strings.TrimSuffix(s, ".exe")
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 64 || strings.Trim(s, ".") == "" {
		return "", ErrBlockAppFormat
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`*?"<>|`, r) {
			return "", ErrBlockAppFormat
		}
	}
	return s, nil
}

// protectedProcesses are never ended, whatever the list says. Names are in
// the canonical form of NormalizeBlockApp. The list is shared by Windows and
// Linux: a name that means nothing on one system does no harm there.
var protectedProcesses = map[string]bool{}

func init() {
	for _, n := range []string{
		// Windows
		"system", "system idle process", "registry", "smss", "csrss", "wininit", "winlogon", "services",
		"lsass", "lsaiso", "lsm", "svchost", "fontdrvhost", "dwm", "explorer", "sihost", "taskhostw",
		"ctfmon", "runtimebroker", "searchhost", "searchindexer", "startmenuexperiencehost",
		"shellexperiencehost", "textinputhost", "applicationframehost", "securityhealthservice",
		"securityhealthsystray", "msmpeng", "nissrv", "wudfhost", "spoolsv", "audiodg", "conhost",
		"dllhost", "wmiprvse", "logonui", "userinit", "taskmgr", "mmc", "memory compression",
		"secure system", "dashost", "smartscreen", "lockapp",
		// Linux
		"init", "dbus-daemon", "dbus-broker", "dbus-broker-launch", "sshd", "login", "agetty", "getty",
		"mingetty", "cron", "crond", "atd", "rsyslogd", "udevd", "networkmanager", "wpa_supplicant",
		"polkitd", "containerd", "dockerd", "docker", "xorg", "x", "xwayland", "gnome-shell",
		"gnome-session", "plasmashell", "kwin_x11", "kwin_wayland", "gdm", "gdm3", "sddm", "lightdm",
		"pipewire", "wireplumber", "pulseaudio", "bash", "sh", "dash", "zsh", "fish", "tmux", "screen",
		"sudo", "su", "kernel_task",
	} {
		protectedProcesses[n] = true
	}
}

var protectedPrefixes = []string{"systemd", "x-console", "kworker", "ksoftirqd", "kthreadd", "migration/", "rcu_"}

// IsProtectedProcess reports whether a program must never be ended. It takes
// any spelling of the name.
func IsProtectedProcess(name string) bool {
	n, err := NormalizeBlockApp(name)
	if err != nil {
		n = strings.ToLower(strings.TrimSpace(name))
	}
	if protectedProcesses[n] {
		return true
	}
	for _, p := range protectedPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}
