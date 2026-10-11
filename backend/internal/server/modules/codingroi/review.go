package codingroi

import (
	"slices"
	"sort"
	"time"
)

// reviewCap is the longest review window of one task. A task nobody
// merges or discards would otherwise collect all coding time from then on.
const reviewCap = 72 * time.Hour

// window is a stretch of time in which the user looked at one task's result.
type window struct{ Start, End time.Time }

// screenMinute is one minute with a computer time record. Minute is the Unix
// minute number. Coding is true when the program in front was a coding one.
type screenMinute struct {
	Minute int64
	Coding bool
}

// reviewResult is the review time of one task. HasData is false when no
// computer time was recorded in any of its windows.
type reviewResult struct {
	Minutes float64
	HasData bool
}

// capWindow limits a window to reviewCap. The second result says whether it
// was cut.
func capWindow(w window) (window, bool) {
	if w.End.Before(w.Start) {
		w.End = w.Start
	}
	if w.End.Sub(w.Start) > reviewCap {
		return window{Start: w.Start, End: w.Start.Add(reviewCap)}, true
	}
	return w, false
}

// mergeWindows sorts windows and joins the ones that overlap, so a minute is
// never counted twice for one task.
func mergeWindows(in []window) []window {
	out := slices.Clone(in)
	out = slices.DeleteFunc(out, func(w window) bool { return !w.End.After(w.Start) })
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	var merged []window
	for _, w := range out {
		if n := len(merged); n > 0 && !w.Start.After(merged[n-1].End) {
			if w.End.After(merged[n-1].End) {
				merged[n-1].End = w.End
			}
			continue
		}
		merged = append(merged, w)
	}
	return merged
}

// allocateReview adds up the coding minutes inside each task's windows. mins
// must be sorted by Minute. A minute inside the windows of several tasks is
// split equally between them, so the tasks together never get more than the
// minutes that really exist.
func allocateReview(wins map[int64][]window, mins []screenMinute) map[int64]reviewResult {
	type span struct{ lo, hi int }
	spans := map[int64][]span{}
	cover := make([]int, len(mins))
	for id, list := range wins {
		for _, w := range mergeWindows(list) {
			start, end := w.Start.Unix(), w.End.Unix()
			lo := sort.Search(len(mins), func(i int) bool { return mins[i].Minute*60 >= start })
			hi := sort.Search(len(mins), func(i int) bool { return mins[i].Minute*60 >= end })
			if hi <= lo {
				continue
			}
			spans[id] = append(spans[id], span{lo, hi})
			for i := lo; i < hi; i++ {
				cover[i]++
			}
		}
	}
	out := make(map[int64]reviewResult, len(wins))
	for id := range wins {
		var r reviewResult
		for _, s := range spans[id] {
			r.HasData = true
			for i := s.lo; i < s.hi; i++ {
				if mins[i].Coding {
					r.Minutes += 1 / float64(cover[i])
				}
			}
		}
		out[id] = r
	}
	return out
}
