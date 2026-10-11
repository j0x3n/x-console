package contracts

import (
	"context"
	"time"
)

// FocusStateKey is where the focus module registers FocusState (B150).
const FocusStateKey = "focus.state"

// FocusState lets other modules know whether a pomodoro is running, for
// example to tell which songs were played during a focus session.
type FocusState interface {
	// CurrentSessionID returns the id of the running session, if any.
	CurrentSessionID(ctx context.Context) (id int64, ok bool)
}

// FocusCurrentKey is where the focus module registers FocusCurrent (B131).
const FocusCurrentKey = "focus.current"

// FocusRunning is the pomodoro that is open now.
type FocusRunning struct {
	ID        int64
	StartedAt time.Time
	// EndsAt is the planned end. The session stays open after it until the
	// user stops it, or until it is closed an hour later.
	EndsAt time.Time
}

// FocusCurrent tells the running pomodoro with its times, for modules that
// act while it runs (the site and program block, B131).
type FocusCurrent interface {
	// Current returns the open session, if any.
	Current(ctx context.Context) (FocusRunning, bool)
}
