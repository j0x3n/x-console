package cards

import (
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/cards/db"
)

var day0 = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func newDBCard(now time.Time) db.Card { return db.Card{Due: now} }

// apply stores the result of a rating on the card, like reviewCard does.
func apply(t *testing.T, retention float64, c db.Card, rating int, now time.Time) db.Card {
	t.Helper()
	info, err := schedule(newScheduler(retention), c, rating, now)
	if err != nil {
		t.Fatal(err)
	}
	n := info.Card
	last := n.LastReview
	c.State, c.Stability, c.Difficulty, c.Due = int64(n.State), n.Stability, n.Difficulty, n.Due
	c.LastReview, c.ScheduledDays, c.Reps, c.Lapses = &last, int64(n.ScheduledDays), int64(n.Reps), int64(n.Lapses)
	return c
}

// A new card at 90% retention. The first stability of FSRS-6 with its default
// weights is 0.212, 1.2931, 2.3065 and 8.2956 days for the four ratings. The
// interval at 90% equals the stability, rounded to whole days with a floor of
// one. The long-term scheduler then makes each rating at least one day longer
// than the one before: Hard 1 -> 2, Good 2 -> 3, Easy 8.
func TestNewCardIntervals(t *testing.T) {
	iv, err := intervals(newScheduler(0.9), newDBCard(day0), day0)
	if err != nil {
		t.Fatal(err)
	}
	if iv.Again != 1 || iv.Hard != 2 || iv.Good != 3 || iv.Easy != 8 {
		t.Fatalf("intervals of a new card: %+v", iv)
	}
}

func TestRatingsGiveLongerIntervals(t *testing.T) {
	// A card that has been learned and is now due.
	c := apply(t, 0.9, newDBCard(day0), 3, day0)
	due := c.Due
	if want := day0.AddDate(0, 0, 3); !due.Equal(want) {
		t.Fatalf("due after a first Good: %v, want %v", due, want)
	}
	iv, err := intervals(newScheduler(0.9), c, due)
	if err != nil {
		t.Fatal(err)
	}
	if !(0 < iv.Again && iv.Again < iv.Hard && iv.Hard < iv.Good && iv.Good < iv.Easy) {
		t.Fatalf("intervals not increasing: %+v", iv)
	}
	if iv.Good <= 3 {
		t.Fatalf("a second Good must wait longer than the first: %+v", iv)
	}
	var dues [4]time.Time
	for r := 1; r <= 4; r++ {
		dues[r-1] = apply(t, 0.9, c, r, due).Due
	}
	for i := 1; i < 4; i++ {
		if !dues[i].After(dues[i-1]) {
			t.Fatalf("due dates not increasing: %v", dues)
		}
	}
}

func TestAgainCountsALapse(t *testing.T) {
	c := apply(t, 0.9, newDBCard(day0), 3, day0)
	failed := apply(t, 0.9, c, 1, c.Due)
	if failed.Lapses != 1 || failed.Reps != 2 {
		t.Fatalf("lapses=%d reps=%d", failed.Lapses, failed.Reps)
	}
	if failed.Stability >= c.Stability {
		t.Fatalf("stability must drop after Again: %v -> %v", c.Stability, failed.Stability)
	}
	good := apply(t, 0.9, c, 3, c.Due)
	if good.Stability <= c.Stability || good.Lapses != 0 {
		t.Fatalf("stability must grow after Good: %v -> %v", c.Stability, good.Stability)
	}
}

func TestHigherRetentionShortensIntervals(t *testing.T) {
	c := apply(t, 0.9, newDBCard(day0), 3, day0)
	low, err := intervals(newScheduler(0.8), c, c.Due)
	if err != nil {
		t.Fatal(err)
	}
	high, err := intervals(newScheduler(0.95), c, c.Due)
	if err != nil {
		t.Fatal(err)
	}
	if low.Good <= high.Good {
		t.Fatalf("80%% should wait longer than 95%%: %+v vs %+v", low, high)
	}
}
