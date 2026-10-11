package music_test

import (
	"fmt"
	"net/http"
	"testing"

	"go.senan.xyz/taglib"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
)

func statsOf(t *testing.T, e *matchEnv, query string) api.MusicStats {
	t.Helper()
	var s api.MusicStats
	e.MustDo(http.MethodGet, "/music/stats"+query, nil, &s)
	return s
}

func (e *matchEnv) played(t *testing.T, id int64, seconds int) {
	t.Helper()
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/played", id), map[string]any{"seconds": seconds}, nil)
}

func TestListeningStats(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", map[string][]string{taglib.Title: {"A"}, taglib.Artist: {"X"}}, nil)
	item, b := e.add(t, "b.mp3", map[string][]string{taglib.Title: {"B"}, taglib.Artist: {"Y"}}, nil)
	_, c := e.add(t, "c.mp3", map[string][]string{taglib.Title: {"C"}, taglib.Artist: {"X"}}, nil)

	if s := statsOf(t, e, ""); s.Plays != 0 || s.TotalSeconds != 0 || s.Days != 30 || len(s.TopTracks) != 0 {
		t.Fatalf("empty: %+v", s)
	}
	e.played(t, a.Id, 100)
	e.played(t, a.Id, 50)
	e.played(t, b.Id, 200)
	e.played(t, c.Id, 10)

	s := statsOf(t, e, "?days=7")
	if s.Days != 7 || s.Plays != 4 || s.TotalSeconds != 360 || s.FocusPlays != 0 || s.FocusSeconds != 0 {
		t.Fatalf("totals: %+v", s)
	}
	// Most plays first, then the longest listening.
	if len(s.TopTracks) != 3 || s.TopTracks[0].Title != "A" || s.TopTracks[0].Plays != 2 || s.TopTracks[0].Seconds != 150 || s.TopTracks[1].Title != "B" {
		t.Fatalf("top tracks: %+v", s.TopTracks)
	}
	if s.TopTracks[0].TrackId == nil || *s.TopTracks[0].TrackId != a.Id {
		t.Fatalf("track id: %+v", s.TopTracks[0])
	}
	if len(s.TopArtists) != 2 || s.TopArtists[0].Artist != "X" || s.TopArtists[0].Plays != 3 || s.TopArtists[0].Seconds != 160 {
		t.Fatalf("top artists: %+v", s.TopArtists)
	}
	if len(s.FocusTopTracks) != 0 {
		t.Fatalf("focus tracks: %+v", s.FocusTopTracks)
	}

	// The numbers stay when a song leaves the library.
	e.MustDo(http.MethodDelete, fmt.Sprintf("/drive/items/%d", item), nil, nil)
	reconcile(t, e.m)
	s = statsOf(t, e, "")
	var gone *api.MusicPlayStat
	for i := range s.TopTracks {
		if s.TopTracks[i].Title == "B" {
			gone = &s.TopTracks[i]
		}
	}
	if s.Plays != 4 || gone == nil || gone.TrackId != nil || gone.Seconds != 200 {
		t.Fatalf("after removing a song: %+v / %+v", s, gone)
	}

	for _, q := range []string{"?days=0", "?days=366"} {
		if status, _ := e.Do(http.MethodGet, "/music/stats"+q, nil, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d", q, status)
		}
	}
}

func TestFocusPlaysAreCountedSeparately(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", map[string][]string{taglib.Title: {"A"}, taglib.Artist: {"X"}}, nil)
	_, b := e.add(t, "b.mp3", map[string][]string{taglib.Title: {"B"}, taglib.Artist: {"Y"}}, nil)

	e.played(t, a.Id, 40) // no pomodoro yet
	var session struct{ Id int64 }
	e.MustDo(http.MethodPost, "/focus/start", map[string]any{"minutes": 25}, &session)
	e.played(t, a.Id, 60)
	e.played(t, b.Id, 90)
	e.MustDo(http.MethodPost, fmt.Sprintf("/focus/%d/stop", session.Id), map[string]any{}, nil)
	e.played(t, b.Id, 30) // after it

	s := statsOf(t, e, "")
	if s.Plays != 4 || s.TotalSeconds != 220 || s.FocusPlays != 2 || s.FocusSeconds != 150 {
		t.Fatalf("totals: %+v", s)
	}
	if len(s.FocusTopTracks) != 2 || s.FocusTopTracks[0].Plays != 1 {
		t.Fatalf("focus tracks: %+v", s.FocusTopTracks)
	}
	for _, f := range s.FocusTopTracks {
		if f.Seconds != 60 && f.Seconds != 90 {
			t.Fatalf("focus listening should only count the focus session: %+v", f)
		}
	}
}

func TestFocusSettings(t *testing.T) {
	e := setupMatch(t)
	var s api.MusicSettings
	e.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if s.Focus.AutoPlay || s.Focus.AutoPause || s.Focus.OnlyFocusList || s.Focus.PlaylistId != 0 {
		t.Fatalf("defaults must all be off: %+v", s.Focus)
	}
	var pl api.MusicPlaylistDetail
	e.MustDo(http.MethodPost, "/music/playlists", map[string]any{"name": "专注"}, &pl)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"focus": map[string]any{"autoPlay": true, "playlistId": pl.Id}}, &s)
	if !s.Focus.AutoPlay || s.Focus.AutoPause || s.Focus.PlaylistId != pl.Id {
		t.Fatalf("partial update: %+v", s.Focus)
	}
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"focus": map[string]any{"autoPause": true, "onlyFocusList": true}}, &s)
	if !s.Focus.AutoPlay || !s.Focus.AutoPause || !s.Focus.OnlyFocusList || s.Focus.PlaylistId != pl.Id {
		t.Fatalf("second update: %+v", s.Focus)
	}
	if status, _ := e.Do(http.MethodPut, "/music/settings", map[string]any{"focus": map[string]any{"playlistId": 99999}}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown playlist: %d", status)
	}
	// Other settings are not touched, and a deleted playlist stops being the focus list.
	e.MustDo(http.MethodDelete, fmt.Sprintf("/music/playlists/%d", pl.Id), nil, nil)
	e.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if s.Focus.PlaylistId != 0 || !s.Focus.AutoPlay || !s.AutoMatch || len(s.Folders) != 1 {
		t.Fatalf("after deleting the playlist: %+v", s)
	}
}
