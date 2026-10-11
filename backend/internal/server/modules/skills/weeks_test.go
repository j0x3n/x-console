package skills

import (
	"reflect"
	"testing"
	"time"
)

func TestMondayAndWeekStarts(t *testing.T) {
	// 2026-11-11 is a Wednesday, 2026-11-15 a Sunday, 2026-11-16 a Monday.
	for day, want := range map[string]string{
		"2026-11-11": "2026-11-09",
		"2026-11-15": "2026-11-09",
		"2026-11-16": "2026-11-16",
		"2026-11-09": "2026-11-09",
		"2026-03-01": "2026-02-23", // across a month boundary
	} {
		d, _ := time.Parse(dateLayout, day)
		if got := monday(d).Format(dateLayout); got != want {
			t.Errorf("monday(%s) = %s, want %s", day, got, want)
		}
	}
	now, _ := time.Parse(dateLayout, "2026-11-11")
	starts := weekStarts(now)
	if len(starts) != weekCount || starts[weekCount-1] != "2026-11-09" || starts[0] != "2026-09-21" {
		t.Fatalf("starts: %v", starts)
	}
}

func TestBucketWeeks(t *testing.T) {
	starts := []string{"2026-11-02", "2026-11-09", "2026-11-16"}
	got := bucketWeeks(starts, map[string]int{
		"2026-10-30": 99, // before the first week: left out
		"2026-11-02": 10,
		"2026-11-08": 5, // Sunday still belongs to the first week
		"2026-11-09": 20,
		"2026-11-18": 7,
	})
	if want := []int{15, 20, 7}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := bucketWeeks(starts, nil); !reflect.DeepEqual(got, []int{0, 0, 0}) {
		t.Fatalf("empty: %v", got)
	}
}
