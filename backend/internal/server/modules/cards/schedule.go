package cards

import (
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v4"

	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/db"
)

// The scheduler is the FSRS library with its default weights. It runs the
// long-term variant: every interval is whole days, so a card comes back at
// most once a day, and the four ratings give strictly longer intervals.
// Fuzz is off, so the result only depends on the card, the time and the
// rating.

// newScheduler builds the FSRS scheduler for a target retention.
func newScheduler(retention float64) *fsrs.FSRS {
	p := fsrs.DefaultParam()
	p.RequestRetention = retention
	p.EnableShortTerm = false
	p.EnableFuzz = false
	return fsrs.NewFSRS(p)
}

// stateNames maps the stored FSRS state to the API name.
var stateNames = map[fsrs.State]api.CardState{
	fsrs.New:        api.New,
	fsrs.Learning:   api.Learning,
	fsrs.Review:     api.Review,
	fsrs.Relearning: api.Relearning,
}

// stateValues is the reverse of stateNames, for the list filter.
func stateValue(s api.CardState) (int64, bool) {
	for v, name := range stateNames {
		if name == s {
			return int64(v), true
		}
	}
	return 0, false
}

// fsrsCard turns a stored card into the library's card.
func fsrsCard(c db.Card) fsrs.Card {
	out := fsrs.Card{
		Due:           c.Due,
		Stability:     c.Stability,
		Difficulty:    c.Difficulty,
		ScheduledDays: uint64(max(c.ScheduledDays, 0)),
		Reps:          uint64(max(c.Reps, 0)),
		Lapses:        uint64(max(c.Lapses, 0)),
		State:         fsrs.State(c.State),
	}
	if c.LastReview != nil {
		out.LastReview = *c.LastReview
	}
	return out
}

// intervals previews the four ratings of a card, in days.
func intervals(f *fsrs.FSRS, c db.Card, now time.Time) (api.CardIntervals, error) {
	log, err := f.Repeat(fsrsCard(c), now)
	if err != nil {
		return api.CardIntervals{}, err
	}
	return api.CardIntervals{
		Again: int(log[fsrs.Again].Card.ScheduledDays),
		Hard:  int(log[fsrs.Hard].Card.ScheduledDays),
		Good:  int(log[fsrs.Good].Card.ScheduledDays),
		Easy:  int(log[fsrs.Easy].Card.ScheduledDays),
	}, nil
}

// schedule applies one rating and returns the card's new state.
func schedule(f *fsrs.FSRS, c db.Card, rating int, now time.Time) (fsrs.SchedulingInfo, error) {
	return f.Next(fsrsCard(c), now, fsrs.Rating(rating))
}
