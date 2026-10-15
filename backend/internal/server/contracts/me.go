package contracts

import "time"

// MeKey is the registry key of the "me" module's state (B162).
const MeKey = "me.state"

// The six states of Me.
const (
	MeWorking  = "working"
	MeAway     = "away"
	MeSleeping = "sleeping"
	MeOut      = "out"
	MeHome     = "home"
	MeUnknown  = "unknown"
)

// MeState says what the user is doing now: working at a computer, away from
// it, asleep, out of the house, at home, or unknown.
type MeState struct {
	State   string    `json:"state"` // working away sleeping out home unknown
	Since   time.Time `json:"since"`
	Guessed bool      `json:"guessed"` // worked out from the schedule, not reported
	Manual  bool      `json:"manual"`
	Source  string    `json:"source"`           // pc phone ha manual schedule
	AtHome  *bool     `json:"atHome,omitempty"` // empty when no location was ever reported
	// B164: the panel was open on the phone in the last two minutes. It does
	// not change State.
	PhoneActive bool `json:"phoneActive"`
	// B164: the work computer in use while working, the one with the latest
	// input when several are.
	Host string `json:"host,omitempty"`
}

// MeWoke is the payload of the "me.woke" event: a sleep of at least three
// hours ended.
type MeWoke struct {
	SleptAt time.Time `json:"sleptAt"`
	WokeAt  time.Time `json:"wokeAt"`
	Guessed bool      `json:"guessed"`
}

// Me is provided by the me module. Subscribe delivers every change of the
// state; the cancel function must be called when done.
type Me interface {
	Current() MeState
	Subscribe() (<-chan MeState, func())
}
