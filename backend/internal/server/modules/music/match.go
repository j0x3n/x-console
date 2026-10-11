package music

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Matching rules (B148). A candidate is used automatically only when all of
// these hold; anything less goes to the list the user confirms:
//  1. the titles are the same once case, width, punctuation and bracketed
//     notes are ignored, and the version marks (live, remix, instrumental)
//     are the same on both sides;
//  2. the artists share at least one name;
//  3. both durations are known and differ by at most 2 seconds.

const maxDurationDiffMs = 2000

var (
	bracketed = regexp.MustCompile(`[\(（\[【][^\)）\]】]*[\)）\]】]`)
	versionRe = map[string]*regexp.Regexp{
		"live":         regexp.MustCompile(`(?i)\blive\b|现场|演唱会`),
		"remix":        regexp.MustCompile(`(?i)\bremix\b|混音`),
		"instrumental": regexp.MustCompile(`(?i)\binstrumental\b|\binst\b|伴奏|纯音乐`),
	}
	artistSplit = regexp.MustCompile(`(?i)\s*(?:/|,|，|&|、|;|；|\bfeat\.?\b|\bft\.?\b|\bx\b|\bvs\.?\b)\s*`)
)

// fold lowers case, turns full-width characters into normal ones and removes
// everything that is not a letter or a digit.
func fold(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKC.String(strings.ToLower(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// versionMarks finds the version marks in a title, in or out of brackets.
func versionMarks(title string) string {
	var marks []string
	for name, re := range versionRe {
		if re.MatchString(title) {
			marks = append(marks, name)
		}
	}
	sort.Strings(marks)
	return strings.Join(marks, ",")
}

// normTitle is the title without bracketed notes, folded for comparison.
func normTitle(title string) string {
	base := bracketed.ReplaceAllString(title, "")
	if strings.TrimSpace(base) == "" {
		base = title
	}
	// The version marks are compared on their own (versionMarks), so they
	// are not part of the title here: "夜曲 (Remix)" and "夜曲 remix" are one title.
	for _, re := range versionRe {
		base = re.ReplaceAllString(base, "")
	}
	return fold(base)
}

func titlesMatch(a, b string) bool {
	na, nb := normTitle(a), normTitle(b)
	return na != "" && na == nb && versionMarks(a) == versionMarks(b)
}

// artistNames splits "A/B", "A & B", "A feat. B" into folded names.
func artistNames(s string) []string {
	var out []string
	for _, p := range artistSplit.Split(s, -1) {
		if f := fold(p); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func artistsOverlap(a, b string) bool {
	na, nb := artistNames(a), artistNames(b)
	for _, x := range na {
		for _, y := range nb {
			if x == y {
				return true
			}
		}
	}
	return false
}

func durationsMatch(a, b int) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= maxDurationDiffMs
}

// accepts reports whether c may be used without asking.
func accepts(q query, c candidate) bool {
	return titlesMatch(q.Title, c.Title) && artistsOverlap(q.Artist, c.Artist) && durationsMatch(q.DurationMs, c.DurationMs)
}

// score ranks the candidates the user will choose from. Higher is closer.
func score(q query, c candidate) int {
	s := 0
	if titlesMatch(q.Title, c.Title) {
		s += 4
	} else if a, b := normTitle(q.Title), normTitle(c.Title); a != "" && b != "" && (strings.Contains(a, b) || strings.Contains(b, a)) {
		s += 1
	}
	if artistsOverlap(q.Artist, c.Artist) {
		s += 3
	}
	if durationsMatch(q.DurationMs, c.DurationMs) {
		s += 2
	}
	if c.Lyrics {
		s++
	}
	if c.Cover {
		s++
	}
	return s
}

// rank sorts candidates, best first, keeping the order of sources for ties,
// and returns at most n of them. Hits with nothing in common score 0 and are dropped.
func rank(q query, cands []candidate, n int) []candidate {
	type scored struct {
		c candidate
		s int
	}
	var list []scored
	for _, c := range cands {
		if s := score(q, c); s > 2 {
			list = append(list, scored{c, s})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].s > list[j].s })
	var out []candidate
	for _, x := range list {
		if len(out) == n {
			break
		}
		out = append(out, x.c)
	}
	return out
}
