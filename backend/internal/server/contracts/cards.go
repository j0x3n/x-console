package contracts

import "context"

// CardsDueKey is where the review cards module (B130) registers itself.
const CardsDueKey = "cards.due"

// CardsDue tells how many review cards are waiting today. The brief uses it
// for its "cards to review" section.
type CardsDue interface {
	// DueCount is the number of cards still to review today, after the daily
	// limits. Suspended cards are not counted.
	DueCount(ctx context.Context) (int, error)
}
