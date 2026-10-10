package music

import (
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func ms(l lyricLine) int {
	if l.TimeMs == nil {
		return -1
	}
	return *l.TimeMs
}

func TestParseSyncedLyrics(t *testing.T) {
	text := "[ti:夜曲]\n[ar:周杰伦]\n[offset:500]\n[00:01.50]一\n[00:03.5][00:01.00]二 <00:01.10>字\n[01:00:25]三\n[00:09]\n\n没有时间的行\n"
	synced, lines := parseLyrics(text)
	if !synced {
		t.Fatal("not synced")
	}
	// stamps sort by time: 0.5 (1.00-0.5), 1.0, 3.0, 8.5, 59.75
	want := []struct {
		ms   int
		text string
	}{{500, "二 字"}, {1000, "一"}, {3000, "二 字"}, {8500, ""}, {59750, "三"}}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines: %+v", len(lines), lines)
	}
	for i, w := range want {
		if ms(lines[i]) != w.ms || lines[i].Text != w.text {
			t.Errorf("line %d: %d %q, want %d %q", i, ms(lines[i]), lines[i].Text, w.ms, w.text)
		}
	}
}

func TestParsePlainLyrics(t *testing.T) {
	synced, lines := parseLyrics("[ti:x]\n\n第一行\r\n第二行\n\n第三行\n\n")
	if synced {
		t.Fatal("plain text reported as synced")
	}
	var texts []string
	for _, l := range lines {
		if l.TimeMs != nil {
			t.Fatal("plain line has a time")
		}
		texts = append(texts, l.Text)
	}
	if len(texts) != 4 || texts[0] != "第一行" || texts[2] != "" || texts[3] != "第三行" {
		t.Fatalf("lines: %q", texts)
	}
}

func TestStampFractions(t *testing.T) {
	for _, c := range []struct {
		stamp string
		want  int
	}{{"[00:01.5]", 1500}, {"[00:01.50]", 1500}, {"[00:01.500]", 1500}, {"[00:01:50]", 1500}, {"[02:00]", 120000}} {
		_, lines := parseLyrics(c.stamp + "x")
		if len(lines) != 1 || ms(lines[0]) != c.want {
			t.Errorf("%s: %+v, want %d", c.stamp, lines, c.want)
		}
	}
}

func TestDecodeLyricsFile(t *testing.T) {
	const text = "[00:01.00]你好，世界"
	gbk, err := simplifiedchinese.GB18030.NewEncoder().String(text)
	if err != nil {
		t.Fatal(err)
	}
	for name, in := range map[string][]byte{
		"utf8":      []byte(text),
		"utf8 bom":  append([]byte{0xEF, 0xBB, 0xBF}, text...),
		"gb18030":   []byte(gbk),
		"utf16 bom": {0xFF, 0xFE, '[', 0, '0', 0, '0', 0, ':', 0, '0', 0, '1', 0, ']', 0, 'a', 0},
	} {
		got := decodeLyricsFile(in)
		want := text
		if name == "utf16 bom" {
			want = "[00:01]a"
		}
		if got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

func TestTagHelpers(t *testing.T) {
	for in, want := range map[string]int{"3": 3, "3/12": 3, " 03 ": 3, "": 0, "x": 0, "-1": 0, "99999": 0} {
		if got := leadingNumber(in); got != want {
			t.Errorf("leadingNumber(%q) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[string]int{"2004-09-01": 2004, "2004": 2004, "99": 0, "abcd": 0, "0001": 0} {
		if got := year(in); got != want {
			t.Errorf("year(%q) = %d, want %d", in, got, want)
		}
	}
	for _, c := range []struct{ name, title, artist string }{
		{"周杰伦 - 夜曲.mp3", "夜曲", "周杰伦"},
		{"夜曲.flac", "夜曲", ""},
		{"A - B - C.mp3", "B - C", "A"},
		{" - x.mp3", "- x", ""},
	} {
		title, artist := titleFromName(c.name)
		if title != c.title || artist != c.artist {
			t.Errorf("%q: %q %q, want %q %q", c.name, title, artist, c.title, c.artist)
		}
	}
}
