package dayplan

import (
	"fmt"
	"sort"
	"strings"
)

// Everything here is plain computation on minutes since midnight, so it can
// be tested without a clock, a database or the other modules.

// span is a stretch of the day, [Start, End) in minutes since midnight.
type span struct{ Start, End int }

const (
	minSlot       = 30 // the shortest free stretch worth planning in
	roundTo       = 15 // "from now" starts at a multiple of this
	defaultIssue  = 60
	defaultRemind = 30
	maxPlanned    = 3 // what the planner picks
	maxItems      = 8 // what a plan may hold
	maxCandidates = 8 // what the AI is shown
)

func fmtHM(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// parseHM reads HH:MM. ok is false for anything else.
func parseHM(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	if h > 23 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// freeSlots is the work window from `from` on, minus the busy spans, keeping
// the stretches of at least minSlot minutes.
func freeSlots(workStart, workEnd, from int, busy []span) []span {
	start := max(workStart, from)
	if start >= workEnd {
		return nil
	}
	b := append([]span(nil), busy...)
	sort.Slice(b, func(i, j int) bool { return b[i].Start < b[j].Start })
	var out []span
	cur := start
	for _, s := range b {
		if s.End <= cur {
			continue
		}
		if s.Start >= workEnd {
			break
		}
		if s.Start > cur && s.Start-cur >= minSlot {
			out = append(out, span{cur, s.Start})
		}
		cur = max(cur, s.End)
	}
	if workEnd-cur >= minSlot {
		out = append(out, span{cur, workEnd})
	}
	return out
}

// roundUp rounds minutes up to a multiple of roundTo.
func roundUp(min int) int { return (min + roundTo - 1) / roundTo * roundTo }

// candidate is something that could go into the plan.
type candidate struct {
	Title    string
	Kind     string // issue, reminder, carry
	Ref      string
	Detail   string // one line for the AI: why it matters
	Score    int
	At       int // preferred start (minutes), -1 when free
	Duration int
}

// rank orders candidates by score, highest first, keeping the order of equal ones.
func rank(c []candidate) []candidate {
	out := append([]candidate(nil), c...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// planned is a candidate with its place in the day. Start and End are -1
// when no free stretch could take it.
type planned struct {
	candidate
	Start, End int
	Reason     string
}

// take cuts [start, end) out of the slot list.
func take(slots []span, start, end int) []span {
	var out []span
	for _, s := range slots {
		if end <= s.Start || start >= s.End {
			out = append(out, s)
			continue
		}
		if start-s.Start >= minSlot {
			out = append(out, span{s.Start, start})
		}
		if s.End-end >= minSlot {
			out = append(out, span{end, s.End})
		}
	}
	return out
}

func fits(slots []span, start, end int) bool {
	for _, s := range slots {
		if start >= s.Start && end <= s.End {
			return true
		}
	}
	return false
}

// pack puts the best `limit` candidates into the free slots: a candidate with
// a fixed time goes there when that stretch is free, the others into the
// earliest stretch that is long enough. What does not fit stays in the plan
// without a time.
func pack(cands []candidate, slots []span, limit int) []planned {
	free := append([]span(nil), slots...)
	var out []planned
	for _, c := range rank(cands) {
		if len(out) == limit {
			break
		}
		dur := c.Duration
		if dur <= 0 {
			dur = defaultIssue
		}
		p := planned{candidate: c, Start: -1, End: -1}
		if c.At >= 0 && fits(free, c.At, c.At+dur) {
			p.Start, p.End = c.At, c.At+dur
		} else {
			for _, s := range free {
				if s.End-s.Start >= dur {
					p.Start, p.End = s.Start, s.Start+dur
					break
				}
			}
		}
		if p.Start >= 0 {
			free = take(free, p.Start, p.End)
		}
		out = append(out, p)
	}
	return sortPlanned(out)
}

// sortPlanned puts the items with a time first, by time; the others keep their order after them.
func sortPlanned(p []planned) []planned {
	sort.SliceStable(p, func(i, j int) bool {
		a, b := p[i].Start, p[j].Start
		switch {
		case a >= 0 && b >= 0:
			return a < b
		case a >= 0:
			return true
		}
		return false
	})
	return p
}

// aiChoice is what the model returns for one item.
type aiChoice struct {
	Candidate int    `json:"candidate"`
	Start     string `json:"start"`
	End       string `json:"end"`
	Reason    string `json:"reason"`
}

// validateChoices turns the model's answer into a plan, or reports ok=false
// when anything in it cannot be trusted: an unknown or repeated candidate,
// a bad time, a stretch that is not free, or too many items.
func validateChoices(choices []aiChoice, cands []candidate, slots []span, limit int) ([]planned, bool) {
	if len(choices) == 0 || len(choices) > limit {
		return nil, false
	}
	free := append([]span(nil), slots...)
	used := map[int]bool{}
	var out []planned
	for _, ch := range choices {
		if ch.Candidate < 0 || ch.Candidate >= len(cands) || used[ch.Candidate] {
			return nil, false
		}
		used[ch.Candidate] = true
		p := planned{candidate: cands[ch.Candidate], Start: -1, End: -1, Reason: strings.TrimSpace(ch.Reason)}
		if ch.Start != "" || ch.End != "" {
			s, ok1 := parseHM(ch.Start)
			e, ok2 := parseHM(ch.End)
			if !ok1 || !ok2 || e <= s || !fits(free, s, e) {
				return nil, false
			}
			p.Start, p.End = s, e
			free = take(free, s, e)
		}
		out = append(out, p)
	}
	return sortPlanned(out), true
}

// scoreIssue ranks an open issue: overdue first, then due today, with a
// bonus for priority (1 urgent, 2 high) and for work already started.
func scoreIssue(overdue bool, priority int, inProgress bool) int {
	score := 70
	if overdue {
		score = 90
	}
	switch priority {
	case 1:
		score += 20
	case 2:
		score += 10
	}
	if inProgress {
		score += 15
	}
	return score
}
