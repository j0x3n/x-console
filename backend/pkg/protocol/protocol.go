// Package protocol defines the messages exchanged between x-console-server
// and x-console-agent over one WebSocket connection. See docs/04-agent-protocol.md.
//
// Every frame is one JSON Envelope. Four patterns share the connection:
//
//   - Request/response: server sends Req{ID, Method, Params}; agent answers
//     Res{ID, Result} or Res{ID, Error}. Server may send Cancel{ID}.
//   - Streams (terminal, logs, coding output): server sends Open{ID, Method,
//     Params}; both sides send Data{ID, Data}; either side ends with End{ID, Error?}.
//   - Events: agent sends Event{Method, Params} without an ID, for example
//     periodic metrics.
//   - Liveness: WebSocket ping/pong frames, handled by the transport.
package protocol

import (
	"encoding/json"
	"fmt"
)

// Version is bumped on incompatible changes. Server and agent exchange it in
// Hello/Welcome and the server refuses agents with a different major version.
const Version = 1

// Kind is the frame type.
type Kind string

const (
	KindHello   Kind = "hello"   // agent -> server, first frame
	KindWelcome Kind = "welcome" // server -> agent, answer to hello
	KindReq     Kind = "req"
	KindRes     Kind = "res"
	KindCancel  Kind = "cancel"
	KindOpen    Kind = "open"
	KindData    Kind = "data"
	KindEnd     Kind = "end"
	KindEvent   Kind = "event"
)

// Envelope is one frame.
type Envelope struct {
	Kind   Kind            `json:"k"`
	ID     string          `json:"id,omitempty"`
	Method string          `json:"m,omitempty"`
	Params json.RawMessage `json:"p,omitempty"`
	Result json.RawMessage `json:"r,omitempty"`
	Error  *Error          `json:"e,omitempty"`
	Data   []byte          `json:"d,omitempty"`
}

// Error is a failure reported by the agent.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Standard error codes.
const (
	CodeUnknownMethod = "unknown_method"
	CodeBadParams     = "bad_params"
	CodeFailed        = "failed"
	CodeCanceled      = "canceled"
	CodeTimeout       = "timeout"
	CodeUnsupported   = "unsupported" // capability missing on this OS
)

// Hello is the agent's first frame.
type Hello struct {
	ProtocolVersion int      `json:"protocolVersion"`
	AgentVersion    string   `json:"agentVersion"`
	OS              string   `json:"os"`
	Arch            string   `json:"arch"`
	Hostname        string   `json:"hostname"`
	Capabilities    []string `json:"capabilities"`
}

// Welcome is the server's answer to Hello.
type Welcome struct {
	ProtocolVersion int    `json:"protocolVersion"`
	AgentID         string `json:"agentId"`
	Name            string `json:"name"`
}

// Capabilities. The agent reports what it supports on its OS; the UI hides
// actions the agent cannot do.
const (
	CapSystemInfo = "system.info"
	CapMetrics    = "metrics"
	CapProcesses  = "processes"
	CapServices   = "services" // systemd on Linux, SCM on Windows
	CapDocker     = "docker"
	// CapDockerLines: docker.logs understands DockerLogsParams.Lines, and the
	// docker.image_remove and docker.image_prune methods exist (B28).
	CapDockerLines = "docker.lines"
	CapSyslog      = "syslog" // B29: systemd journal, syslog files or the Windows event log
	CapPTY         = "pty"
	CapFiles       = "files"
	CapFilesRange  = "files.range"
	// CapFilesPrivate: files.write understands FilesWriteParams.Private.
	CapFilesPrivate = "files.private"
	CapExec         = "exec"
	CapClipboard    = "clipboard" // desktop only
	CapPower        = "power"     // lock/sleep/shutdown, desktop only
	CapCoding       = "coding"    // Claude Code / Codex runner, desktop only
)

// Methods implemented in batch 0. Modules add their own method constants in
// their own files in this package (methods_<module>.go).
const (
	MethodPing       = "ping"        // params: none, result: Pong
	MethodSystemInfo = "system.info" // params: none, result: SystemInfo
)

// Pong answers MethodPing.
type Pong struct {
	Time string `json:"time"`
}

// SystemInfo answers MethodSystemInfo.
type SystemInfo struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Platform      string `json:"platform"`
	PlatformVer   string `json:"platformVersion"`
	KernelVersion string `json:"kernelVersion"`
	Arch          string `json:"arch"`
	CPUModel      string `json:"cpuModel"`
	CPUCores      int    `json:"cpuCores"`
	MemoryTotal   uint64 `json:"memoryTotal"`
	UptimeSeconds uint64 `json:"uptimeSeconds"`
}

// Marshal is json.Marshal that panics on error; params are always plain structs.
func Marshal(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}
