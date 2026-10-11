package contracts

import "context"

// FocusStateKey is where the focus module registers FocusState (B150).
const FocusStateKey = "focus.state"

// FocusState lets other modules know whether a pomodoro is running, for
// example to tell which songs were played during a focus session.
type FocusState interface {
	// CurrentSessionID returns the id of the running session, if any.
	CurrentSessionID(ctx context.Context) (id int64, ok bool)
}
