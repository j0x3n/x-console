package protocol

import "time"

// Methods and events of M2 servers / M3 this PC. See docs/specs/M2-M3.md.
const (
	// EventMetrics is pushed by the agent every 10 seconds. Params: MetricsSample.
	EventMetrics = "metrics"

	MethodProcList = "proc.list" // ProcListParams -> ProcessList
	MethodProcKill = "proc.kill" // ProcKillParams -> nil

	MethodSvcList   = "svc.list"   // nil -> ServiceList
	MethodSvcAction = "svc.action" // SvcActionParams -> nil
	MethodSvcLogs   = "svc.logs"   // SvcLogsParams -> ServiceLogs

	// MethodPTYOpen is a stream. Every chunk in both directions starts with a
	// type byte: PTYFrameData followed by terminal bytes, or PTYFrameResize
	// followed by JSON PTYResize (server -> agent only).
	MethodPTYOpen = "pty.open" // PTYOpenParams

	MethodFilesList   = "files.list"   // FilesListParams -> FileList
	MethodFilesRead   = "files.read"   // stream, FilesReadParams; see below
	MethodFilesWrite  = "files.write"  // stream, FilesWriteParams; see below
	MethodFilesRemove = "files.remove" // FilesRemoveParams -> nil
	MethodFilesMkdir  = "files.mkdir"  // FilesPathParams -> FileEntry
	MethodFilesRename = "files.rename" // FilesRenameParams -> FileEntry

	MethodExecRun = "exec.run" // ExecParams -> ExecResult

	MethodClipboardGet = "clipboard.get" // nil -> Clipboard (desktop only)
	MethodClipboardSet = "clipboard.set" // Clipboard -> nil (desktop only)
	MethodPowerAction  = "power.action"  // PowerParams -> nil (desktop only)
	MethodAppOpen      = "app.open"      // AppOpenParams -> nil (desktop only)
)

// CapOpen is announced by agents that implement MethodAppOpen (desktop only).
const CapOpen = "open"

// Error codes used by the host methods in addition to the standard ones.
const (
	CodeNotFound   = "not_found"
	CodePermission = "permission_denied"
	CodeExists     = "exists"
)

// FileChunkSize is the chunk size of files.read and files.write streams.
const FileChunkSize = 64 << 10

// MetricsSample is one EventMetrics push.
type MetricsSample struct {
	At            time.Time   `json:"at"`
	CPU           float64     `json:"cpu"` // percent of all cores
	CPUPerCore    []float64   `json:"cpuPerCore"`
	MemUsed       uint64      `json:"memUsed"`
	MemTotal      uint64      `json:"memTotal"`
	SwapUsed      uint64      `json:"swapUsed"`
	SwapTotal     uint64      `json:"swapTotal"`
	Disks         []DiskUsage `json:"disks"`
	NetRxRate     float64     `json:"netRx"` // bytes per second, all interfaces except loopback
	NetTxRate     float64     `json:"netTx"`
	DiskReadRate  float64     `json:"diskRead"` // bytes per second
	DiskWriteRate float64     `json:"diskWrite"`
	Load1         float64     `json:"load1"`
	Load5         float64     `json:"load5"`
	Load15        float64     `json:"load15"`
	UptimeSeconds uint64      `json:"uptimeSeconds"`
	Procs         int         `json:"procs"`
}

// DiskUsage is one mounted file system.
type DiskUsage struct {
	Mount  string `json:"mount"`
	FSType string `json:"fsType"`
	Used   uint64 `json:"used"`
	Total  uint64 `json:"total"`
}

// ProcListParams selects processes. Sort is cpu (default), mem, pid or name.
type ProcListParams struct {
	Sort  string `json:"sort,omitempty"`
	Limit int    `json:"limit,omitempty"` // 0 means 200
}

// ProcessInfo is one process.
type ProcessInfo struct {
	PID        int32     `json:"pid"`
	PPID       int32     `json:"ppid"`
	Name       string    `json:"name"`
	User       string    `json:"user"`
	CPU        float64   `json:"cpu"` // percent of one core, sampled over ~0.5s
	MemRSS     uint64    `json:"memRss"`
	MemPercent float64   `json:"memPercent"`
	Cmdline    string    `json:"cmdline"`
	StartedAt  time.Time `json:"startedAt"`
	Status     string    `json:"status"`
}

// ProcessList answers MethodProcList.
type ProcessList struct {
	Items []ProcessInfo `json:"items"`
	Total int           `json:"total"` // number of processes before the limit
}

// ProcKillParams ends a process. Signal is TERM (default), KILL, INT, HUP,
// STOP or CONT on Linux; Windows always terminates.
type ProcKillParams struct {
	PID    int32  `json:"pid"`
	Signal string `json:"signal,omitempty"`
}

// ServiceInfo is one systemd unit or Windows service.
type ServiceInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`    // running, stopped, failed, starting, stopping, other
	SubState    string `json:"subState"` // raw state from the system
	Enabled     bool   `json:"enabled"`  // starts at boot
	StartType   string `json:"startType"`
}

// ServiceList answers MethodSvcList.
type ServiceList struct {
	Items []ServiceInfo `json:"items"`
}

// Service actions.
const (
	SvcStart   = "start"
	SvcStop    = "stop"
	SvcRestart = "restart"
	SvcEnable  = "enable"
	SvcDisable = "disable"
)

// SvcActionParams runs a service action.
type SvcActionParams struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// SvcLogsParams asks for the last Lines log lines (default 200, max 5000).
type SvcLogsParams struct {
	Name  string `json:"name"`
	Lines int    `json:"lines,omitempty"`
}

// ServiceLogs answers MethodSvcLogs.
type ServiceLogs struct {
	Lines []string `json:"lines"`
}

// PTY stream frame types (first byte of every chunk).
const (
	PTYFrameData   byte = 0
	PTYFrameResize byte = 1
)

// PTYOpenParams opens a terminal. Shell empty means the default shell
// ($SHELL or /bin/bash on Linux, PowerShell on Windows).
type PTYOpenParams struct {
	Shell string `json:"shell,omitempty"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	Cwd   string `json:"cwd,omitempty"`
}

// PTYResize is the payload of a PTYFrameResize chunk.
type PTYResize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// FilesListParams lists a directory. Empty Path means the agent user's home.
// On Windows "/" lists the drives.
type FilesListParams struct {
	Path string `json:"path"`
}

// FileEntry is one directory entry.
type FileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Type    string    `json:"type"` // file, dir, symlink, symlink_dir, other
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	Mode    string    `json:"mode"` // for example "-rw-r--r--"
}

// FileList answers MethodFilesList. Parent is empty at the root.
type FileList struct {
	Path    string      `json:"path"`
	Parent  string      `json:"parent"`
	Sep     string      `json:"sep"`
	Entries []FileEntry `json:"entries"`
}

// FilesReadParams starts a download stream. The agent sends one JSON
// FileHeader chunk first, then the content in FileChunkSize chunks, then
// ends the stream.
type FilesReadParams struct {
	Path string `json:"path"`
}

// FileHeader is the first chunk of a files.read stream.
type FileHeader struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// FilesWriteParams starts an upload stream. The server sends exactly Size
// bytes in chunks. The agent writes them to a temporary file next to Path,
// renames it into place, answers with one JSON FileEntry chunk and ends the
// stream. If the server ends the stream early the temporary file is removed.
type FilesWriteParams struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// FilesRemoveParams deletes a file, or a directory when Recursive is set
// (an empty directory is removed without it).
type FilesRemoveParams struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

// FilesPathParams names one path.
type FilesPathParams struct {
	Path string `json:"path"`
}

// FilesRenameParams moves From to To. To must not exist.
type FilesRenameParams struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ExecParams runs a command through the shell (/bin/sh -c on Linux,
// PowerShell on Windows). TimeoutSeconds defaults to 60, maximum 1800.
type ExecParams struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
}

// ExecMaxOutput caps stdout and stderr of exec.run each.
const ExecMaxOutput = 1 << 20

// ExecResult answers MethodExecRun. ExitCode is -1 when the process did not
// exit normally (timeout, signal).
type ExecResult struct {
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	TimedOut   bool   `json:"timedOut"`
	Truncated  bool   `json:"truncated"`
	DurationMs int64  `json:"durationMs"`
}

// Clipboard is the text clipboard.
type Clipboard struct {
	Text string `json:"text"`
}

// Power actions.
const (
	PowerLock     = "lock"
	PowerSleep    = "sleep"
	PowerShutdown = "shutdown"
	PowerRestart  = "restart"
)

// PowerParams runs a power action.
type PowerParams struct {
	Action string `json:"action"`
}

// AppOpenParams opens a program, file or URL with the desktop shell.
type AppOpenParams struct {
	Target string `json:"target"`
	Args   string `json:"args,omitempty"`
}
