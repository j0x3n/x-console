package music_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.senan.xyz/taglib"
)

func TestMatchedAlbumOnlyFillsUneditedEmptyAlbum(t *testing.T) {
	for _, tc := range []struct {
		name, album               string
		manual, writeBack, choose bool
	}{
		{name: "empty", writeBack: true}, {name: "existing", album: "原专辑", writeBack: true},
		{name: "manual", manual: true}, {name: "no-writeback"}, {name: "chosen", choose: true, writeBack: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupMatch(t)
			e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": tc.writeBack}, nil)
			_, tr := e.add(t, "album.mp3", map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}, taglib.Album: {tc.album}, taglib.AlbumArtist: {"原专辑歌手"}}, nil)
			if tc.manual {
				e.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", tr.Id), map[string]any{"album": ""}, nil)
			}
			e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
			path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
			if tc.choose {
				e.MustDo(http.MethodPost, path, map[string]any{"choose": true, "parts": []string{"lyrics"}}, nil)
				e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]string{"source": "lrclib", "sourceId": "11"}}, nil)
			} else {
				e.m.RunMatches(context.Background(), false)
			}
			got := e.track(t, tr.Id)
			want := tc.album
			if want == "" && !tc.manual {
				want = "专辑"
			}
			if got.Album != want || got.Title != tr.Title || got.Artist != tr.Artist || got.AlbumArtist != tr.AlbumArtist {
				t.Fatalf("unexpected metadata: %+v", got)
			}
			tmp := filepath.Join(t.TempDir(), "read.mp3")
			if err := os.WriteFile(tmp, e.streamBytes(t, tr.Id), 0600); err != nil {
				t.Fatal(err)
			}
			tags, err := taglib.ReadTags(tmp)
			if err != nil {
				t.Fatal(err)
			}
			written := tc.album
			if tc.writeBack {
				written = want
			}
			if values := tags[taglib.Album]; strings.Join(values, "") != written {
				t.Fatalf("album tags: %v, want %q", values, written)
			}
		})
	}
}

func TestFillAlbumsOnlySearchesAndReportsProgress(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": true}, nil)
	_, a := e.add(t, "empty.mp3", idTags, nil)
	_, b := e.add(t, "existing.mp3", map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}, taglib.Album: {"原专辑"}, taglib.Date: {"2001"}}, nil)
	_, c := e.add(t, "manual.mp3", idTags, nil)
	e.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", c.Id), map[string]any{"album": ""}, nil)
	_, d := e.add(t, "skipped.mp3", idTags, nil)
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/skip", d.Id), nil, nil)
	_, missing := e.add(t, "missing.mp3", map[string][]string{taglib.Title: {"另一首"}, taglib.Artist: {"周杰伦"}}, nil)
	entered, release := make(chan struct{}, 1), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	e.src.on("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("track_name") == "夜曲" {
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		json.NewEncoder(w).Encode([]map[string]any{{"id": 11, "trackName": "夜曲", "artistName": "周杰伦", "albumName": "专辑", "duration": float64(a.DurationMs) / 1000, "syncedLyrics": syncedLRC}})
	})
	metadataHit(e, a.DurationMs, "周杰伦", "2005-11-01")
	events, cancel := e.App.Deps.Bus.Subscribe("music.", 64)
	defer cancel()
	var out struct{ Total int }
	e.MustDo(http.MethodPost, "/music/fill-albums", nil, &out)
	if out.Total != 2 {
		t.Fatalf("total: %d", out.Total)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("job not started")
	}
	status, _ := e.Do(http.MethodPost, "/music/fill-albums", nil, nil)
	if status != 409 {
		t.Fatalf("duplicate: %d", status)
	}
	before := e.src.called("/api/search")
	e.m.RunMatches(context.Background(), false)
	if e.src.called("/api/search") != before {
		t.Fatal("matching ran during album fill")
	}
	close(release)
	sawRunning, sawFinished := false, false
	deadline := time.After(5 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev.Topic == "music.album_progress" {
				raw, _ := json.Marshal(ev.Data)
				var p struct {
					Running     bool
					Done, Total int
					Current     string
				}
				json.Unmarshal(raw, &p)
				if p.Total != 2 {
					t.Fatalf("progress: %s", raw)
				}
				sawRunning = sawRunning || p.Running
				if !p.Running {
					if p.Done != 2 {
						t.Fatalf("final: %s", raw)
					}
					sawFinished = true
				}
			}
			if ev.Topic == "music.library_changed" && sawFinished {
				goto finished
			}
		case <-deadline:
			t.Fatal("missing completion events")
		}
	}
finished:
	if !sawRunning {
		t.Fatal("missing running event")
	}
	got := e.track(t, a.Id)
	if got.Album != "专辑" || got.LyricsSource != a.LyricsSource || got.HasCover || got.MatchState != a.MatchState {
		t.Fatalf("album-only: %+v", got)
	}
	for _, tr := range []struct {
		id    int64
		album string
	}{{b.Id, "原专辑"}, {c.Id, ""}, {d.Id, ""}, {missing.Id, ""}} {
		if e.track(t, tr.id).Album != tr.album {
			t.Fatal("excluded song changed")
		}
	}
	if e.src.called("/lyric/fcgi-bin/fcg_query_lyric_new.fcg") != 0 || e.src.called("/art/600x600bb.jpg") != 0 {
		t.Fatal("downloaded lyrics or cover")
	}
	e.MustDo(http.MethodPost, "/music/fill-albums", nil, &out)
	if out.Total != 1 {
		t.Fatalf("unfound song must be retried: %d", out.Total)
	}
	for {
		select {
		case ev := <-events:
			if ev.Topic == "music.library_changed" {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatal("retry not finished")
		}
	}
}

func TestFillAlbumsEmptyAndConflictsWithMatches(t *testing.T) {
	e := setupMatch(t)
	var out struct{ Total int }
	e.MustDo(http.MethodPost, "/music/fill-albums", nil, &out)
	if out.Total != 0 {
		t.Fatal(out)
	}
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false}, nil)
	e.add(t, "match.mp3", idTags, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	e.src.on("/api/search", func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.Write([]byte(`[]`))
	})
	done := make(chan struct{})
	go func() { defer close(done); e.m.RunMatches(context.Background(), false) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("match not started")
	}
	status, _ := e.Do(http.MethodPost, "/music/fill-albums", nil, nil)
	close(release)
	<-done
	if status != 409 {
		t.Fatalf("conflict: %d", status)
	}
}
