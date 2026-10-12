package music_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.senan.xyz/taglib"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

// sources is a fake of every lyric and cover source on one test server.
type sources struct {
	t       *testing.T
	srv     *httptest.Server
	cover   []byte
	mu      sync.Mutex
	calls   []string
	agents  []string
	handler map[string]func(w http.ResponseWriter, r *http.Request)
}

func newSources(t *testing.T) *sources {
	s := &sources{t: t, cover: fixture(t, "cover.jpg"), handler: map[string]func(http.ResponseWriter, *http.Request){}}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls = append(s.calls, r.URL.Path)
		s.agents = append(s.agents, r.Header.Get("User-Agent"))
		h := s.handler[r.URL.Path]
		s.mu.Unlock()
		if h != nil {
			h(w, r)
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, ".jpg") || strings.Contains(r.URL.Path, "/front-"):
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(s.cover)
		case r.URL.Path == "/api/search":
			w.Write([]byte(`[]`))
		case r.URL.Path == "/api/search/get/web":
			w.Write([]byte(`{"code":200,"result":{"songs":[]}}`))
		case r.URL.Path == "/soso/fcgi-bin/client_search_cp":
			w.Write([]byte(`{"data":{"song":{"list":[]}}}`))
		case r.URL.Path == "/search":
			w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/ws/2/recording":
			w.Write([]byte(`{"recordings":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sources) on(path string, h func(http.ResponseWriter, *http.Request)) {
	s.mu.Lock()
	s.handler[path] = h
	s.mu.Unlock()
}

func (s *sources) called(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == path {
			n++
		}
	}
	return n
}

func (s *sources) urls() map[string]string {
	u := s.srv.URL
	return map[string]string{"lrclib": u, "netease": u, "qqmusic": u, "itunes": u, "musicbrainz": u, "coverart": u, "qqcover": u}
}

func jsonReply(v any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}
}

const syncedLRC = "[00:00.50]第一句\n[00:00.90]第二句\n"

// lrclibHit answers the LRCLIB search with one hit.
func (s *sources) lrclibHit(durationSec float64, title, artist, lyrics string) {
	s.on("/api/search", jsonReply([]map[string]any{{"id": 11, "trackName": title, "artistName": artist, "albumName": "专辑", "duration": durationSec, "syncedLyrics": lyrics}}))
}

// itunesHit answers the iTunes search with one hit whose artwork is on this server.
func (s *sources) itunesHit(durationMs int, title, artist string) {
	s.on("/search", jsonReply(map[string]any{"results": []map[string]any{{"trackId": 5, "trackName": title, "artistName": artist, "collectionName": "专辑",
		"trackTimeMillis": durationMs, "artworkUrl100": s.srv.URL + "/art/100x100bb.jpg"}}}))
}

type matchEnv struct {
	*testutil.Env
	m    *music.Module
	src  *sources
	root int64
}

func setupMatch(t *testing.T) *matchEnv {
	env, m := setup(t)
	src := newSources(t)
	m.SetSourceURLs(src.urls())
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	return &matchEnv{Env: env, m: m, src: src, root: root}
}

func (e *matchEnv) add(t *testing.T, name string, tags map[string][]string, cover []byte) (itemID int64, track api.MusicTrack) {
	t.Helper()
	itemID = upload(t, e.Env, e.root, name, song(t, "mp3", tags, cover))
	reconcile(t, e.m)
	for _, tr := range tracks(t, e.Env, "?limit=500").Items {
		if tr.DriveItemId == itemID {
			return itemID, tr
		}
	}
	t.Fatalf("track for %s not indexed", name)
	return
}

func (e *matchEnv) track(t *testing.T, id int64) api.MusicTrack {
	t.Helper()
	var tr api.MusicTrack
	e.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d", id), nil, &tr)
	return tr
}

func (e *matchEnv) streamBytes(t *testing.T, id int64) []byte {
	t.Helper()
	resp, body := get(t, e.Env, fmt.Sprintf("/music/tracks/%d/stream", id), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("stream: %d", resp.StatusCode)
	}
	return body
}

// readTags writes the bytes to a file and reads the tags and the cover back.
func readBack(t *testing.T, data []byte) (lyrics string, cover []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x.mp3")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	tags, err := taglib.ReadTags(path)
	if err != nil {
		t.Fatal(err)
	}
	img, _ := taglib.ReadImage(path)
	return strings.Join(tags[taglib.Lyrics], ""), img
}

var idTags = map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}}

func TestAutoMatchEmbedsLyricsAndCover(t *testing.T) {
	e := setupMatch(t)
	item, tr := e.add(t, "yequ.mp3", idTags, nil)
	if tr.LyricsSource != "none" || tr.HasCover || tr.MatchState != "none" {
		t.Fatalf("start: %+v", tr)
	}
	original := e.streamBytes(t, tr.Id)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")

	e.m.RunMatches(context.Background(), false)

	tr = e.track(t, tr.Id)
	if tr.MatchState != "matched" || tr.LyricsSource != "embedded" || !tr.LyricsSynced || !tr.HasCover || tr.CoverSource != "embedded" {
		t.Fatalf("after match: %+v", tr)
	}
	// The file itself now holds them.
	written := e.streamBytes(t, tr.Id)
	if bytes.Equal(written, original) {
		t.Fatal("the file was not changed")
	}
	lyrics, cover := readBack(t, written)
	if lyrics != strings.TrimSpace(syncedLRC) && lyrics != syncedLRC || len(cover) == 0 {
		t.Fatalf("file tags: lyrics %q, cover %d bytes", lyrics, len(cover))
	}
	// The drive record follows the new content.
	var di struct{ Size int64 }
	e.MustDo(http.MethodGet, fmt.Sprintf("/drive/items/%d", item), nil, &di)
	if di.Size != int64(len(written)) {
		t.Fatalf("drive size %d, file %d", di.Size, len(written))
	}
	// Lyrics and cover are served.
	var ly api.MusicLyrics
	e.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), nil, &ly)
	if !ly.Synced || len(ly.Lines) != 2 || ly.Lines[0].Text != "第一句" {
		t.Fatalf("lyrics: %+v", ly)
	}
	if resp, _ := get(t, e.Env, fmt.Sprintf("/music/tracks/%d/cover", tr.Id), nil); resp.StatusCode != 200 {
		t.Fatalf("cover: %d", resp.StatusCode)
	}
	// Sources are asked politely and only once per song.
	for _, ua := range e.src.agents {
		if !strings.HasPrefix(ua, "X-Console/") {
			t.Fatalf("user agent %q", ua)
		}
	}
	// A new scan does not read the changed file again, and the song is not matched twice.
	events, cancel := e.App.Deps.Bus.Subscribe("music.", 8)
	defer cancel()
	reconcile(t, e.m)
	e.m.RunMatches(context.Background(), false)
	if n := e.src.called("/api/search"); n != 1 {
		t.Fatalf("lrclib asked %d times", n)
	}
	for len(events) > 0 {
		if ev := <-events; ev.Topic == "music.library_changed" {
			if d, ok := ev.Data.(map[string]int); ok && d["updated"] > 0 {
				t.Fatalf("the written file was read again: %v", d)
			}
		}
	}
}

func TestMatchWithoutWriteBackKeepsTheFile(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"writeBack": false}, nil)
	_, tr := e.add(t, "yequ.mp3", idTags, nil)
	original := e.streamBytes(t, tr.Id)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")

	e.m.RunMatches(context.Background(), false)

	tr = e.track(t, tr.Id)
	if tr.MatchState != "matched" || tr.LyricsSource != "online" || tr.CoverSource != "online" || !tr.HasCover {
		t.Fatalf("after match: %+v", tr)
	}
	if !bytes.Equal(e.streamBytes(t, tr.Id), original) {
		t.Fatal("the file changed although write-back is off")
	}
	// The library still has them after a rescan.
	reconcile(t, e.m)
	if got := e.track(t, tr.Id); got.LyricsSource != "online" || !got.HasCover {
		t.Fatalf("after rescan: %+v", got)
	}
}

func TestUnsureMatchesWaitForTheUser(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", idTags, nil)
	_, b := e.add(t, "b.mp3", map[string][]string{taglib.Title: {"七里香"}, taglib.Artist: {"周杰伦"}}, nil)
	// The duration is off by 10 seconds: close, but not sure.
	e.src.lrclibHit(float64(a.DurationMs)/1000+10, "夜曲", "周杰伦", syncedLRC)

	e.m.RunMatches(context.Background(), false)

	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	// The same fake answers for both songs; neither is a sure match.
	if len(pending) != 2 {
		t.Fatalf("pending: %+v", pending)
	}
	var c api.MusicCandidate
	for _, p := range pending {
		if p.Track.Id == a.Id {
			if len(p.Candidates) != 1 {
				t.Fatalf("candidates: %+v", p.Candidates)
			}
			c = p.Candidates[0]
		}
	}
	if c.Source != "lrclib" || c.Title != "夜曲" || !c.Lyrics || c.Cover {
		t.Fatalf("candidate: %+v", c)
	}
	_ = b
	if e.track(t, a.Id).LyricsSource != "none" {
		t.Fatal("an unsure candidate was used")
	}

	// Choosing the candidate applies it.
	var got api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", a.Id), map[string]any{"candidate": map[string]any{"source": c.Source, "sourceId": c.SourceId}}, &got)
	if got.MatchState != "matched" || !got.LyricsSynced || got.LyricsSource != "embedded" {
		t.Fatalf("chosen: %+v", got)
	}
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	for _, p := range pending {
		if p.Track.Id == a.Id {
			t.Fatal("a chosen song is still pending")
		}
	}
	// A candidate that is not in the list is refused.
	if status, _ := e.Do(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", b.Id), map[string]any{"candidate": map[string]any{"source": "lrclib", "sourceId": "nope"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown candidate: %d", status)
	}
}

// 2026-10-11 只找到封面、歌词的候选不够准时，这首歌要进待确认，不能算已匹配。
func TestCoverOnlyMatchStillAsksForLyrics(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", idTags, nil)
	e.src.itunesHit(a.DurationMs, "夜曲", "周杰伦")
	// 歌词来源的时长差 10 秒：像是同一首，但不能直接用。
	e.src.lrclibHit(float64(a.DurationMs)/1000+10, "夜曲", "周杰伦", syncedLRC)

	e.m.RunMatches(context.Background(), false)

	got := e.track(t, a.Id)
	if got.CoverSource == "none" || got.LyricsSource != "none" || got.MatchState != "pending" {
		t.Fatalf("after match: %+v", got)
	}
	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 1 || len(pending[0].Candidates) != 1 || !pending[0].Candidates[0].Lyrics {
		t.Fatalf("pending: %+v", pending)
	}
	c := pending[0].Candidates[0]
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", a.Id), map[string]any{"candidate": map[string]any{"source": c.Source, "sourceId": c.SourceId}}, &got)
	if got.MatchState != "matched" || !got.LyricsSynced {
		t.Fatalf("chosen: %+v", got)
	}
}

// 自动匹配不对时，用户可以手动看别的来源再选一个。
func TestChooseCandidatesFromOtherSources(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", idTags, nil)

	var got api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", a.Id), map[string]any{"choose": true}, &got)
	if got.MatchState != "failed" {
		t.Fatalf("nothing found: %+v", got)
	}

	e.src.lrclibHit(float64(a.DurationMs)/1000+10, "夜曲", "周杰伦", syncedLRC)
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", a.Id), map[string]any{"choose": true}, &got)
	if got.MatchState != "pending" || got.LyricsSource != "none" {
		t.Fatalf("choose: %+v", got)
	}
	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 1 || len(pending[0].Candidates) != 1 {
		t.Fatalf("pending: %+v", pending)
	}
	c := pending[0].Candidates[0]
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", a.Id), map[string]any{"candidate": map[string]any{"source": c.Source, "sourceId": c.SourceId}}, &got)
	if got.LyricsSource != "embedded" || !got.LyricsSynced {
		t.Fatalf("chosen: %+v", got)
	}
}

func TestSkipStopsFurtherMatching(t *testing.T) {
	e := setupMatch(t)
	_, a := e.add(t, "a.mp3", idTags, nil)
	e.src.lrclibHit(float64(a.DurationMs)/1000+10, "夜曲", "周杰伦", syncedLRC)
	e.m.RunMatches(context.Background(), false)
	var got api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/skip", a.Id), nil, &got)
	if got.MatchState != "skipped" {
		t.Fatalf("skip: %+v", got)
	}
	calls := e.src.called("/api/search")
	e.m.RunMatches(context.Background(), true)
	if e.src.called("/api/search") != calls {
		t.Fatal("a skipped song was matched again")
	}
	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 0 {
		t.Fatalf("pending after skip: %+v", pending)
	}
}

func TestOneSourceDownDoesNotStopTheMatch(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "a.mp3", idTags, nil)
	e.src.on("/api/search", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", 500) })
	e.src.on("/search", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("not json")) })
	e.src.on("/api/search/get/web", jsonReply(map[string]any{"code": 200, "result": map[string]any{"songs": []map[string]any{
		{"id": 99, "name": "夜曲", "duration": tr.DurationMs, "artists": []map[string]any{{"name": "周杰伦"}}, "album": map[string]any{"name": "专辑"}}}}}))
	e.src.on("/api/song/lyric", jsonReply(map[string]any{"code": 200, "lrc": map[string]any{"lyric": syncedLRC}}))
	e.src.on("/api/song/detail/", jsonReply(map[string]any{"code": 200, "songs": []map[string]any{{"album": map[string]any{"picUrl": e.src.srv.URL + "/netease.jpg"}}}}))

	e.m.RunMatches(context.Background(), false)

	got := e.track(t, tr.Id)
	if got.MatchState != "matched" || !got.LyricsSynced || !got.HasCover {
		t.Fatalf("netease only: %+v", got)
	}
}

func TestQQLyricsEntitiesAreDecoded(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"providers": map[string]bool{"lrclib": false, "netease": false, "itunes": false, "musicbrainz": false}, "writeBack": false}, nil)
	_, tr := e.add(t, "a.mp3", idTags, nil)
	e.src.on("/soso/fcgi-bin/client_search_cp", jsonReply(map[string]any{"data": map[string]any{"song": map[string]any{"list": []map[string]any{
		{"songmid": "mid1", "songname": "夜曲", "albumname": "专辑", "albummid": "alb1", "interval": (tr.DurationMs + 400) / 1000, "singer": []map[string]any{{"name": "周杰伦"}}}}}}}))
	e.src.on("/lyric/fcgi-bin/fcg_query_lyric_new.fcg", jsonReply(map[string]any{"lyric": "[00&#58;00&#46;50]第一句&#10;[00&#58;00&#46;90]第二句"}))

	e.m.RunMatches(context.Background(), false)

	var ly api.MusicLyrics
	e.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), nil, &ly)
	if !ly.Synced || len(ly.Lines) != 2 || ly.Lines[1].Text != "第二句" {
		t.Fatalf("qq lyrics: %+v", ly)
	}
	if e.src.called("/api/search") != 0 {
		t.Fatal("a disabled source was asked")
	}
}

func TestSmallCoversAreNotUsed(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"writeBack": false}, nil)
	_, tr := e.add(t, "a.mp3", idTags, nil)
	e.src.cover = fixture(t, "small.png") // 200 x 200
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")
	e.m.RunMatches(context.Background(), false)
	got := e.track(t, tr.Id)
	if got.HasCover {
		t.Fatalf("a 200 pixel cover was used: %+v", got)
	}
}

func TestAnUnwritableFileKeepsItsOriginal(t *testing.T) {
	e := setupMatch(t)
	item := upload(t, e.Env, e.root, "broken.mp3", []byte("this is not audio at all, just text pretending to be a song"))
	reconcile(t, e.m)
	var tr api.MusicTrack
	for _, x := range tracks(t, e.Env, "").Items {
		if x.DriveItemId == item {
			tr = x
		}
	}
	if tr.Id == 0 {
		t.Fatal("not indexed")
	}
	before := e.streamBytes(t, tr.Id)
	// The file name gives "broken" as the title; answer for that.
	e.src.lrclibHit(0, "broken", "x", syncedLRC)
	// Without a known duration or artist nothing is accepted, so use a manual lyric to reach write-back.
	var got api.MusicTrack
	e.MustDo(http.MethodPut, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), map[string]any{"text": "手写歌词"}, &got)
	if got.LyricsSource != "online" {
		t.Fatalf("write-back to a non-audio file should fail and keep the lyrics in the library: %+v", got)
	}
	if !bytes.Equal(e.streamBytes(t, tr.Id), before) {
		t.Fatal("the original file was changed")
	}
}

func TestOverwriteReplacesWhatThereAlreadyIs(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "a.mp3", map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}, taglib.Lyrics: {"旧歌词"}}, nil)
	if tr.LyricsSource != "embedded" {
		t.Fatalf("start: %+v", tr)
	}
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	// Without overwrite a song that has lyrics and only lacks a cover looks for the cover only.
	var got api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), map[string]any{}, &got)
	if got.LyricsSource != "embedded" || got.LyricsSynced {
		t.Fatalf("lyrics replaced without overwrite: %+v", got)
	}
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), map[string]any{"overwrite": true}, &got)
	if !got.LyricsSynced {
		t.Fatalf("overwrite did not replace the lyrics: %+v", got)
	}
	lyrics, _ := readBack(t, e.streamBytes(t, tr.Id))
	if !strings.Contains(lyrics, "第一句") || strings.Contains(lyrics, "旧歌词") {
		t.Fatalf("file lyrics: %q", lyrics)
	}
}

func TestManualLyricsAndCover(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "a.mp3", idTags, nil)
	var got api.MusicTrack
	e.MustDo(http.MethodPut, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), map[string]any{"text": "[00:01.00]手写\r\n", "writeBack": false}, &got)
	if got.LyricsSource != "online" || !got.LyricsSynced {
		t.Fatalf("manual lyrics: %+v", got)
	}
	for _, body := range []map[string]any{{"text": "   "}, {"text": strings.Repeat("字", 200000)}} {
		if status, _ := e.Do(http.MethodPut, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), body, nil); status != http.StatusBadRequest {
			t.Errorf("bad lyrics: %d", status)
		}
	}

	post := func(body []byte, query string) int {
		req, _ := http.NewRequest(http.MethodPost, e.URL(fmt.Sprintf("/music/tracks/%d/cover%s", tr.Id, query)), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Requested-With", "x-console")
		resp, err := e.Client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if status := post(fixture(t, "cover.jpg"), "?writeBack=true"); status != 200 {
		t.Fatalf("upload cover: %d", status)
	}
	_, cover := readBack(t, e.streamBytes(t, tr.Id))
	if len(cover) == 0 {
		t.Fatal("the uploaded cover was not written into the file")
	}
	if got = e.track(t, tr.Id); !got.HasCover || got.CoverSource != "embedded" {
		t.Fatalf("after upload: %+v", got)
	}
	if status := post([]byte("not an image"), ""); status != http.StatusBadRequest {
		t.Fatalf("bad image: %d", status)
	}
	if status := post(nil, ""); status != http.StatusBadRequest {
		t.Fatalf("empty image: %d", status)
	}
}

func TestMatchSettings(t *testing.T) {
	e := setupMatch(t)
	var s api.MusicSettings
	e.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if !s.AutoMatch || !s.WriteBack || len(s.Providers) != 5 || !s.Providers["qqmusic"] || len(s.Folders) != 1 {
		t.Fatalf("defaults: %+v", s)
	}
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "providers": map[string]bool{"netease": false}}, &s)
	if s.AutoMatch || !s.WriteBack || s.Providers["netease"] || !s.Providers["lrclib"] || len(s.Folders) != 1 {
		t.Fatalf("partial update: %+v", s)
	}
	if status, _ := e.Do(http.MethodPut, "/music/settings", map[string]any{"providers": map[string]bool{"kugou": true}}, nil); status != http.StatusBadRequest {
		t.Fatalf("unknown source: %d", status)
	}
}

func TestAutoMatchRunsInTheBackgroundAndCanBeSwitchedOff(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false}, nil)
	_, tr := e.add(t, "a.mp3", idTags, nil)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	var st api.MusicScanStatus
	e.MustDo(http.MethodPost, "/music/scan", nil, &st)
	waitUntil(t, func() bool { return e.track(t, tr.Id).Id != 0 })
	if e.src.called("/api/search") != 0 {
		t.Fatal("matched although auto match is off")
	}
	// The batch endpoint matches on request even then.
	e.MustDo(http.MethodPost, "/music/match", nil, &st)
	waitUntil(t, func() bool { return e.track(t, tr.Id).MatchState == "matched" })
}

func TestSourceCheckReportsEverySourceAndSearchesConcurrently(t *testing.T) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"providers": map[string]bool{"netease": false}}, nil)
	arrived := make(chan struct{}, len(music.AllSources))
	release := make(chan struct{})
	go func() {
		defer close(release)
		for range music.AllSources {
			select {
			case <-arrived:
			case <-time.After(2 * time.Second):
				t.Error("来源搜索没有并发执行")
				return
			}
		}
	}()
	for _, path := range []string{"/api/search", "/api/search/get/web", "/soso/fcgi-bin/client_search_cp", "/search", "/ws/2/recording"} {
		e.src.on(path, func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.RawQuery, "%E6%99%B4%E5%A4%A9") || !strings.Contains(r.URL.RawQuery, "%E5%91%A8%E6%9D%B0%E4%BC%A6") {
				t.Errorf("没有用默认歌名和歌手搜索：%s", r.URL.RawQuery)
			}
			arrived <- struct{}{}
			<-release
			switch r.URL.Path {
			case "/api/search":
				w.Write([]byte(`[]`))
			case "/api/search/get/web":
				if got := r.Header.Get("X-Real-IP"); got != "211.161.244.70" {
					t.Errorf("网易云请求头错误：%q", got)
				}
				w.Write([]byte(`{"abroad":true,"code":200,"result":"加密内容"}`))
			case "/soso/fcgi-bin/client_search_cp":
				w.WriteHeader(http.StatusForbidden)
			case "/search":
				w.Write([]byte(`{"results":[]}`))
			case "/ws/2/recording":
				w.Write([]byte(`{"recordings":[]}`))
			}
		})
	}
	var rows []api.MusicSourceCheck
	e.MustDo(http.MethodPost, "/music/sources/check", nil, &rows)
	if len(rows) != len(music.AllSources) {
		t.Fatalf("来源检查行数不对：%+v", rows)
	}
	for i, row := range rows {
		if string(row.Source) != music.AllSources[i] || row.Ms < 0 || row.Hits != 0 || row.LyricsHits != 0 || row.CoverHits != 0 {
			t.Fatalf("来源检查结果错误：%+v", row)
		}
		switch string(row.Source) {
		case "netease":
			if row.Enabled || row.Ok || row.Error != "网易云限制了海外访问" {
				t.Fatalf("没有明确报告海外限制：%+v", row)
			}
		case "qqmusic":
			if !row.Enabled || row.Ok || row.Error != "HTTP 403" {
				t.Fatalf("没有报告 HTTP 错误：%+v", row)
			}
		default:
			if !row.Enabled || !row.Ok || row.Error != "" {
				t.Fatalf("空搜索结果应算成功：%+v", row)
			}
		}
	}
	for _, path := range []string{"/api/search", "/api/search/get/web", "/soso/fcgi-bin/client_search_cp", "/search", "/ws/2/recording"} {
		if n := e.src.called(path); n != 1 {
			t.Errorf("来源应该只搜索一次：%s，实际 %d 次", path, n)
		}
	}
}

func TestSourceCheckUsesTrackAndDoesNotChangeIt(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "check.mp3", idTags, nil)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")
	var chosen api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), map[string]any{"choose": true}, &chosen)
	var before []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &before)
	original := e.streamBytes(t, tr.Id)
	e.src.on("/api/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("track_name") != tr.Title || r.URL.Query().Get("artist_name") != tr.Artist {
			t.Error("来源检查没有使用歌曲信息")
		}
		jsonReply([]map[string]any{{"id": 11, "trackName": tr.Title, "artistName": tr.Artist, "syncedLyrics": syncedLRC}})(w, r)
	})
	var rows []api.MusicSourceCheck
	e.MustDo(http.MethodPost, "/music/sources/check", map[string]any{"trackId": tr.Id}, &rows)
	if len(rows) != 5 || rows[0].Hits != 1 || rows[0].LyricsHits != 1 || rows[0].CoverHits != 0 || rows[3].Hits != 1 || rows[3].CoverHits != 1 {
		t.Fatalf("命中统计错误：%+v", rows)
	}
	var after []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &after)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) || e.track(t, tr.Id) != chosen || !bytes.Equal(original, e.streamBytes(t, tr.Id)) {
		t.Fatal("来源检查修改了歌曲、文件或候选")
	}
	if status, _ := e.Do(http.MethodPost, "/music/sources/check", map[string]any{"trackId": 999999}, nil); status != http.StatusNotFound {
		t.Fatalf("不存在的歌曲应返回 404：%d", status)
	}
}

func TestNeteaseChecksStatusBeforeParsingSearchAndLyrics(t *testing.T) {
	e := setupMatch(t)
	for _, tc := range []struct{ body, reason string }{
		{`{"abroad":true,"code":200,"result":"加密内容","lrc":"加密内容"}`, "网易云限制了海外访问"},
		{`{"code":403,"result":"错误内容","lrc":"错误内容"}`, "网易云返回 code=403"},
		{`<html>拒绝访问</html>`, "返回的不是预期格式（可能限制了海外访问）"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			h := func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Real-IP") != "211.161.244.70" {
					t.Error("网易云搜索和歌词请求都要带国内 IP")
				}
				w.Write([]byte(tc.body))
			}
			e.src.on("/api/search/get/web", h)
			e.src.on("/api/song/lyric", h)
			var rows []api.MusicSourceCheck
			e.MustDo(http.MethodPost, "/music/sources/check", map[string]any{}, &rows)
			if rows[1].Ok || rows[1].Error != tc.reason {
				t.Fatalf("搜索错误原因不明确：%+v", rows[1])
			}
			if _, err := e.m.SourceLyrics(context.Background(), "netease", "1"); err == nil || err.Error() != tc.reason {
				t.Fatalf("歌词错误原因不明确：%v", err)
			}
		})
	}
}

func TestSourceCheckHonorsDeadline(t *testing.T) {
	e := setupMatch(t)
	for _, path := range []string{"/api/search", "/api/search/get/web", "/soso/fcgi-bin/client_search_cp", "/search", "/ws/2/recording"} {
		e.src.on(path, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	rows := e.m.CheckSources(ctx, music.MatchQuery{Title: "晴天", Artist: "周杰伦"})
	if len(rows) != 5 || time.Since(start) > time.Second {
		t.Fatalf("来源检查没有及时结束：%+v", rows)
	}
	for _, row := range rows {
		if row.Ok || row.Error != "超时" {
			t.Fatalf("超时应返回中文原因：%+v", row)
		}
	}
}

func TestSourceCheckRejectsUnexpectedResponseStructure(t *testing.T) {
	e := setupMatch(t)
	for _, path := range []string{"/api/search", "/soso/fcgi-bin/client_search_cp", "/search", "/ws/2/recording"} {
		e.src.on(path, jsonReply(map[string]any{}))
	}
	e.src.on("/api/search/get/web", jsonReply(map[string]any{"code": 200}))
	var rows []api.MusicSourceCheck
	e.MustDo(http.MethodPost, "/music/sources/check", nil, &rows)
	for _, row := range rows {
		if row.Ok || row.Error != "返回的不是预期格式（可能限制了海外访问）" {
			t.Fatalf("缺少搜索结果字段时不应算成功：%+v", row)
		}
	}
}

func TestMatchPartsOnlyChangeTheRequestedPart(t *testing.T) {
	for _, mode := range []string{"auto", "candidate"} {
		for _, part := range []string{"lyrics", "cover"} {
			t.Run(mode+"/"+part, func(t *testing.T) {
				e := setupMatch(t)
				_, tr := e.add(t, "parts.mp3", map[string][]string{taglib.Title: {"夜曲"}, taglib.Artist: {"周杰伦"}, taglib.Lyrics: {"旧歌词"}}, fixture(t, "small.png"))
				_, oldCover := get(t, e.Env, fmt.Sprintf("/music/tracks/%d/cover", tr.Id), nil)
				e.src.on("/api/search/get/web", jsonReply(map[string]any{"code": 200, "result": map[string]any{"songs": []map[string]any{
					{"id": 99, "name": "夜曲", "duration": tr.DurationMs, "artists": []map[string]any{{"name": "周杰伦"}}}}}}))
				e.src.on("/api/song/lyric", jsonReply(map[string]any{"code": 200, "lrc": map[string]any{"lyric": syncedLRC}}))
				e.src.on("/api/song/detail/", jsonReply(map[string]any{"code": 200, "songs": []map[string]any{{"album": map[string]any{"picUrl": e.src.srv.URL + "/netease.jpg"}}}}))
				path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
				body := map[string]any{"parts": []string{part}, "overwrite": true}
				if mode == "candidate" {
					e.MustDo(http.MethodPost, path, map[string]any{"choose": true, "parts": []string{part}}, nil)
					body["candidate"] = map[string]any{"source": "netease", "sourceId": "99"}
				}
				var got api.MusicTrack
				e.MustDo(http.MethodPost, path, body, &got)
				fileLyrics, fileCover := readBack(t, e.streamBytes(t, tr.Id))
				if part == "cover" {
					var ly api.MusicLyrics
					e.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d/lyrics", tr.Id), nil, &ly)
					if len(ly.Lines) != 1 || ly.Lines[0].Text != "旧歌词" || fileLyrics != "旧歌词" || got.LyricsSource != tr.LyricsSource || got.LyricsSynced != tr.LyricsSynced || e.src.called("/api/song/lyric") != 0 {
						t.Fatal("只匹配封面时修改或请求了歌词")
					}
					if e.src.called("/api/song/detail/") != 1 || len(fileCover) == 0 {
						t.Fatal("没有采用新的封面")
					}
				} else {
					_, newCover := get(t, e.Env, fmt.Sprintf("/music/tracks/%d/cover", tr.Id), nil)
					if !bytes.Equal(oldCover, newCover) || !bytes.Equal(fileCover, fixture(t, "small.png")) || got.CoverSource != tr.CoverSource || e.src.called("/api/song/detail/") != 0 {
						t.Fatal("只匹配歌词时修改或请求了封面")
					}
					if !got.LyricsSynced || !strings.Contains(fileLyrics, "第一句") {
						t.Fatal("没有采用新的歌词")
					}
				}
			})
		}
	}
}

func TestApplyingCoverKeepsLyricsCandidatesPending(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "pending.mp3", idTags, nil)
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "Jay Chou", syncedLRC)
	e.src.itunesHit(tr.DurationMs, "夜曲", "Jay Chou")
	path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
	e.MustDo(http.MethodPost, path, map[string]any{"choose": true}, nil)
	var got api.MusicTrack
	e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]any{"source": "itunes", "sourceId": "5"}, "parts": []string{"cover"}}, &got)
	if !got.HasCover || got.LyricsSource != "none" || got.MatchState != "pending" || e.src.called("/api/search") != 1 {
		t.Fatalf("选封面后应保留歌词候选，不应重新搜歌词：%+v", got)
	}
	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 1 || len(pending[0].Candidates) != 1 || pending[0].Candidates[0].Source != "lrclib" {
		t.Fatalf("应该只留下歌词候选：%+v", pending)
	}
	e.MustDo(http.MethodPost, path, map[string]any{"candidate": map[string]any{"source": "lrclib", "sourceId": "11"}, "parts": []string{"lyrics"}}, &got)
	if got.MatchState != "matched" || !got.HasCover || !got.LyricsSynced {
		t.Fatalf("补齐歌词后应该完成匹配：%+v", got)
	}
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 0 {
		t.Fatalf("完成后应清空候选：%+v", pending)
	}
}

func TestCoverCandidateCannotSupplyRequestedLyrics(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "cover.mp3", idTags, nil)
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")
	path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
	e.MustDo(http.MethodPost, path, map[string]any{"choose": true}, nil)
	body := map[string]any{"candidate": map[string]any{"source": "itunes", "sourceId": "5"}, "parts": []string{"lyrics"}}
	var failure struct{ Message string }
	status, raw := e.Do(http.MethodPost, path, body, nil)
	if err := json.Unmarshal(raw, &failure); err != nil {
		t.Fatalf("错误响应格式不对：%v", err)
	}
	if status != http.StatusBadRequest || failure.Message != "这个候选没有歌词" {
		t.Fatalf("只要歌词而候选没有时应该返回明确的 400：%d %+v", status, failure)
	}
	if e.src.called("/api/search") != 1 || e.src.called("/art/1000x1000bb.jpg") != 0 || e.track(t, tr.Id).HasCover {
		t.Fatal("拒绝候选时不应搜索其他来源或修改歌曲")
	}

	// 不传 parts 时，候选有封面就用封面，不报错，也不去别处找歌词。
	delete(body, "parts")
	e.MustDo(http.MethodPost, path, body, nil)
	got := e.track(t, tr.Id)
	if !got.HasCover || got.LyricsSource != "none" || e.src.called("/api/search") != 1 {
		t.Fatalf("不传 parts 应该只用候选有的封面：%+v", got)
	}
}

func TestChoosePartsLimitsEachPartAndDeduplicates(t *testing.T) {
	e := setupMatch(t)
	_, tr := e.add(t, "choose.mp3", idTags, nil)
	var songs []map[string]any
	for id := 1; id <= 10; id++ {
		songs = append(songs, map[string]any{"id": id, "name": "夜曲", "duration": tr.DurationMs, "artists": []map[string]any{{"name": "周杰伦"}}})
	}
	e.src.on("/api/search/get/web", jsonReply(map[string]any{"code": 200, "result": map[string]any{"songs": songs}}))
	e.src.lrclibHit(float64(tr.DurationMs)/1000, "夜曲", "周杰伦", syncedLRC)
	e.src.itunesHit(tr.DurationMs, "夜曲", "周杰伦")
	path := fmt.Sprintf("/music/tracks/%d/match", tr.Id)
	for _, tc := range []struct {
		parts []string
		count int
	}{
		{[]string{"lyrics", "cover"}, 8}, {[]string{"lyrics"}, 8}, {[]string{"cover"}, 8},
	} {
		e.MustDo(http.MethodPost, path, map[string]any{"choose": true, "parts": tc.parts}, nil)
		var pending []api.MusicPending
		e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
		if len(pending) != 1 || len(pending[0].Candidates) != tc.count {
			t.Fatalf("候选上限或去重错误：%+v", pending)
		}
		seen := map[string]bool{}
		for _, c := range pending[0].Candidates {
			key := string(c.Source) + "/" + c.SourceId
			if seen[key] {
				t.Fatalf("重复候选：%s", key)
			}
			seen[key] = true
			if len(tc.parts) == 1 && (tc.parts[0] == "lyrics" && !c.Lyrics || tc.parts[0] == "cover" && !c.Cover) {
				t.Fatalf("候选不能提供请求的部分：%+v", c)
			}
		}
	}
	var lyrics []map[string]any
	var covers []map[string]any
	for id := 1; id <= 10; id++ {
		lyrics = append(lyrics, map[string]any{"id": id, "trackName": "夜曲", "artistName": "周杰伦", "duration": float64(tr.DurationMs) / 1000, "syncedLyrics": syncedLRC})
		covers = append(covers, map[string]any{"trackId": id, "trackName": "夜曲", "artistName": "周杰伦", "trackTimeMillis": tr.DurationMs, "artworkUrl100": e.src.srv.URL + "/art/100x100bb.jpg"})
	}
	e.src.on("/api/search", jsonReply(lyrics))
	e.src.on("/search", jsonReply(map[string]any{"results": covers}))
	e.src.on("/api/search/get/web", jsonReply(map[string]any{"code": 200, "result": map[string]any{"songs": []any{}}}))
	e.MustDo(http.MethodPost, path, map[string]any{"choose": true}, nil)
	var pending []api.MusicPending
	e.MustDo(http.MethodGet, "/music/pending", nil, &pending)
	if len(pending) != 1 || len(pending[0].Candidates) != 16 {
		t.Fatalf("两部分都要时应保留各 8 个候选：%+v", pending)
	}
	for _, parts := range [][]string{{}, {"other"}} {
		if status, _ := e.Do(http.MethodPost, path, map[string]any{"parts": parts}, nil); status != http.StatusBadRequest {
			t.Fatalf("无效 parts 应返回 400：%v，%d", parts, status)
		}
	}
}
