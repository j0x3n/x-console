package music

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

// lyricLine is one line. TimeMs is nil when the lyrics have no time axis.
type lyricLine struct {
	TimeMs *int
	Text   string
}

var (
	lrcStamp   = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	lrcWord    = regexp.MustCompile(`<\d{1,3}:\d{1,2}(?:[.:]\d{1,3})?>`)
	lrcMeta    = regexp.MustCompile(`^\[[A-Za-z]+:[^\]]*\]$`)
	lrcOffset  = regexp.MustCompile(`(?mi)^\[offset:\s*([+-]?\d+)\s*\]`)
	lrcLeading = regexp.MustCompile(`^(\s*\[\d{1,3}:\d{1,2}(?:[.:]\d{1,3})?\])+`)
)

// isSyncedLyrics reports whether the text has at least one time stamp.
func isSyncedLyrics(text string) bool { return lrcStamp.MatchString(text) }

// decodeLyricsFile turns the bytes of a .lrc file into text. It accepts UTF-8
// with or without a BOM, UTF-16 with a BOM and GB18030 (the old Chinese
// default for .lrc files).
func decodeLyricsFile(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		if out, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder().Bytes(b); err == nil {
			return string(out)
		}
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().Bytes(b); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(b), "")
}

// parseLyrics reads LRC text. Lines with several stamps repeat once per
// stamp, [offset:] shifts every stamp, word-level <mm:ss.xx> marks are
// dropped and tags such as [ar:] are ignored. Text without any stamp comes
// back as plain lines.
func parseLyrics(text string) (synced bool, lines []lyricLine) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if !isSyncedLyrics(text) {
		for _, raw := range strings.Split(text, "\n") {
			raw = strings.TrimSpace(raw)
			if lrcMeta.MatchString(raw) {
				continue
			}
			lines = append(lines, lyricLine{Text: raw})
		}
		return false, trimBlankLines(lines)
	}
	offset := 0
	if m := lrcOffset.FindStringSubmatch(text); m != nil {
		offset, _ = strconv.Atoi(m[1])
	}
	for _, raw := range strings.Split(text, "\n") {
		head := lrcLeading.FindString(raw)
		if head == "" {
			continue
		}
		body := strings.TrimSpace(lrcWord.ReplaceAllString(raw[len(head):], ""))
		for _, m := range lrcStamp.FindAllStringSubmatch(head, -1) {
			ms := stampMs(m[1], m[2], m[3]) - offset
			if ms < 0 {
				ms = 0
			}
			t := ms
			lines = append(lines, lyricLine{TimeMs: &t, Text: body})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return *lines[i].TimeMs < *lines[j].TimeMs })
	return true, lines
}

func stampMs(min, sec, frac string) int {
	m, _ := strconv.Atoi(min)
	s, _ := strconv.Atoi(sec)
	ms := 0
	if frac != "" {
		f, _ := strconv.Atoi(frac)
		switch len(frac) {
		case 1:
			ms = f * 100
		case 2:
			ms = f * 10
		default:
			ms = f
		}
	}
	return (m*60+s)*1000 + ms
}

func trimBlankLines(lines []lyricLine) []lyricLine {
	for len(lines) > 0 && lines[0].Text == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1].Text == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
