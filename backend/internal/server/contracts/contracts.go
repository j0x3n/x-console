// Package contracts defines the interfaces modules offer each other.
//
// A provider registers its implementation in New:
//
//	module.Provide[contracts.Issues](d.Registry, contracts.IssuesKey, svc)
//
// A consumer looks it up when it needs it (not in New, the provider may be
// built later). A missing provider means that feature is not available:
//
//	issues, ok := module.Lookup[contracts.Issues](m.d.Registry, contracts.IssuesKey)
//	if !ok { return httpx.NewError(501, "feature_unavailable", "项目模块未启用") }
//
// Keep these types small and stable. Changing a signature here affects
// several modules; record the change in docs/tasks.md first.
package contracts

import (
	"context"
	"time"
)

// Registry keys.
const (
	IssuesKey        = "projects.issues"     // M5 provides
	IssueSyncKey     = "projects.sync"       // M5 provides, M13 (Linear) uses
	NotesKey         = "notes.notes"         // M6 provides
	RemindersKey     = "reminders.reminders" // M7 provides
	HabitsKey        = "habits.habits"       // M8 provides
	HostsKey         = "hosts.hosts"         // M2/M3 provides
	RenewalsKey      = "monitoring.renewals" // M10 provides
	HomeAssistantKey = "homeassistant.ha"    // M9 provides
	CodingKey        = "coding.launcher"     // M4 provides
	CalendarKey      = "calendar.calendar"   // M11 provides
	GitHubKey        = "github.github"       // M13 provides
)

// ---- M5 projects ----

// IssueRef is the cross-module view of an issue.
type IssueRef struct {
	ID          int64      `json:"id"`
	Key         string     `json:"key"` // for example "XC-12"
	ProjectID   int64      `json:"projectId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"` // backlog, todo, in_progress, in_review, done, canceled
	Priority    int        `json:"priority"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
}

// IssueLink attaches something external to an issue.
type IssueLink struct {
	Kind  string `json:"kind"` // "pull_request", "coding_task", "note", "url"
	Title string `json:"title"`
	URL   string `json:"url"` // absolute URL or in-app path such as /coding/12
	Ref   string `json:"ref"` // external id, for example "owner/repo#34" or coding task id
}

// CreateIssue is the input of Issues.Create.
type CreateIssue struct {
	ProjectID   int64      `json:"projectId"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    int        `json:"priority"`
	DueDate     *time.Time `json:"dueDate,omitempty"`
}

// Issues is provided by M5.
type Issues interface {
	Get(ctx context.Context, key string) (IssueRef, error)
	Create(ctx context.Context, in CreateIssue) (IssueRef, error)
	SetStatus(ctx context.Context, key, status string) error
	AttachLink(ctx context.Context, key string, link IssueLink) error
	// ListDue returns open issues due before until, soonest first.
	ListDue(ctx context.Context, until time.Time) ([]IssueRef, error)
}

// SyncIssue is an issue as seen by an external sync (M13 Linear).
type SyncIssue struct {
	ProjectID      int64      `json:"projectId"`
	Key            string     `json:"key"` // empty when creating
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	Priority       int        `json:"priority"`
	DueDate        *time.Time `json:"dueDate,omitempty"`
	ExternalSource string     `json:"externalSource"` // "linear"
	ExternalID     string     `json:"externalId"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// IssueSync is provided by M5 for two-way sync with external trackers.
type IssueSync interface {
	// FindByExternal returns the local issue linked to an external id.
	FindByExternal(ctx context.Context, source, externalID string) (SyncIssue, bool, error)
	// Upsert creates or updates the issue identified by (ExternalSource,
	// ExternalID) and keeps UpdatedAt as given, so the next sync does not
	// echo the change back. It must not publish an event that triggers a
	// push to the same source.
	Upsert(ctx context.Context, in SyncIssue) (SyncIssue, error)
	// Link stores the external id on an existing local issue.
	Link(ctx context.Context, key, source, externalID string) error
	// ChangedSince lists issues in the given projects updated after since.
	ChangedSince(ctx context.Context, projectIDs []int64, since time.Time) ([]SyncIssue, error)
}

// ---- M6 notes ----

// Notes is provided by M6.
type Notes interface {
	Create(ctx context.Context, title, body string, tags []string) (int64, error)
}

// ---- M7 reminders ----

// CreateReminder is the input of Reminders.Create.
type CreateReminder struct {
	Title string    `json:"title"`
	Body  string    `json:"body"`
	At    time.Time `json:"at"`
	// RRule makes it repeat, for example "FREQ=DAILY;BYHOUR=9". Empty means once.
	RRule string `json:"rrule"`
	Link  string `json:"link"`
}

// ReminderRef is the cross-module view of a reminder occurrence.
type ReminderRef struct {
	ID    int64     `json:"id"`
	Title string    `json:"title"`
	At    time.Time `json:"at"`
	Link  string    `json:"link"`
}

// Reminders is provided by M7.
type Reminders interface {
	Create(ctx context.Context, in CreateReminder) (int64, error)
	Upcoming(ctx context.Context, until time.Time) ([]ReminderRef, error)
}

// ---- M10 monitoring ----

// RenewalRef is a subscription's next renewal. Date is a civil date at UTC
// midnight; consumers must not shift it into the user's time zone.
type RenewalRef struct {
	Name     string
	Date     time.Time
	Amount   float64
	Currency string
}

// Renewals is provided by M10. Upcoming includes overdue, unarchived
// subscriptions and excludes dates on or after until.
type Renewals interface {
	Upcoming(ctx context.Context, until time.Time) ([]RenewalRef, error)
}

// ---- M8 habits ----

// HabitProgress is today's state of one habit.
type HabitProgress struct {
	ID     int64   `json:"id"`
	Name   string  `json:"name"`
	Unit   string  `json:"unit"`
	Target float64 `json:"target"`
	Done   float64 `json:"done"`
	Streak int     `json:"streak"`
}

// Habits is provided by M8.
type Habits interface {
	Checkin(ctx context.Context, habitID int64, amount float64, source string) error
	Today(ctx context.Context) ([]HabitProgress, error)
}

// ---- M2/M3 hosts ----

// HostSummary is the latest state of a machine.
type HostSummary struct {
	ID       string     `json:"id"` // agent id, or "ssh:<id>" for SSH-only hosts
	Name     string     `json:"name"`
	Kind     string     `json:"kind"` // server, desktop
	Online   bool       `json:"online"`
	CPU      float64    `json:"cpu"`      // percent
	Memory   float64    `json:"memory"`   // percent
	Disk     float64    `json:"disk"`     // percent of the fullest mount
	LastSeen *time.Time `json:"lastSeen"` // nil if never seen
}

// HostAlert is a fired alert.
type HostAlert struct {
	HostID   string     `json:"hostId"`
	HostName string     `json:"hostName"`
	Rule     string     `json:"rule"`
	Message  string     `json:"message"`
	FiredAt  time.Time  `json:"firedAt"`
	Resolved *time.Time `json:"resolvedAt,omitempty"`
}

// Hosts is provided by M2/M3.
type Hosts interface {
	Summaries(ctx context.Context) ([]HostSummary, error)
	Alerts(ctx context.Context, since time.Time) ([]HostAlert, error)
	// Exec runs a shell command on a host and waits for it (used by scripts
	// and automations). Callers must check elevation or confirmation first.
	Exec(ctx context.Context, hostID, command string, timeout time.Duration) (ExecResult, error)
}

// ExecResult is the outcome of Hosts.Exec.
type ExecResult struct {
	ExitCode int    `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ---- M9 Home Assistant ----

// HAState is one entity state.
type HAState struct {
	EntityID    string         `json:"entityId"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged time.Time      `json:"lastChanged"`
}

// HomeAssistant is provided by M9. It publishes "ha.state_changed" events
// with HAState payloads on the bus, for favorite entities and for entities
// registered with WatchEntity.
type HomeAssistant interface {
	State(ctx context.Context, entityID string) (HAState, error)
	CallService(ctx context.Context, domain, service string, data map[string]any) error
	// WatchEntity makes changes of entityID appear on the bus. Habits (M8)
	// and automations (M12) call it for the entities they react to.
	WatchEntity(entityID string)
}

// ---- M4 coding ----

// LaunchCoding is the input of Coding.Launch.
type LaunchCoding struct {
	RepoID     int64  `json:"repoId"`
	Executor   string `json:"executor"` // "claude" or "codex"
	Prompt     string `json:"prompt"`
	BaseBranch string `json:"baseBranch"` // empty means the repo default
	IssueKey   string `json:"issueKey"`   // optional
}

// Coding is provided by M4.
type Coding interface {
	Launch(ctx context.Context, in LaunchCoding) (taskID int64, err error)
}

// ---- M11 calendar ----

// CalendarEvent is one event occurrence.
type CalendarEvent struct {
	Title    string    `json:"title"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	AllDay   bool      `json:"allDay"`
	Location string    `json:"location"`
	Calendar string    `json:"calendar"`
}

// Calendar is provided by M11.
type Calendar interface {
	Events(ctx context.Context, from, to time.Time) ([]CalendarEvent, error)
}

// ---- M13 GitHub ----

// CreatePR is the input of GitHub.CreatePR.
type CreatePR struct {
	Repo  string `json:"repo"` // "owner/name"
	Head  string `json:"head"`
	Base  string `json:"base"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Draft bool   `json:"draft"`
}

// GitHub is provided by M13.
type GitHub interface {
	CreatePR(ctx context.Context, in CreatePR) (url string, number int, err error)
}
