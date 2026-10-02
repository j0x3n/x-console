package presence

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type Sample = protocol.PresenceSample

func sampleUserSession(session uint32, idle func() (int64, bool), locked func() *bool) Sample {
	if session == 0 {
		return Sample{}
	}
	seconds, known := idle()
	if !known || seconds < 0 {
		return Sample{}
	}
	return Sample{Known: true, IdleSeconds: seconds, Locked: locked()}
}

type commandRunner func(context.Context, string, ...string) ([]byte, error)

func runCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func Register(c *conn.Client) {
	register(c, Get)
}

func register(c *conn.Client, sampleNow func(context.Context) Sample) {
	c.Handle(protocol.MethodPresenceGet, func(ctx context.Context, _ json.RawMessage) (any, error) { return sampleNow(ctx), nil })
	c.OnConnect(func(ctx context.Context) {
		last := sampleNow(ctx)
		_ = c.Emit(ctx, protocol.EventPresenceUpdate, last)
		at := time.Now()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				sample := sampleNow(ctx)
				if shouldReport(last, sample, at, now) {
					if err := c.Emit(ctx, protocol.EventPresenceUpdate, sample); err == nil {
						at = now
					}
				}
				last = sample
			}
		}
	})
}

func shouldReport(last, current Sample, at, now time.Time) bool {
	return now.Sub(at) >= 30*time.Second || last.Known != current.Known || !sameBool(last.Locked, current.Locked) || !sameBool(last.DisplayOff, current.DisplayOff) || current.IdleSeconds < last.IdleSeconds || last.IdleSeconds/60 != current.IdleSeconds/60
}

func sameBool(a, b *bool) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

var idleNanoseconds = regexp.MustCompile(`"HIDIdleTime"\s*=\s*([0-9]+)`)
var darwinLock = regexp.MustCompile(`"CGSSessionScreenIsLocked"\s*=\s*(Yes|No|true|false)\b`)
var displayPower = regexp.MustCompile(`"CurrentPowerState"\s*=\s*([0-9]+)`)
var busIdle = regexp.MustCompile(`^\(\s*(?:(?:uint32|uint64|int64)\s+)?([0-9]+)\s*,?\s*\)$`)

func sampleLinux(ctx context.Context, run commandRunner, session string) Sample {
	var out Sample
	if raw, err := run(ctx, "xprintidle"); err == nil {
		if ms, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); err == nil && ms >= 0 {
			out.Known = true
			out.IdleSeconds = ms / 1000
		}
	}
	if !out.Known {
		for _, service := range []string{"org.freedesktop.ScreenSaver", "org.gnome.ScreenSaver"} {
			path := "/org/freedesktop/ScreenSaver"
			if service == "org.gnome.ScreenSaver" {
				path = "/org/gnome/ScreenSaver"
			}
			raw, err := run(ctx, "gdbus", "call", "--session", "--dest", service, "--object-path", path, "--method", service+".GetSessionIdleTime")
			if err != nil {
				continue
			}
			if match := busIdle.FindStringSubmatch(strings.TrimSpace(string(raw))); len(match) == 2 {
				if seconds, err := strconv.ParseInt(match[1], 10, 64); err == nil && seconds >= 0 {
					out.Known = true
					out.IdleSeconds = seconds
					break
				}
			}
		}
	}
	if session != "" {
		if raw, err := run(ctx, "loginctl", "show-session", session, "--property=LockedHint", "--value"); err == nil {
			switch strings.TrimSpace(string(raw)) {
			case "yes":
				out.Locked = new(true)
			case "no":
				out.Locked = new(false)
			}
		}
	}
	if raw, err := run(ctx, "xset", "-q"); err == nil {
		if strings.Contains(string(raw), "Monitor is Off") {
			out.DisplayOff = new(true)
		} else if strings.Contains(string(raw), "Monitor is On") {
			out.DisplayOff = new(false)
		}
	}
	return out
}

func sampleDarwin(ctx context.Context, run commandRunner) Sample {
	var out Sample
	if raw, err := run(ctx, "ioreg", "-r", "-c", "IOHIDSystem"); err == nil {
		if match := idleNanoseconds.FindStringSubmatch(string(raw)); len(match) == 2 {
			if ns, err := strconv.ParseInt(match[1], 10, 64); err == nil && ns >= 0 {
				out.Known = true
				out.IdleSeconds = ns / int64(time.Second)
			}
		}
	}
	if raw, err := run(ctx, "ioreg", "-n", "Root", "-d", "1"); err == nil {
		if match := darwinLock.FindStringSubmatch(string(raw)); len(match) == 2 {
			out.Locked = new(match[1] == "Yes" || match[1] == "true")
		}
	}
	if raw, err := run(ctx, "ioreg", "-r", "-c", "IODisplayWrangler"); err == nil {
		state := displayPower.FindStringSubmatch(string(raw))
		if len(state) == 2 {
			value, _ := strconv.Atoi(state[1])
			out.DisplayOff = new(value < 3)
		}
	}
	return out
}
