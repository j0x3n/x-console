package music_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.senan.xyz/taglib"
	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func setup(t *testing.T) (*testutil.Env, *music.Module) {
	t.Helper()
	env := testutil.New(t)
	m, ok := module.Lookup[*music.Module](env.App.Deps.Registry, music.ServiceKey)
	if !ok {
		t.Fatal("music module not registered")
	}
	return env, m
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// song is the tone fixture of a format with tags and an optional cover.
func song(t *testing.T, ext string, tags map[string][]string, cover []byte) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "song."+ext)
	if err := os.WriteFile(path, fixture(t, "tone."+ext), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(tags) > 0 {
		if err := taglib.WriteTags(path, tags, 0); err != nil {
			t.Fatal(err)
		}
	}
	if len(cover) > 0 {
		if err := taglib.WriteImage(path, cover); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mkdir(t *testing.T, env *testutil.Env, parent int64, name string) int64 {
	t.Helper()
	var out struct{ Id int64 }
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"parentId": parent, "name": name}, &out)
	return out.Id
}

func upload(t *testing.T, env *testutil.Env, parent int64, name string, data []byte) int64 {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	w.Close()
	req, err := http.NewRequest(http.MethodPost, env.URL(fmt.Sprintf("/drive/upload?parent=%d", parent)), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload %s: %d %s", name, resp.StatusCode, raw)
	}
	var out struct{ Items []struct{ Id int64 } }
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Items) != 1 {
		t.Fatalf("upload result: %s", raw)
	}
	return out.Items[0].Id
}

func useFolders(t *testing.T, env *testutil.Env, ids ...int64) {
	t.Helper()
	env.MustDo(http.MethodPut, "/music/settings", map[string]any{"folders": ids}, nil)
}

func reconcile(t *testing.T, m *music.Module) {
	t.Helper()
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func tracks(t *testing.T, env *testutil.Env, query string) api.MusicTrackPage {
	t.Helper()
	var page api.MusicTrackPage
	env.MustDo(http.MethodGet, "/music/tracks"+query, nil, &page)
	return page
}

func byTitle(t *testing.T, env *testutil.Env, title string) api.MusicTrack {
	t.Helper()
	for _, tr := range tracks(t, env, "?limit=500").Items {
		if tr.Title == title {
			return tr
		}
	}
	t.Fatalf("no track titled %q", title)
	return api.MusicTrack{}
}

func get(t *testing.T, env *testutil.Env, path string, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.URL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp, raw
}

const lrc = "[ti:夜曲]\n[00:01.00]一路向北\n[00:02.50]离开有你的季节\n"

func TestIndexesEveryFormat(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	cover := fixture(t, "cover.jpg")
	for _, ext := range []string{"mp3", "flac", "m4a", "ogg", "opus", "wav"} {
		data := song(t, ext, map[string][]string{
			taglib.Title: {"夜曲 " + ext}, taglib.Artist: {"周杰伦"}, taglib.Album: {"十一月的萧邦"}, taglib.AlbumArtist: {"周杰伦"},
			taglib.TrackNumber: {"3/12"}, taglib.DiscNumber: {"1"}, taglib.Date: {"2005-11-01"}, taglib.Lyrics: {lrc},
		}, cover)
		upload(t, env, root, "tone."+ext, data)
	}
	upload(t, env, root, "readme.txt", []byte("not music"))
	upload(t, env, root, "tone.wma", []byte("unsupported"))
	reconcile(t, m)

	page := tracks(t, env, "")
	if page.Total != 6 || len(page.Items) != 6 || page.NextCursor != nil {
		t.Fatalf("page: total %d items %d", page.Total, len(page.Items))
	}
	for _, tr := range page.Items {
		if tr.Artist != "周杰伦" || tr.Album != "十一月的萧邦" || tr.AlbumArtist != "周杰伦" || tr.TrackNo != 3 || tr.DiscNo != 1 || tr.Year != 2005 {
			t.Errorf("%s tags: %+v", tr.Format, tr)
		}
		if tr.DurationMs < 900 || tr.DurationMs > 1200 {
			t.Errorf("%s duration %d", tr.Format, tr.DurationMs)
		}
		if !tr.HasCover || tr.CoverSource != "embedded" || tr.LyricsSource != "embedded" || !tr.LyricsSynced || tr.MatchState != "none" {
			t.Errorf("%s cover/lyrics: %+v", tr.Format, tr)
		}
		if tr.Title != "夜曲 "+tr.Format {
			t.Errorf("title %q for %s", tr.Title, tr.Format)
		}
	}

	// A second scan changes nothing.
	events, cancel := env.App.Deps.Bus.Subscribe("music.", 8)
	defer cancel()
	reconcile(t, m)
	select {
	case ev := <-events:
		t.Fatalf("unchanged scan published %s", ev.Topic)
	default:
	}
}

func TestNameAndFolderFallbacks(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	sub := mkdir(t, env, root, "周杰伦")
	useFolders(t, env, root)
	upload(t, env, root, "无名 - 稻香.mp3", song(t, "mp3", nil, nil))
	upload(t, env, root, "loose.mp3", song(t, "mp3", nil, nil))
	upload(t, env, sub, "track01.mp3", song(t, "mp3", nil, nil))
	reconcile(t, m)

	if tr := byTitle(t, env, "稻香"); tr.Artist != "无名" {
		t.Errorf("name split: %+v", tr)
	}
	if tr := byTitle(t, env, "loose"); tr.Artist != "" {
		t.Errorf("a song in the music folder itself got artist %q", tr.Artist)
	}
	if tr := byTitle(t, env, "track01"); tr.Artist != "周杰伦" {
		t.Errorf("folder name as artist: %+v", tr)
	}
}

func TestSiblingLyricsAndCover(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	gbk, err := simplifiedchinese.GB18030.NewEncoder().String(lrc)
	if err != nil {
		t.Fatal(err)
	}
	upload(t, env, root, "a.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"A"}}, nil))
	upload(t, env, root, "A.LRC", []byte(gbk))
	upload(t, env, root, "b.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"B"}, taglib.Lyrics: {"内嵌歌词"}}, nil))
	upload(t, env, root, "b.lrc", []byte(lrc))
	upload(t, env, root, "Folder.JPG", fixture(t, "cover.jpg"))
	upload(t, env, root, "c.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"C"}}, fixture(t, "small.png")))
	reconcile(t, m)

	a := byTitle(t, env, "A")
	if a.LyricsSource != "lrc" || !a.LyricsSynced || a.CoverSource != "folder" || !a.HasCover {
		t.Errorf("A: %+v", a)
	}
	var lyrics api.MusicLyrics
	env.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d/lyrics", a.Id), nil, &lyrics)
	if !lyrics.Synced || lyrics.Source != "lrc" || len(lyrics.Lines) != 2 || lyrics.Lines[1].Text != "离开有你的季节" || *lyrics.Lines[1].TimeMs != 2500 {
		t.Errorf("A lyrics (GBK file): %+v", lyrics)
	}
	if b := byTitle(t, env, "B"); b.LyricsSource != "embedded" || b.LyricsSynced {
		t.Errorf("embedded lyrics must win over the .lrc: %+v", b)
	}
	if c := byTitle(t, env, "C"); c.CoverSource != "embedded" {
		t.Errorf("embedded cover must win over the folder image: %+v", c)
	}
}

func TestStreamRangeAndRemoval(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	data := song(t, "mp3", map[string][]string{taglib.Title: {"S"}}, nil)
	item := upload(t, env, root, "s.mp3", data)
	reconcile(t, m)
	tr := byTitle(t, env, "S")

	resp, body := get(t, env, fmt.Sprintf("/music/tracks/%d/stream", tr.Id), nil)
	if resp.StatusCode != 200 || !bytes.Equal(body, data) || resp.Header.Get("Content-Type") != "audio/mpeg" || resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("stream: %d %s %d bytes", resp.StatusCode, resp.Header.Get("Content-Type"), len(body))
	}
	resp, body = get(t, env, fmt.Sprintf("/music/tracks/%d/stream", tr.Id), map[string]string{"Range": "bytes=10-19"})
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(body, data[10:20]) || resp.Header.Get("Content-Range") != fmt.Sprintf("bytes 10-19/%d", len(data)) {
		t.Fatalf("range: %d %q", resp.StatusCode, resp.Header.Get("Content-Range"))
	}

	// In the trash the song cannot be played even before the next scan.
	if status, raw := env.Do(http.MethodDelete, fmt.Sprintf("/drive/items/%d", item), nil, nil); status != http.StatusNoContent {
		t.Fatalf("trash: %d %s", status, raw)
	}
	if resp, _ = get(t, env, fmt.Sprintf("/music/tracks/%d/stream", tr.Id), nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("stream of trashed file: %d", resp.StatusCode)
	}
	reconcile(t, m)
	if page := tracks(t, env, ""); page.Total != 0 {
		t.Fatalf("trashed song still listed: %+v", page)
	}
	if page := tracks(t, env, "?q=S"); page.Total != 0 {
		t.Fatalf("trashed song still found: %+v", page)
	}
	if status, _ := env.Do(http.MethodGet, fmt.Sprintf("/music/tracks/%d", tr.Id), nil, nil); status != http.StatusNotFound {
		t.Fatalf("removed track: %d", status)
	}
}

func TestCovers(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	upload(t, env, root, "a.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"A"}}, fixture(t, "cover.jpg")))
	upload(t, env, root, "b.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"B"}}, nil))
	upload(t, env, root, "c.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"C"}}, fixture(t, "small.png")))
	reconcile(t, m)

	a, b, c := byTitle(t, env, "A"), byTitle(t, env, "B"), byTitle(t, env, "C")
	for size, long := range map[int]int{96: 96, 256: 256, 640: 640} {
		resp, body := get(t, env, fmt.Sprintf("/music/tracks/%d/cover?size=%d", a.Id, size), nil)
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/jpeg" || resp.Header.Get("ETag") == "" {
			t.Fatalf("cover %d: %d", size, resp.StatusCode)
		}
		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != long || h != long*2/3 {
			t.Errorf("size %d: %dx%d", size, w, h) // the fixture is 900x600
		}
	}
	// The default size is 256, and a small image is not enlarged.
	resp, body := get(t, env, fmt.Sprintf("/music/tracks/%d/cover", a.Id), nil)
	if img, _, _ := image.Decode(bytes.NewReader(body)); resp.StatusCode != 200 || img.Bounds().Dx() != 256 {
		t.Errorf("default size")
	}
	_, body = get(t, env, fmt.Sprintf("/music/tracks/%d/cover?size=640", c.Id), nil)
	if img, _, _ := image.Decode(bytes.NewReader(body)); img == nil || img.Bounds().Dx() != 200 {
		t.Errorf("small cover was resized")
	}
	if resp, _ := get(t, env, fmt.Sprintf("/music/tracks/%d/cover", b.Id), nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("no cover: %d", resp.StatusCode)
	}
	if resp, _ := get(t, env, fmt.Sprintf("/music/tracks/%d/cover?size=100", a.Id), nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad size: %d", resp.StatusCode)
	}
	// The ETag gives a 304.
	resp, _ = get(t, env, fmt.Sprintf("/music/tracks/%d/cover", a.Id), nil)
	if resp2, _ := get(t, env, fmt.Sprintf("/music/tracks/%d/cover", a.Id), map[string]string{"If-None-Match": resp.Header.Get("ETag")}); resp2.StatusCode != http.StatusNotModified {
		t.Errorf("etag: %d", resp2.StatusCode)
	}
}

func TestListSearchSortAndPaging(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	type s struct{ title, artist, album, lyrics string }
	songs := []s{
		{"Alpha", "Bee", "Two", "hello world"},
		{"Beta", "Ann", "One", "good_bye 100%"},
		{"Gamma", "Ann", "One", ""},
		{"delta", "Cee", "Three", "hello again"},
		{"Epsilon", "Ann", "Two", ""},
	}
	for i, x := range songs {
		tags := map[string][]string{taglib.Title: {x.title}, taglib.Artist: {x.artist}, taglib.Album: {x.album}, taglib.TrackNumber: {fmt.Sprint(len(songs) - i)}}
		if x.lyrics != "" {
			tags[taglib.Lyrics] = []string{x.lyrics}
		}
		upload(t, env, root, x.title+".mp3", song(t, "mp3", tags, nil))
	}
	reconcile(t, m)

	titles := func(query string) string {
		var out []string
		for _, tr := range tracks(t, env, query).Items {
			out = append(out, tr.Title)
		}
		return strings.Join(out, ",")
	}
	// Default order: artist, album, track number, title.
	if got := titles(""); got != "Gamma,Beta,Epsilon,Alpha,delta" {
		t.Errorf("artist order: %s", got)
	}
	if got := titles("?sort=title"); got != "Alpha,Beta,delta,Epsilon,Gamma" {
		t.Errorf("title order: %s", got)
	}
	if got := titles("?q=hello"); got != "Alpha,delta" { // lyrics are searched
		t.Errorf("q=hello: %s", got)
	}
	if got := titles("?q=" + url.QueryEscape("ann one")); got != "Gamma,Beta" { // every word must match
		t.Errorf("q=ann one: %s", got)
	}
	if got := titles("?q=" + url.QueryEscape("100%")); got != "Beta" { // % is not a wildcard
		t.Errorf("q=100%%: %s", got)
	}
	if got := titles("?q=" + url.QueryEscape("good_bye")); got != "Beta" {
		t.Errorf("q=good_bye: %s", got)
	}
	if got := titles("?q=" + url.QueryEscape("goodxbye")); got != "" { // _ is not a wildcard
		t.Errorf("q=goodxbye: %s", got)
	}
	if got := titles("?q=el"); got != "delta" { // under three characters: title, artist and album
		t.Errorf("q=el: %s", got)
	}
	if got := titles("?q=ld"); got != "" { // ... but not the lyrics ("world")
		t.Errorf("q=ld: %s", got)
	}
	if got := titles("?artist=Ann&album=Two"); got != "Epsilon" {
		t.Errorf("artist+album: %s", got)
	}
	// A drive file finds its song.
	if page := tracks(t, env, "?driveItemId=999999"); page.Total != 0 {
		t.Errorf("unknown drive file: %d", page.Total)
	}
	one := tracks(t, env, "?q=Alpha").Items[0]
	if page := tracks(t, env, fmt.Sprintf("?driveItemId=%d", one.DriveItemId)); page.Total != 1 || page.Items[0].Id != one.Id {
		t.Errorf("driveItemId filter: %+v", page)
	}
	if status, _ := env.Do(http.MethodGet, "/music/tracks?sort=bogus", nil, nil); status != http.StatusBadRequest {
		t.Errorf("bad sort: %d", status)
	}

	// Paging by cursor covers every song once.
	var seen []string
	query := "?sort=title&limit=2"
	for pages := 0; ; pages++ {
		page := tracks(t, env, query)
		if page.Total != 5 {
			t.Fatalf("total %d", page.Total)
		}
		for _, tr := range page.Items {
			seen = append(seen, tr.Title)
		}
		if page.NextCursor == nil {
			break
		}
		if pages > 5 {
			t.Fatal("paging does not end")
		}
		query = "?sort=title&limit=2&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	if strings.Join(seen, ",") != "Alpha,Beta,delta,Epsilon,Gamma" {
		t.Errorf("paged: %v", seen)
	}

	// Albums and artists group the songs.
	var albums []api.MusicAlbum
	env.MustDo(http.MethodGet, "/music/albums", nil, &albums)
	if len(albums) != 4 || albums[0].Album != "One" || albums[0].TrackCount != 2 || albums[0].AlbumArtist != "Ann" {
		t.Errorf("albums: %+v", albums)
	}
	var artists []api.MusicArtist
	env.MustDo(http.MethodGet, "/music/artists", nil, &artists)
	if len(artists) != 3 || artists[0].Artist != "Ann" || artists[0].TrackCount != 3 || artists[0].AlbumCount != 2 {
		t.Errorf("artists: %+v", artists)
	}
}

func TestFavoriteManualEditAndPlays(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	upload(t, env, root, "a.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"A"}, taglib.Artist: {"X"}}, nil))
	upload(t, env, root, "b.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"B"}}, nil))
	reconcile(t, m)
	a, b := byTitle(t, env, "A"), byTitle(t, env, "B")

	var got api.MusicTrack
	env.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", a.Id), map[string]any{"favorite": true}, &got)
	if !got.Favorite {
		t.Fatal("favorite not set")
	}
	if page := tracks(t, env, "?favorite=true"); page.Total != 1 || page.Items[0].Id != a.Id {
		t.Errorf("favorites: %+v", page)
	}
	env.MustDo(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", a.Id), map[string]any{"title": " 新歌名 ", "artist": "新歌手"}, &got)
	if got.Title != "新歌名" || got.Artist != "新歌手" || !got.Favorite {
		t.Errorf("edit: %+v", got)
	}
	for _, body := range []map[string]any{{"title": "  "}, {"title": strings.Repeat("长", 301)}, {"nope": 1}} {
		if status, _ := env.Do(http.MethodPatch, fmt.Sprintf("/music/tracks/%d", a.Id), body, nil); status != http.StatusBadRequest {
			t.Errorf("patch %v: %d", body, status)
		}
	}

	// A new cover image next to the song makes the scan read it again. The
	// hand-made title survives, the favorite and the cover are updated.
	upload(t, env, root, "cover.jpg", fixture(t, "cover.jpg"))
	reconcile(t, m)
	env.MustDo(http.MethodGet, fmt.Sprintf("/music/tracks/%d", a.Id), nil, &got)
	if got.Title != "新歌名" || got.Artist != "新歌手" || !got.Favorite || got.CoverSource != "folder" {
		t.Errorf("after rescan: %+v", got)
	}

	// The search index follows edits.
	if page := tracks(t, env, "?q="+url.QueryEscape("新歌名")); page.Total != 1 || page.Items[0].Id != a.Id {
		t.Errorf("search by the new title: %+v", page)
	}
	if page := tracks(t, env, "?q="+url.QueryEscape("新歌手")); page.Total != 1 {
		t.Errorf("search by the new artist: %+v", page)
	}

	// Plays: counted, listed most recent first.
	env.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/played", a.Id), map[string]any{"seconds": 31}, &got)
	if got.PlayCount != 1 || got.LastPlayedAt == nil {
		t.Errorf("played: %+v", got)
	}
	time.Sleep(10 * time.Millisecond)
	env.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/played", b.Id), map[string]any{"seconds": 100}, nil)
	env.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/played", b.Id), map[string]any{"seconds": 100}, nil)
	page := tracks(t, env, "?sort=recent")
	if page.Total != 2 || page.Items[0].Id != b.Id || page.Items[0].PlayCount != 2 {
		t.Errorf("recent: %+v", page)
	}
	if page := tracks(t, env, "?sort=plays"); page.Items[0].Id != b.Id {
		t.Errorf("plays: %+v", page)
	}
	if status, _ := env.Do(http.MethodPost, fmt.Sprintf("/music/tracks/%d/played", a.Id), map[string]any{"seconds": -1}, nil); status != http.StatusBadRequest {
		t.Errorf("negative seconds: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/music/tracks/999/played", map[string]any{"seconds": 1}, nil); status != http.StatusNotFound {
		t.Errorf("missing track: %d", status)
	}
}

func TestPlaylists(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	items := map[string]int64{}
	for _, n := range []string{"A", "B", "C"} {
		data := song(t, "mp3", map[string][]string{taglib.Title: {n}}, fixture(t, "cover.jpg"))
		items[n] = upload(t, env, root, n+".mp3", data)
	}
	reconcile(t, m)
	id := map[string]int64{}
	for _, n := range []string{"A", "B", "C"} {
		id[n] = byTitle(t, env, n).Id
	}

	var pl api.MusicPlaylistDetail
	env.MustDo(http.MethodPost, "/music/playlists", map[string]any{"name": " 跑步 ", "trackIds": []int64{id["B"], id["A"], id["B"]}}, &pl)
	if pl.Name != "跑步" || pl.TrackCount != 2 || pl.Tracks[0].Id != id["B"] || pl.Tracks[1].Id != id["A"] || len(pl.CoverTrackIds) != 2 || pl.DurationMs < 1800 {
		t.Fatalf("create: %+v", pl)
	}
	path := fmt.Sprintf("/music/playlists/%d", pl.Id)

	// Append skips what is already there.
	env.MustDo(http.MethodPost, path+"/items", map[string]any{"trackIds": []int64{id["A"], id["C"]}}, &pl)
	if got := []int64{pl.Tracks[0].Id, pl.Tracks[1].Id, pl.Tracks[2].Id}; got[0] != id["B"] || got[1] != id["A"] || got[2] != id["C"] {
		t.Errorf("append: %v", got)
	}
	// Replace sets the order.
	env.MustDo(http.MethodPut, path+"/items", map[string]any{"trackIds": []int64{id["C"], id["B"]}}, &pl)
	if pl.TrackCount != 2 || pl.Tracks[0].Id != id["C"] || pl.Tracks[1].Id != id["B"] {
		t.Errorf("replace: %+v", pl)
	}
	// An unknown song changes nothing.
	if status, _ := env.Do(http.MethodPut, path+"/items", map[string]any{"trackIds": []int64{id["A"], 9999}}, nil); status != http.StatusBadRequest {
		t.Errorf("unknown track: %d", status)
	}
	env.MustDo(http.MethodGet, path, nil, &pl)
	if pl.TrackCount != 2 || pl.Tracks[0].Id != id["C"] {
		t.Errorf("failed replace changed the list: %+v", pl)
	}
	env.MustDo(http.MethodPatch, path, map[string]any{"name": "晨跑"}, &pl)
	if pl.Name != "晨跑" {
		t.Errorf("rename: %+v", pl)
	}
	for _, body := range []map[string]any{{"name": ""}, {"name": strings.Repeat("长", 101)}} {
		if status, _ := env.Do(http.MethodPatch, path, body, nil); status != http.StatusBadRequest {
			t.Errorf("rename %v: %d", body, status)
		}
	}
	var list []api.MusicPlaylist
	env.MustDo(http.MethodGet, "/music/playlists", nil, &list)
	if len(list) != 1 || list[0].Name != "晨跑" || list[0].TrackCount != 2 {
		t.Errorf("list: %+v", list)
	}

	// A song that leaves the drive leaves the playlist.
	env.MustDo(http.MethodDelete, fmt.Sprintf("/drive/items/%d", items["C"]), nil, nil)
	reconcile(t, m)
	env.MustDo(http.MethodGet, path, nil, &pl)
	if pl.TrackCount != 1 || pl.Tracks[0].Id != id["B"] {
		t.Errorf("after removing a file: %+v", pl)
	}

	if status, _ := env.Do(http.MethodDelete, path, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete: %d", status)
	}
	if status, _ := env.Do(http.MethodGet, path, nil, nil); status != http.StatusNotFound {
		t.Errorf("deleted playlist: %d", status)
	}
	if status, _ := env.Do(http.MethodPost, "/music/playlists", map[string]any{"name": " "}, nil); status != http.StatusBadRequest {
		t.Errorf("empty name: %d", status)
	}
	// The songs are still there.
	if page := tracks(t, env, ""); page.Total != 2 {
		t.Errorf("deleting a playlist removed songs: %d", page.Total)
	}
}

func TestSettings(t *testing.T) {
	env, m := setup(t)
	root := mkdir(t, env, 0, "音乐")
	inner := mkdir(t, env, root, "周杰伦")
	other := mkdir(t, env, 0, "播客")
	upload(t, env, inner, "a.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"A"}}, nil))
	upload(t, env, other, "b.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"B"}}, nil))
	file := upload(t, env, 0, "c.mp3", song(t, "mp3", nil, nil))

	var s api.MusicSettings
	env.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if len(s.Folders) != 0 {
		t.Fatalf("default: %+v", s)
	}
	// A folder inside another chosen folder is dropped; duplicates too.
	env.MustDo(http.MethodPut, "/music/settings", map[string]any{"folders": []int64{inner, root, root, other}}, &s)
	if len(s.Folders) != 2 || s.Folders[0].Id != root || s.Folders[0].Path != "/音乐" || s.Folders[1].Path != "/播客" {
		t.Fatalf("saved: %+v", s)
	}
	for _, folders := range [][]int64{{file}, {99999}, {root, 0}} {
		if status, _ := env.Do(http.MethodPut, "/music/settings", map[string]any{"folders": folders}, nil); status != http.StatusBadRequest {
			t.Errorf("folders %v: %d", folders, status)
		}
	}
	tooMany := make([]int64, 21)
	if status, _ := env.Do(http.MethodPut, "/music/settings", map[string]any{"folders": tooMany}, nil); status != http.StatusBadRequest {
		t.Errorf("21 folders: %d", status)
	}
	env.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if len(s.Folders) != 2 {
		t.Errorf("a rejected save changed the settings: %+v", s)
	}

	reconcile(t, m)
	if page := tracks(t, env, ""); page.Total != 2 {
		t.Errorf("both folders: %d", page.Total)
	}
	// Clearing the folders empties the library; the files stay in the drive.
	env.MustDo(http.MethodPut, "/music/settings", map[string]any{"folders": []int64{}}, &s)
	reconcile(t, m)
	if page := tracks(t, env, ""); page.Total != 0 {
		t.Errorf("no folders: %d", page.Total)
	}
	// A deleted folder drops out of the settings.
	useFolders(t, env, other)
	env.MustDo(http.MethodDelete, fmt.Sprintf("/drive/items/%d", other), nil, nil)
	env.MustDo(http.MethodGet, "/music/settings", nil, &s)
	if len(s.Folders) != 0 {
		t.Errorf("trashed folder: %+v", s)
	}
}

// The scan runs by itself: when the drive changes, and after a request.
func TestBackgroundScan(t *testing.T) {
	env, _ := setup(t)
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	upload(t, env, root, "a.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"A"}}, nil))

	deadline := time.Now().Add(15 * time.Second)
	for tracks(t, env, "").Total != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the upload was not indexed in the background")
		}
		time.Sleep(100 * time.Millisecond)
	}
	var st api.MusicScanStatus
	if status, _ := env.Do(http.MethodPost, "/music/scan", nil, &st); status != http.StatusAccepted || !st.Running {
		t.Errorf("scan: %d %+v", status, st)
	}
}

func TestLargeLibrarySearchIsFast(t *testing.T) {
	env, _ := setup(t)
	conn := env.App.Deps.DB
	ctx := context.Background()
	lyrics := strings.Repeat("这是一行很长的歌词内容 la la la\n", 80) // about 2 KB
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 5000; i++ {
		res, err := tx.ExecContext(ctx, `INSERT INTO drive_items (name, is_dir, size, mime, sha256, created_at, updated_at) VALUES (?, 0, 1, 'audio/mpeg', ?, ?, ?)`,
			fmt.Sprintf("s%d.mp3", i), fmt.Sprintf("%064d", i), now, now)
		if err != nil {
			t.Fatal(err)
		}
		item, _ := res.LastInsertId()
		if _, err := tx.ExecContext(ctx, `INSERT INTO music_tracks (drive_item_id, title, artist, album, lyrics_text, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item, fmt.Sprintf("歌 %d", i), fmt.Sprintf("歌手 %d", i%200), fmt.Sprintf("专辑 %d", i%500), lyrics+fmt.Sprintf("独有词%d", i), now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"?q=" + url.QueryEscape("独有词4999"), "?q=" + url.QueryEscape("歌手 7"), "?limit=500&sort=title", "?limit=500&sort=artist"} {
		start := time.Now()
		page := tracks(t, env, q)
		elapsed := time.Since(start)
		t.Logf("%s: %d of %d in %v", q, len(page.Items), page.Total, elapsed)
		if elapsed > 2*time.Second {
			t.Errorf("%s took %v", q, elapsed)
		}
	}
	if page := tracks(t, env, "?q="+url.QueryEscape("独有词4999")); page.Total != 1 {
		t.Errorf("lyrics search: %d", page.Total)
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
