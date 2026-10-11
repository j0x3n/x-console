package music_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"go.senan.xyz/taglib"
)

// ---- sleep timer ----

func TestSleepTimer(t *testing.T) {
	env, m := setup(t)
	m.SetSleepUnit(150 * time.Millisecond)
	events, cancel := env.App.Deps.Bus.Subscribe("music.sleep_", 16)
	defer cancel()

	var s api.MusicSleep
	env.MustDo(http.MethodGet, "/music/sleep", nil, &s)
	if s.Active {
		t.Fatalf("starts active: %+v", s)
	}

	// Minutes: the server ends it and tells every page.
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"minutes": 2}, &s)
	if !s.Active || s.Mode == nil || *s.Mode != "time" || s.EndsAt == nil {
		t.Fatalf("set: %+v", s)
	}
	end := *s.EndsAt
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"extendMinutes": 1}, &s)
	if got := s.EndsAt.Sub(end); got != 150*time.Millisecond {
		t.Fatalf("extend by %v", got)
	}
	fired := false
	deadline := time.After(5 * time.Second)
	for !fired {
		select {
		case ev := <-events:
			if ev.Topic == "music.sleep_fired" {
				fired = true
			}
		case <-deadline:
			t.Fatal("the timer never fired")
		}
	}
	env.MustDo(http.MethodGet, "/music/sleep", nil, &s)
	if s.Active {
		t.Fatalf("still active after firing: %+v", s)
	}
}

func TestSleepTimerReplacedAndCancelledDoNotFire(t *testing.T) {
	env, m := setup(t)
	m.SetSleepUnit(100 * time.Millisecond)
	events, cancel := env.App.Deps.Bus.Subscribe("music.sleep_fired", 4)
	defer cancel()
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"minutes": 1}, nil)
	var s api.MusicSleep
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"minutes": 50}, &s) // replaces the short one
	time.Sleep(400 * time.Millisecond)
	select {
	case <-events:
		t.Fatal("a replaced timer fired")
	default:
	}
	if status, _ := env.Do(http.MethodDelete, "/music/sleep", nil, nil); status != http.StatusNoContent {
		t.Fatalf("cancel: %d", status)
	}
	if status, _ := env.Do(http.MethodDelete, "/music/sleep", nil, nil); status != http.StatusNoContent {
		t.Fatalf("cancel twice: %d", status)
	}
	env.MustDo(http.MethodGet, "/music/sleep", nil, &s)
	if s.Active {
		t.Fatalf("after cancel: %+v", s)
	}
}

func TestSleepByTracks(t *testing.T) {
	env, _ := setup(t)
	var s api.MusicSleep
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"tracks": 3}, &s)
	if !s.Active || *s.Mode != "tracks" || s.TracksLeft == nil || *s.TracksLeft != 3 || s.EndsAt != nil {
		t.Fatalf("set: %+v", s)
	}
	// The page that is playing counts down.
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"tracks": 2}, &s)
	if *s.TracksLeft != 2 {
		t.Fatalf("count down: %+v", s)
	}
	if status, _ := env.Do(http.MethodPut, "/music/sleep", map[string]any{"extendMinutes": 5}, nil); status != http.StatusBadRequest {
		t.Fatalf("extend a track timer: %d", status)
	}
}

func TestSleepValidation(t *testing.T) {
	env, _ := setup(t)
	for _, body := range []map[string]any{
		{}, {"minutes": 0}, {"minutes": 721}, {"tracks": 0}, {"tracks": 21},
		{"minutes": 5, "tracks": 2}, {"extendMinutes": 0}, {"extendMinutes": 5}, {"nope": 1},
	} {
		if status, _ := env.Do(http.MethodPut, "/music/sleep", body, nil); status != http.StatusBadRequest {
			t.Errorf("%v: %d", body, status)
		}
	}
}

func TestSleepTimerSurvivesARestart(t *testing.T) {
	env, m := setup(t)
	m.SetSleepUnit(100 * time.Millisecond)
	events, cancel := env.App.Deps.Bus.Subscribe("music.sleep_fired", 4)
	defer cancel()
	env.MustDo(http.MethodPut, "/music/sleep", map[string]any{"minutes": 2}, nil)
	m.ArmSleepAtStart(context.Background()) // as Start does after a restart
	select {
	case <-events:
	case <-time.After(5 * time.Second):
		t.Fatal("the re-armed timer never fired")
	}
}

// ---- playlist shares ----

type sharedEnv struct {
	*matchEnv
	playlist int64
	tracks   []api.MusicTrack
	other    api.MusicTrack
}

func setupShare(t *testing.T) *sharedEnv {
	e := setupMatch(t)
	var made []api.MusicTrack
	for _, n := range []string{"A", "B"} {
		_, tr := e.add(t, n+".mp3", map[string][]string{taglib.Title: {n}, taglib.Artist: {"歌手"}, taglib.Lyrics: {"[00:00.50]" + n + "的歌词"}}, fixture(t, "cover.jpg"))
		made = append(made, tr)
	}
	_, other := e.add(t, "C.mp3", map[string][]string{taglib.Title: {"C"}}, nil)
	var pl api.MusicPlaylistDetail
	e.MustDo(http.MethodPost, "/music/playlists", map[string]any{"name": "分享用", "trackIds": []int64{made[0].Id, made[1].Id}}, &pl)
	return &sharedEnv{matchEnv: e, playlist: pl.Id, tracks: made, other: other}
}

func (e *sharedEnv) create(t *testing.T, body map[string]any) api.MusicShare {
	var s api.MusicShare
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/playlists/%d/shares", e.playlist), body, &s)
	return s
}

// public sends a request with no login, as a visitor would.
func (e *sharedEnv) public(t *testing.T, method, path string, body any, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.URL(path), rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{}).Do(req) // no cookie jar
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return resp, buf.Bytes()
}

func tokenOf(s api.MusicShare) string { return strings.TrimPrefix(s.Path, "/m/") }

func TestShareWithoutPassword(t *testing.T) {
	e := setupShare(t)
	s := e.create(t, map[string]any{})
	if s.HasPassword || s.ExpiresAt != nil || !strings.HasPrefix(s.Path, "/m/") || len(tokenOf(s)) < 30 {
		t.Fatalf("share: %+v", s)
	}
	base := "/public/music/" + tokenOf(s)
	resp, body := e.public(t, http.MethodGet, base, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("public info: %d %s", resp.StatusCode, body)
	}
	var pl api.MusicPublicPlaylist
	json.Unmarshal(body, &pl)
	if pl.Name != "分享用" || len(pl.Tracks) != 2 || pl.Tracks[0].Title != "A" || !pl.Tracks[0].HasCover || !pl.Tracks[0].HasLyrics {
		t.Fatalf("playlist: %+v", pl)
	}
	// Songs of the playlist play, with Range.
	stream := fmt.Sprintf("%s/tracks/%d/stream", base, e.tracks[0].Id)
	resp, body = e.public(t, http.MethodGet, stream, nil, map[string]string{"Range": "bytes=0-9"})
	if resp.StatusCode != http.StatusPartialContent || len(body) != 10 {
		t.Fatalf("public stream: %d %d bytes", resp.StatusCode, len(body))
	}
	if resp, _ = e.public(t, http.MethodGet, fmt.Sprintf("%s/tracks/%d/cover", base, e.tracks[0].Id), nil, nil); resp.StatusCode != 200 {
		t.Fatalf("public cover: %d", resp.StatusCode)
	}
	resp, body = e.public(t, http.MethodGet, fmt.Sprintf("%s/tracks/%d/lyrics", base, e.tracks[0].Id), nil, nil)
	var ly api.MusicLyrics
	json.Unmarshal(body, &ly)
	if resp.StatusCode != 200 || !ly.Synced || ly.Lines[0].Text != "A的歌词" {
		t.Fatalf("public lyrics: %d %+v", resp.StatusCode, ly)
	}
	// A song outside the playlist is not reachable through the link, nor is anything else.
	for _, path := range []string{
		fmt.Sprintf("%s/tracks/%d/stream", base, e.other.Id),
		fmt.Sprintf("%s/tracks/%d/cover", base, e.other.Id),
		fmt.Sprintf("%s/tracks/%d/lyrics", base, e.other.Id),
		fmt.Sprintf("%s/tracks/99999/stream", base),
	} {
		if resp, _ := e.public(t, http.MethodGet, path, nil, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
	// The ordinary API still needs a login.
	if resp, _ := e.public(t, http.MethodGet, fmt.Sprintf("/music/tracks/%d/stream", e.tracks[0].Id), nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("owner stream without login: %d", resp.StatusCode)
	}
	// Views are counted.
	var list []api.MusicShare
	e.MustDo(http.MethodGet, fmt.Sprintf("/music/playlists/%d/shares", e.playlist), nil, &list)
	if len(list) != 1 || list[0].ViewCount != 1 {
		t.Fatalf("list: %+v", list)
	}
}

func TestShareWithPassword(t *testing.T) {
	e := setupShare(t)
	s := e.create(t, map[string]any{"password": "秘密123", "expiresInDays": 7})
	if !s.HasPassword || s.ExpiresAt == nil {
		t.Fatalf("share: %+v", s)
	}
	base := "/public/music/" + tokenOf(s)
	stream := fmt.Sprintf("%s/tracks/%d/stream", base, e.tracks[0].Id)
	for _, path := range []string{base, stream, fmt.Sprintf("%s/tracks/%d/cover", base, e.tracks[0].Id)} {
		resp, body := e.public(t, http.MethodGet, path, nil, nil)
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(body), "share_code_required") {
			t.Fatalf("%s without a password: %d %s", path, resp.StatusCode, body)
		}
	}
	if resp, _ := e.public(t, http.MethodGet, base+"?access=garbage.1.2", nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("forged access: %d", resp.StatusCode)
	}
	resp, body := e.public(t, http.MethodPost, base+"/unlock", map[string]any{"code": "不对"}, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong password: %d %s", resp.StatusCode, body)
	}
	resp, body = e.public(t, http.MethodPost, base+"/unlock", map[string]any{"code": "秘密123"}, nil)
	var unlocked struct{ Access string }
	json.Unmarshal(body, &unlocked)
	if resp.StatusCode != 200 || unlocked.Access == "" {
		t.Fatalf("unlock: %d %s", resp.StatusCode, body)
	}
	if resp, _ := e.public(t, http.MethodGet, base+"?access="+unlocked.Access, nil, nil); resp.StatusCode != 200 {
		t.Fatalf("with access: %d", resp.StatusCode)
	}
	if resp, _ := e.public(t, http.MethodGet, stream+"?access="+unlocked.Access, nil, map[string]string{"Range": "bytes=0-3"}); resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("stream with access: %d", resp.StatusCode)
	}
	// An access from another share is no good here.
	s2 := e.create(t, map[string]any{"password": "另一个"})
	_, body = e.public(t, http.MethodPost, "/public/music/"+tokenOf(s2)+"/unlock", map[string]any{"code": "另一个"}, nil)
	var other struct{ Access string }
	json.Unmarshal(body, &other)
	if resp, _ := e.public(t, http.MethodGet, base+"?access="+other.Access, nil, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("access of another share: %d", resp.StatusCode)
	}
}

func TestSharePasswordLockout(t *testing.T) {
	e := setupShare(t)
	s := e.create(t, map[string]any{"password": "right"})
	base := "/public/music/" + tokenOf(s)
	for i := 0; i < 5; i++ {
		if resp, _ := e.public(t, http.MethodPost, base+"/unlock", map[string]any{"code": "wrong"}, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("try %d: %d", i, resp.StatusCode)
		}
	}
	// Locked: even the right password is refused now.
	resp, _ := e.public(t, http.MethodPost, base+"/unlock", map[string]any{"code": "right"}, nil)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("after 5 failures: %d", resp.StatusCode)
	}
}

func TestShareEndsWhenRevokedExpiredOrPlaylistDeleted(t *testing.T) {
	e := setupShare(t)
	revoked := e.create(t, map[string]any{})
	expired := e.create(t, map[string]any{"expiresInDays": 1})
	gone := e.create(t, map[string]any{})
	ok := func(s api.MusicShare) int {
		resp, _ := e.public(t, http.MethodGet, "/public/music/"+tokenOf(s), nil, nil)
		return resp.StatusCode
	}
	for _, s := range []api.MusicShare{revoked, expired, gone} {
		if ok(s) != 200 {
			t.Fatalf("share %d not working at the start", s.Id)
		}
	}
	// Revoking works at once.
	if status, _ := e.Do(http.MethodDelete, fmt.Sprintf("/music/shares/%d", revoked.Id), nil, nil); status != http.StatusNoContent {
		t.Fatalf("revoke: %d", status)
	}
	if ok(revoked) != http.StatusNotFound {
		t.Fatal("a revoked link still works")
	}
	if status, _ := e.Do(http.MethodDelete, fmt.Sprintf("/music/shares/%d", revoked.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("revoke twice: %d", status)
	}
	// An end date in the past.
	if _, err := e.App.Deps.DB.Exec("UPDATE music_playlist_shares SET expires_at = ? WHERE id = ?", time.Now().UTC().Add(-time.Minute), expired.Id); err != nil {
		t.Fatal(err)
	}
	if ok(expired) != http.StatusNotFound {
		t.Fatal("an expired link still works")
	}
	// Deleting the playlist ends its links.
	if status, _ := e.Do(http.MethodDelete, fmt.Sprintf("/music/playlists/%d", e.playlist), nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete playlist: %d", status)
	}
	if ok(gone) != http.StatusNotFound {
		t.Fatal("a link of a deleted playlist still works")
	}
	if resp, _ := e.public(t, http.MethodGet, "/public/music/nonsense", nil, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown token: %d", resp.StatusCode)
	}
}

func TestShareValidation(t *testing.T) {
	e := setupShare(t)
	for _, body := range []map[string]any{{"password": ""}, {"password": strings.Repeat("长", 65)}, {"expiresInDays": 0}, {"expiresInDays": 366}, {"nope": 1}} {
		if status, _ := e.Do(http.MethodPost, fmt.Sprintf("/music/playlists/%d/shares", e.playlist), body, nil); status != http.StatusBadRequest {
			t.Errorf("%v: %d", body, status)
		}
	}
	if status, _ := e.Do(http.MethodPost, "/music/playlists/9999/shares", map[string]any{}, nil); status != http.StatusNotFound {
		t.Errorf("unknown playlist: %d", status)
	}
	if status, _ := e.Do(http.MethodGet, "/music/playlists/9999/shares", nil, nil); status != http.StatusNotFound {
		t.Errorf("list for an unknown playlist: %d", status)
	}
}
