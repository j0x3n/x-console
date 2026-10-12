package music_test

import (
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music"
)

func TestTitlesMatch(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"夜曲", "夜曲", true},
		{"Yequ", "YEQU", true},
		{"夜曲 (Live)", "夜曲", false}, // live is a different recording
		{"夜曲 (Live)", "夜曲 [live]", true},
		{"夜曲（伴奏）", "夜曲", false},
		{"夜曲 (Remix)", "夜曲 remix", true},
		{"夜曲 (电影《无间道》插曲)", "夜曲", true},        // other notes in brackets are ignored
		{"Ｈｅｌｌｏ　World!", "hello world", true}, // full-width and punctuation
		{"", "", false},
		{"夜曲", "夜曲2", false},
		{"(Intro)", "Intro", true}, // a title that is only a bracket keeps its text
	} {
		if got := music.TitlesMatch(c.a, c.b); got != c.want {
			t.Errorf("music.TitlesMatch(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestArtistsOverlap(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"周杰伦", "周杰伦", true},
		{"周杰伦", "方文山/周杰伦", true},
		{"A & B", "b", true},
		{"A feat. B", "B", true},
		{"A、B", "C、D", false},
		{"", "周杰伦", false},
		{"周杰伦", "", false},
		{"Jay Chou", "jay chou", true},
	} {
		if got := music.ArtistsOverlap(c.a, c.b); got != c.want {
			t.Errorf("music.ArtistsOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestDurationsMatch(t *testing.T) {
	for _, c := range []struct {
		a, b int
		want bool
	}{{226000, 226000, true}, {226000, 227999, true}, {226000, 228001, false}, {226000, 224000, true}, {0, 226000, false}, {226000, 0, false}} {
		if got := music.DurationsMatch(c.a, c.b); got != c.want {
			t.Errorf("music.DurationsMatch(%d, %d) = %v", c.a, c.b, got)
		}
	}
}

func TestAcceptsNeedsAllThree(t *testing.T) {
	q := music.MatchQuery{Title: "夜曲", Artist: "周杰伦", DurationMs: 226000}
	ok := music.MatchCandidate{Title: "夜曲", Artist: "周杰伦", DurationMs: 226500}
	if !music.AcceptsCandidate(q, ok) {
		t.Fatal("exact match rejected")
	}
	for name, c := range map[string]music.MatchCandidate{
		"title":    {Title: "七里香", Artist: "周杰伦", DurationMs: 226000},
		"artist":   {Title: "夜曲", Artist: "林俊杰", DurationMs: 226000},
		"duration": {Title: "夜曲", Artist: "周杰伦", DurationMs: 300000},
		"unknown":  {Title: "夜曲", Artist: "周杰伦"},
	} {
		if music.AcceptsCandidate(q, c) {
			t.Errorf("%s mismatch accepted", name)
		}
	}
	if music.AcceptsCandidate(music.MatchQuery{Title: "夜曲", DurationMs: 226000}, ok) {
		t.Error("a song with no artist must not be matched automatically")
	}
}

func TestRankKeepsTheClosestFew(t *testing.T) {
	q := music.MatchQuery{Title: "夜曲", Artist: "周杰伦", DurationMs: 226000}
	list := music.RankCandidates(q, []music.MatchCandidate{
		{Source: "a", Title: "夜曲 (Live)", Artist: "周杰伦", DurationMs: 226000},
		{Source: "b", Title: "完全不同", Artist: "别人", DurationMs: 1},
		{Source: "c", Title: "夜曲", Artist: "周杰伦", DurationMs: 400000},
		{Source: "d", Title: "夜曲", Artist: "周杰伦", DurationMs: 226000, Lyrics: true},
	}, 2)
	if len(list) != 2 || list[0].Source != "d" || list[1].Source != "c" {
		t.Fatalf("rank: %+v", list)
	}
}

func TestLyricsRankPrefersTitleAndDurationOverArtist(t *testing.T) {
	q := music.MatchQuery{Title: "夜曲", Artist: "周杰伦", DurationMs: 226000}
	translated := music.MatchCandidate{Source: "translated", Title: "夜曲", Artist: "Jay Chou", DurationMs: 226500, Lyrics: true}
	list := music.RankCandidates(q, []music.MatchCandidate{
		{Source: "wrong-duration", Title: "夜曲", Artist: "周杰伦", DurationMs: 400000, Lyrics: true},
		{Source: "wrong-title", Title: "七里香", Artist: "周杰伦", DurationMs: 226000, Lyrics: true},
		translated,
		{Source: "exact", Title: "夜曲", Artist: "周杰伦", DurationMs: 226500, Lyrics: true},
	}, 4)
	if len(list) != 4 || list[0].Source != "exact" || list[1].Source != "translated" {
		t.Fatalf("歌词排序不符合标题和时长优先的规则：%+v", list)
	}
	if music.AcceptsCandidate(q, translated) {
		t.Fatal("歌手不重叠的候选不能自动采用")
	}
	list = music.RankCandidates(q, []music.MatchCandidate{{Title: "夜曲", Artist: "Jay Chou", Lyrics: true}}, 8)
	if len(list) != 1 {
		t.Fatal("歌手不重叠且时长未知的同名歌词候选也应保留")
	}
}
