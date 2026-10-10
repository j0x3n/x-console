package contracts

import (
	"context"
	"time"
)

// ActivitySourcePrefix is the registry key prefix of the modules that tell the
// journal (B118) what happened on a day. The last part of the key is the
// source name, for example "journal.sources.projects".
const ActivitySourcePrefix = "journal.sources."

// Activity is one thing that happened, as the journal shows it.
type Activity struct {
	// Ref is unique inside the source, for example "issue:12". The journal
	// stores an item once per (source, Ref).
	Ref string
	// Module is the hideable module the item belongs to (projects, coding,
	// habits, calendar, ...). While hidden content is locked the journal
	// leaves the item out.
	Module string
	// Kind is card, task, commit, pr, focus, habit, workout, event, alert,
	// screen, link or note.
	Kind  string
	At    time.Time
	Title string // one line
	// Detail is an optional second line.
	Detail string
	// Link is an in-app path or an http(s) address.
	Link string
	// Minutes is set for items with a length: focus, screen time.
	Minutes int
}

// ActivitySource is offered by modules that have something to say about a
// day. It does not check hidden modules and does not depend on the journal.
type ActivitySource interface {
	// Activity returns what happened in [from, until).
	Activity(ctx context.Context, from, until time.Time) ([]Activity, error)
}
