// Command x-console-agent runs on a Linux server or the Windows desktop and
// executes requests from x-console-server.
//
//	x-console-agent pair --server https://console.example.com --code ABCD-EFGH [--kind desktop]
//	x-console-agent run [--config path]
//
// On Windows the program under the name x-console-agent-setup-ABCD-EFGH.exe,
// started without arguments, installs itself (B30).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/j0x3n/x-console/backend/internal/agent/clipboard"
	"github.com/j0x3n/x-console/backend/internal/agent/coding"
	"github.com/j0x3n/x-console/backend/internal/agent/config"
	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/agent/docker"
	agentexec "github.com/j0x3n/x-console/backend/internal/agent/exec"
	"github.com/j0x3n/x-console/backend/internal/agent/files"
	"github.com/j0x3n/x-console/backend/internal/agent/metrics"
	"github.com/j0x3n/x-console/backend/internal/agent/netproxy"
	"github.com/j0x3n/x-console/backend/internal/agent/notifyshow"
	"github.com/j0x3n/x-console/backend/internal/agent/power"
	"github.com/j0x3n/x-console/backend/internal/agent/presence"
	"github.com/j0x3n/x-console/backend/internal/agent/proc"
	"github.com/j0x3n/x-console/backend/internal/agent/pty"
	"github.com/j0x3n/x-console/backend/internal/agent/quota"
	"github.com/j0x3n/x-console/backend/internal/agent/screentime"
	"github.com/j0x3n/x-console/backend/internal/agent/setup"
	"github.com/j0x3n/x-console/backend/internal/agent/svc"
	"github.com/j0x3n/x-console/backend/internal/agent/sysinfo"
	"github.com/j0x3n/x-console/backend/internal/agent/syslog"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// Version is set with -ldflags "-X main.Version=...".
var Version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	cmd := "run"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		cmd, args = args[0], args[1:]
	}
	var err error
	if len(args) == 0 && cmd == "run" && setup.IsSetup() {
		// Started by double click under the name the panel gave the download.
		cmd = "setup"
	}
	switch cmd {
	case "setup":
		err = setup.Run(setup.Options{ConfigPath: config.DefaultPath(), Pair: func(server, code string) error {
			return doPair(server, code, config.DefaultPath())
		}})
	case "pair":
		err = pair(args)
	case "run":
		hideOwnConsole() // 任务计划启动时不显示黑窗口
		err = run(args)
	case "version":
		fmt.Println(Version)
	default:
		err = fmt.Errorf("unknown command %q (use pair, run or version)", cmd)
	}
	if err != nil {
		slog.Error("agent failed", "err", err)
		if errors.Is(err, conn.ErrRevoked) {
			os.Exit(3)
		}
		os.Exit(1)
	}
}

func hello() protocol.Hello {
	host, _ := os.Hostname()
	return protocol.Hello{AgentVersion: Version, OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: host, Capabilities: capabilities()}
}

func pair(args []string) error {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	server := fs.String("server", "", "server URL, for example https://console.example.com")
	code := fs.String("code", "", "pairing code shown in the web UI")
	path := fs.String("config", config.DefaultPath(), "config file")
	_ = fs.Parse(args)
	if *server == "" || *code == "" {
		return fmt.Errorf("--server and --code are required")
	}
	return doPair(*server, *code, *path)
}

// doPair exchanges a pairing code for a token and saves the config file.
func doPair(server, code, path string) error {
	id, token, err := conn.Pair(context.Background(), server, code, hello())
	if err != nil {
		return err
	}
	if err := config.Save(path, config.Config{Server: server, AgentID: id, Token: token}); err != nil {
		return err
	}
	fmt.Printf("paired as %s, config saved to %s\n", id, path)
	return nil
}

func run(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	path := fs.String("config", config.DefaultPath(), "config file")
	_ = fs.Parse(args)
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := conn.New(cfg.Server, cfg.Token, hello())
	register(client, cfg)
	return client.Run(ctx)
}

// register wires every feature package. Each module adds one line here and
// its capability in capabilities().
func register(c *conn.Client, cfg config.Config) {
	c.Handle(protocol.MethodPing, sysinfo.Ping)
	c.Handle(protocol.MethodSystemInfo, sysinfo.SystemInfo)
	sysinfo.RegisterAddresses(c)
	presence.Register(c)
	notifyshow.Register(c)
	sysinfo.Info = metrics.SystemInfo                   // M2/M3: full system.info via gopsutil
	metrics.Register(c)                                 // M2/M3: metrics event every 30s or 5s on demand
	proc.Register(c)                                    // M2/M3
	svc.Register(c)                                     // M2/M3
	pty.Register(c)                                     // M2/M3
	files.Register(c)                                   // M2/M3
	agentexec.Register(c)                               // M2/M3
	clipboard.Register(c)                               // M3
	power.Register(c)                                   // M3: power.action and app.open
	c.Handle(protocol.MethodHTTPProxy, netproxy.HTTP)   // M9
	c.HandleStream(protocol.MethodWSProxy, netproxy.WS) // M9
	coding.Register(c, cfg.Coding)                      // M4: also adds the coding capability for configured executor paths
	docker.Register(c)                                  // M10: docker.* over the Engine socket
	syslog.Register(c)                                  // B29: system logs, only when there is something to read
	quota.Register(c)                                   // B110: AI quota readings (Claude, Codex, Grok)
	screentime.Register(c)                              // B116: foreground program, one sample per minute (Windows)
}

// capabilities lists what this build supports on this OS.
func capabilities() []string {
	caps := []string{protocol.CapSystemInfo, protocol.CapPresence}
	if notifyshow.Available() {
		caps = append(caps, protocol.CapNotifyShow)
	}
	// M2/M3: metrics, processes, files and exec work everywhere; terminal
	// and services depend on the system; clipboard, power and open are
	// Windows desktop only.
	caps = append(caps, protocol.CapMetrics, protocol.CapProcesses, protocol.CapFiles, protocol.CapFilesRange, protocol.CapExec)
	caps = append(caps, protocol.CapFilesPrivate)
	if pty.Available() {
		caps = append(caps, protocol.CapPTY)
	}
	if svc.Available() {
		caps = append(caps, protocol.CapServices)
	}
	if clipboard.Available() {
		caps = append(caps, protocol.CapClipboard)
	}
	if power.Available() {
		caps = append(caps, protocol.CapPower, protocol.CapOpen)
	}
	caps = append(caps, protocol.CapProxy) // M9: http.proxy and ws.proxy
	if coding.Available() {                // M4: Windows desktop, or claude/codex on PATH
		caps = append(caps, protocol.CapCoding)
	}
	if syslog.Available() {
		caps = append(caps, protocol.CapSyslog) // B29
	}
	if quota.Available() {
		caps = append(caps, protocol.CapQuota) // B110
	}
	if screentime.Available() {
		caps = append(caps, protocol.CapScreenTime) // B116
	}
	if docker.Available() {
		caps = append(caps, protocol.CapDocker, protocol.CapDockerLines) // M10: only when the Docker socket answers
	}
	return caps
}
