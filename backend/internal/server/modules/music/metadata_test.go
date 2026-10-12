package music_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music"
	"go.senan.xyz/taglib"
)

func metadataHit(e *matchEnv, duration int, artist, date string) {
	e.src.on("/search", jsonReply(map[string]any{"results": []map[string]any{{
		"trackId": 5, "trackName": "夜曲", "artistName": artist, "collectionName": "候选专辑",
		"collectionArtistName": "专辑歌手", "releaseDate": date, "trackTimeMillis": duration,
		"artworkUrl100": e.src.srv.URL + "/art/100x100bb.jpg",
	}}}))
}

func TestMatchingFillsMetadataAndWritesTags(t *testing.T) {
	for _, mode := range []string{"auto", "chosen", "existing", "manual", "no-writeback", "metadata-only"} {
		t.Run(mode, func(t *testing.T) {
			e := setupMatch(t)
			e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": mode != "no-writeback"}, nil)
			tags := map[string][]string{taglib.Title: {"夜曲"}, taglib.TrackNumber: {"3"}, taglib.DiscNumber: {"2"}}
			if mode == "existing" {
				tags[taglib.Artist], tags[taglib.Album], tags[taglib.AlbumArtist], tags[taglib.Date] = []string{"周杰伦"}, []string{"原专辑"}, []string{"原专辑歌手"}, []string{"2001"}
			}
			if mode == "metadata-only" {
				tags[taglib.Lyrics] = []string{syncedLRC}
			}
			var embedded []byte
			if mode == "metadata-only" {
				embedded = e.src.cover
			}
			_, tr := e.add(t, "夜曲.mp3", tags, embedded)
			if mode == "manual" {
				e.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", tr.Id), map[string]any{"artist": ""}, nil)
			}
			metadataHit(e, tr.DurationMs, "周杰伦", "2005-11-01T00:00:00Z")
			path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
			if mode == "chosen" {
				e.MustDo(http.MethodPost, path, map[string]any{"choose": true, "parts": []string{"cover"}}, nil)
				// 选择时来源搜索已不可用，仍应使用持久保存的候选信息。
				e.src.on("/search", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
				e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]string{"source": "itunes", "sourceId": "5"}, "parts": []string{"cover"}}, nil)
				if exists, err := e.App.Deps.Settings.Has(context.Background(), fmt.Sprintf("music.candidate_metadata.%d", tr.Id)); err != nil || exists {
					t.Fatalf("candidate metadata not cleared: exists=%v, err=%v", exists, err)
				}
			} else if mode == "metadata-only" {
				e.MustDo(http.MethodPost, path, nil, nil)
			} else {
				e.m.RunMatches(context.Background(), false)
			}
			got := e.track(t, tr.Id)
			wantArtist, wantAlbum, wantAlbumArtist, wantYear := "周杰伦", "候选专辑", "专辑歌手", 2005
			if mode == "existing" {
				wantAlbum, wantAlbumArtist, wantYear = "原专辑", "原专辑歌手", 2001
			}
			if mode == "manual" {
				wantArtist, wantAlbum, wantAlbumArtist, wantYear = "", "", "", 0
			}
			if got.Artist != wantArtist || got.Album != wantAlbum || got.AlbumArtist != wantAlbumArtist || got.Year != wantYear {
				t.Fatalf("metadata: %+v", got)
			}
			if got.Title != tr.Title || got.TrackNo != 3 || got.DiscNo != 2 {
				t.Fatalf("unrelated tags changed: %+v", got)
			}
			path = filepath.Join(t.TempDir(), "read.mp3")
			if err := os.WriteFile(path, e.streamBytes(t, tr.Id), 0600); err != nil {
				t.Fatal(err)
			}
			written, err := taglib.ReadTags(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "no-writeback" {
				wantArtist, wantAlbum, wantAlbumArtist, wantYear = "", "", "", 0
			}
			for key, want := range map[string]string{taglib.Artist: wantArtist, taglib.Album: wantAlbum, taglib.AlbumArtist: wantAlbumArtist} {
				if got := strings.Join(written[key], ""); got != want {
					t.Fatalf("tag %s = %q, want %q", key, got, want)
				}
			}
			date := strings.Join(written[taglib.Date], "")
			if wantYear > 0 && !strings.HasPrefix(date, fmt.Sprint(wantYear)) || wantYear == 0 && date != "" {
				t.Fatalf("date = %q, want %d", date, wantYear)
			}
		})
	}
}

func TestPendingMetadataSurvivesChoosingOtherPart(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
	_, tr := e.add(t, "夜曲.mp3", idTags, nil)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	metadataHit(e, tr.DurationMs, "周杰伦", "2005-11-01")
	path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
	e.MustDo(http.MethodPost, path, map[string]any{"choose": true}, nil)
	e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]string{"source": "lrclib", "sourceId": "11"}, "parts": []string{"lyrics"}}, nil)
	e.src.on("/search", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]string{"source": "itunes", "sourceId": "5"}, "parts": []string{"cover"}}, nil)
	got := e.track(t, tr.Id)
	if got.Album != "专辑" || got.AlbumArtist != "专辑歌手" || got.Year != 2005 || !got.HasCover || got.MatchState != "matched" {
		t.Fatalf("remaining candidate metadata: %+v", got)
	}
}

func TestRemovedTrackClearsPersistedCandidateMetadata(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
	item, tr := e.add(t, "夜曲.mp3", idTags, nil)
	metadataHit(e, tr.DurationMs, "周杰伦", "2005-11-01")
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), map[string]any{"choose": true}, nil)
	key := fmt.Sprintf("music.candidate_metadata.%d", tr.Id)
	if exists, err := e.App.Deps.Settings.Has(context.Background(), key); err != nil || !exists {
		t.Fatalf("candidate not persisted: %v, %v", exists, err)
	}
	// 模拟云盘彻底删除文件，外键先删歌曲和候选，再让音乐模块扫描。
	if _, err := e.App.Deps.DB.Exec("DELETE FROM drive_items WHERE id = ?", item); err != nil {
		t.Fatal(err)
	}
	reconcile(t, e.m)
	if exists, err := e.App.Deps.Settings.Has(context.Background(), key); err != nil || exists {
		t.Fatalf("orphan candidate metadata: %v, %v", exists, err)
	}
}

func TestUnknownArtistIsFixedBeforeFetchingOtherParts(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
	_, tr := e.add(t, "夜曲.mp3", map[string][]string{taglib.Title: {"夜曲"}}, nil)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	metadataHit(e, tr.DurationMs, "林俊杰", "2005-11-01")
	e.m.RunMatches(context.Background(), false)
	got := e.track(t, tr.Id)
	if got.Artist != "周杰伦" || got.Album != "专辑" || got.HasCover || got.Year != 0 || got.AlbumArtist != "" {
		t.Fatalf("mixed artists: %+v", got)
	}
	if e.src.called("/art/1000x1000bb.jpg") != 0 {
		t.Fatal("downloaded another artist's cover")
	}
}

func TestSourcesReturnMetadataAndSearchWithoutArtist(t *testing.T) {
	e := setupMatch(t)
	for _, tc := range []struct {
		source, path, param, search, body string
		year                              int
		albumArtist                       string
	}{
		{"lrclib", "/api/search", "track_name", "夜曲", `[{"id":11,"trackName":"夜曲","artistName":"周杰伦","duration":226,"plainLyrics":"歌词"}]`, 0, ""},
		{"netease", "/api/search/get/web", "s", "夜曲", `{"code":200,"result":{"songs":[{"id":1,"name":"夜曲","publishTime":1130803200000,"album":{"name":"专辑"},"artists":[{"name":"周杰伦"}]}]}}`, 2005, ""},
		{"qqmusic", "/soso/fcgi-bin/client_search_cp", "w", "夜曲", `{"data":{"song":{"list":[{"songmid":"1","songname":"夜曲","publictime":"2005-11-01","singer":[{"name":"周杰伦"}]}]}}}`, 2005, ""},
		{"itunes", "/search", "term", "夜曲", `{"results":[{"trackId":1,"trackName":"夜曲","artistName":"周杰伦","collectionArtistName":"专辑歌手","releaseDate":"2005-11-01T00:00:00Z"}]}`, 2005, "专辑歌手"},
		{"musicbrainz", "/ws/2/recording", "query", `recording:"夜曲"`, `{"recordings":[{"id":"1","title":"夜曲","releases":[{"id":"r","title":"专辑","date":"2005"}]}]}`, 2005, ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			e.src.on(tc.path, func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get(tc.param); got != tc.search {
					t.Errorf("search = %q, want %q", got, tc.search)
				}
				if _, ok := r.URL.Query()["artist_name"]; ok {
					t.Error("empty artist parameter")
				}
				w.Write([]byte(tc.body))
			})
			cands, err := e.m.SourceSearchQuery(context.Background(), tc.source, music.MatchQuery{Title: "夜曲"})
			if err != nil || len(cands) != 1 {
				t.Fatalf("candidates: %+v, %v", cands, err)
			}
			if cands[0].Year != tc.year || cands[0].AlbumArtist != tc.albumArtist {
				t.Fatalf("metadata: %+v", cands[0])
			}
		})
	}
}

func TestInvalidReleaseYearsAreNotFilled(t *testing.T) {
	for _, date := range []string{"", "0000-01-01", "-2005-01-01", "invalid"} {
		t.Run(date, func(t *testing.T) {
			e := setupMatch(t)
			e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
			_, tr := e.add(t, "夜曲.mp3", idTags, nil)
			metadataHit(e, tr.DurationMs, "周杰伦", date)
			e.m.RunMatches(context.Background(), false)
			if got := e.track(t, tr.Id); got.Year != 0 {
				t.Fatalf("invalid year: %+v", got)
			}
		})
	}
}

func TestUnknownArtistUncertainMatchesGoPending(t *testing.T) {
	for _, mode := range []string{"unknown-duration", "partial-title"} {
		t.Run(mode, func(t *testing.T) {
			e := setupMatch(t)
			e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
			_, tr := e.add(t, "夜曲.mp3", map[string][]string{taglib.Title: {"夜曲"}}, nil)
			title := "夜曲"
			if mode == "unknown-duration" {
				if _, err := e.App.Deps.DB.Exec("UPDATE music_tracks SET duration_ms = 0 WHERE id = ?", tr.Id); err != nil {
					t.Fatal(err)
				}
			} else {
				title = "夜曲2"
			}
			e.src.lrclibHit(float64(tr.DurationMs)/1000, title, "周杰伦", syncedLRC)
			e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), nil, nil)
			got := e.track(t, tr.Id)
			if got.MatchState != "pending" || got.Artist != "" || got.Album != "" || got.LyricsSource != "none" {
				t.Fatalf("uncertain match: %+v", got)
			}
		})
	}
}

func TestFillAlbumsFillsAllMissingFields(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": true}, nil)
	type expected struct {
		id                         int64
		artist, album, albumArtist string
		year                       int
		state                      string
	}
	var checks []expected
	for _, mode := range []string{"all", "artist", "album", "year", "complete", "manual", "skipped", "not-found", "unknown-duration"} {
		tags := map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}, taglib.Album: {"原专辑"}, taglib.Date: {"2001"}}
		want := expected{artist: "周杰伦", album: "原专辑", year: 2001, albumArtist: "专辑歌手", state: "none"}
		if mode == "all" || mode == "artist" {
			delete(tags, taglib.Artist)
		}
		if mode == "all" || mode == "album" {
			delete(tags, taglib.Album)
			want.album = "候选专辑"
		}
		if mode == "all" || mode == "year" {
			delete(tags, taglib.Date)
			want.year = 2005
		}
		if mode == "complete" {
			want.albumArtist = ""
		} // 只有专辑歌手为空不纳入批量。
		if mode == "manual" || mode == "skipped" || mode == "not-found" || mode == "unknown-duration" {
			delete(tags, taglib.Artist)
			delete(tags, taglib.Album)
			delete(tags, taglib.Date)
			want.artist, want.album, want.albumArtist, want.year = "", "", "", 0
		}
		if mode == "not-found" {
			tags[taglib.Title] = []string{"另一首"}
		}
		_, tr := e.add(t, mode+".mp3", tags, nil)
		want.id = tr.Id
		if mode == "manual" {
			e.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", tr.Id), map[string]any{"artist": ""}, nil)
		}
		if mode == "skipped" {
			e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/skip", tr.Id), nil, nil)
			want.state = "skipped"
		}
		if mode == "unknown-duration" {
			if _, err := e.App.Deps.DB.Exec("UPDATE music_tracks SET duration_ms = 0 WHERE id = ?", tr.Id); err != nil {
				t.Fatal(err)
			}
		}
		metadataHit(e, tr.DurationMs, "周杰伦", "2005-11-01")
		checks = append(checks, want)
	}
	events, cancel := e.App.Deps.Bus.Subscribe("music.", 64)
	defer cancel()
	var out struct{ Total int }
	e.MustDo(http.MethodPost, "/music/fill-albums", nil, &out)
	if out.Total != 6 {
		t.Fatalf("total = %d, want 6", out.Total)
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Topic == "music.library_changed" {
				goto completed
			}
		case <-deadline:
			t.Fatal("missing completion event")
		}
	}
completed:
	for _, want := range checks {
		got := e.track(t, want.id)
		if got.Artist != want.artist || got.Album != want.album || got.AlbumArtist != want.albumArtist || got.Year != want.year || string(got.MatchState) != want.state || got.LyricsSource != "none" || got.HasCover {
			t.Fatalf("batch metadata: %+v, want %+v", got, want)
		}
	}
	if e.src.called("/art/1000x1000bb.jpg") != 0 || e.src.called("/api/song/lyric") != 0 || e.src.called("/lyric/fcgi-bin/fcg_query_lyric_new.fcg") != 0 {
		t.Fatal("batch downloaded lyrics or covers")
	}
}
