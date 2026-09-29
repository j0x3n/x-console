package protocol

import "time"

// Methods of B29 system logs. The agent announces CapSyslog when it can read
// the systemd journal, a syslog file or the Windows event log.
const (
	MethodSyslogQuery  = "syslog.query"  // SyslogQueryParams -> SyslogPage
	MethodSyslogUnits  = "syslog.units"  // nil -> SyslogUnits
	MethodSyslogFollow = "syslog.follow" // stream, SyslogFollowParams; every frame is a JSON array of SyslogEntry

	// CodeSyslogPermission: the agent may not read the logs. The message says
	// how to give it the permission.
	CodeSyslogPermission = "syslog_permission"
)

// SyslogQueryParams selects log lines. Zero fields do not filter. Items come
// back in time order, oldest first, and are the newest Limit ones that match.
type SyslogQueryParams struct {
	Since *time.Time `json:"since,omitempty"`
	Until *time.Time `json:"until,omitempty"`
	// Priority is the lowest level wanted, 0 to 7: only entries with a number
	// less than or equal to it match. Nil means all.
	Priority *int   `json:"priority,omitempty"`
	Unit     string `json:"unit,omitempty"` // systemd unit, syslog program or Windows event source
	Grep     string `json:"grep,omitempty"` // case-insensitive keyword
	Limit    int    `json:"limit,omitempty"`
	// Cursor is SyslogPage.Cursor of the previous page and asks for the page
	// before it.
	Cursor string `json:"cursor,omitempty"`
}

// SyslogEntry is one log line.
type SyslogEntry struct {
	Time     time.Time `json:"time"`
	Priority int       `json:"priority"`
	Unit     string    `json:"unit"`
	PID      int       `json:"pid,omitempty"`
	Message  string    `json:"message"`
}

// SyslogPage answers MethodSyslogQuery. Cursor is set when older entries exist.
type SyslogPage struct {
	Items  []SyslogEntry `json:"items"`
	Cursor string        `json:"cursor,omitempty"`
}

// SyslogUnits answers MethodSyslogUnits: units of the last 7 days, most
// entries first, at most 300.
type SyslogUnits struct {
	Items []string `json:"items"`
}

// SyslogFollowParams starts a syslog.follow stream of new entries.
type SyslogFollowParams struct {
	Priority *int   `json:"priority,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Grep     string `json:"grep,omitempty"`
}
