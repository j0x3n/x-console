package music

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
)

// The music actions for AI assistants and MCP (B151). They call the same
// handlers as the web page. There is no play or pause: sound comes out of the
// browser, and the server cannot play for it. Songs in hidden drive folders
// are never in the library, so they never show up here either.

func (m *Module) registerActions() {
	reg := func(a actions.Action) { m.d.Actions.Register(a) }
	reg(actions.Action{Name: "music.search", Title: "搜索音乐库", Effect: actions.Read,
		Description: "Search the music library by title, artist, album or lyrics. Returns {items, total}; each item has id, title, artist, album, durationSec, hasLyrics, hasCover and matchState. " +
			"Terms shorter than 3 characters only match title, artist and album. Use the ids with music.playlist_create and music.playlist_add.",
		Input: actions.Schema(`{"type":"object","properties":{"q":{"type":"string"},"artist":{"type":"string"},"album":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Default 20."}},"additionalProperties":false}`), Run: m.actionSearch})
	reg(actions.Action{Name: "music.scan", Title: "重新扫描音乐库", Effect: actions.Write,
		Description: "Rescan the music folders (chosen in Settings) so songs just uploaded to the drive show up. The scan runs in the background and takes seconds to minutes; call music.search afterwards. " +
			"New songs are also found by themselves a few seconds after an upload.",
		Input: actions.Schema(`{"type":"object","additionalProperties":false}`), Run: m.actionScan})
	reg(actions.Action{Name: "music.match", Title: "匹配歌词和封面", Effect: actions.Write,
		Description: "Look up missing lyrics and covers online and write them into the files. With trackId it matches that song now and returns it (can take up to 45 seconds; overwrite:true replaces lyrics and cover that already exist). " +
			"Without trackId it starts a background run over every song not tried yet (retryFailed:true also retries failed ones) and returns at once. " +
			"Songs that could not be matched with confidence are listed by music.pending.",
		Input: actions.Schema(`{"type":"object","properties":{"trackId":{"type":"integer"},"overwrite":{"type":"boolean"},"retryFailed":{"type":"boolean"}},"additionalProperties":false}`), Run: m.actionMatch})
	reg(actions.Action{Name: "music.pending", Title: "待确认的歌曲", Effect: actions.Read,
		Description: "Songs whose online match needs a person's check, each with its candidates (source, sourceId, title, artist, score). The user confirms a candidate on the music page; " +
			"use this list to tell them what is waiting.",
		Input: actions.Schema(`{"type":"object","additionalProperties":false}`), Run: m.actionPending})
	reg(actions.Action{Name: "music.playlist_create", Title: "新建播放列表", Effect: actions.Write,
		Description: "Create a playlist, optionally with songs (trackIds from music.search). Duplicates are skipped. Returns the playlist with its id.",
		Input:       actions.Schema(`{"type":"object","properties":{"name":{"type":"string"},"trackIds":{"type":"array","items":{"type":"integer"}}},"required":["name"],"additionalProperties":false}`), Run: m.actionPlaylistCreate})
	reg(actions.Action{Name: "music.playlist_add", Title: "往播放列表加歌", Effect: actions.Write,
		Description: "Append songs to the end of a playlist. Songs already in it are skipped. Returns the playlist.",
		Input:       actions.Schema(`{"type":"object","properties":{"playlistId":{"type":"integer"},"trackIds":{"type":"array","items":{"type":"integer"},"minItems":1}},"required":["playlistId","trackIds"],"additionalProperties":false}`), Run: m.actionPlaylistAdd})
}

func actionInput(raw json.RawMessage, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return httpx.Invalid("参数不正确")
	}
	return nil
}

// call runs a handler with a fake request and returns its JSON result.
func call(ctx context.Context, method, url string, body any, handler func(http.ResponseWriter, *http.Request)) (any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(raw)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		}
		_ = json.NewDecoder(res.Body).Decode(&e)
		return nil, httpx.NewError(res.StatusCode, e.Code, e.Message)
	}
	var out any
	if res.StatusCode == http.StatusNoContent {
		return map[string]bool{"ok": true}, nil
	}
	err = json.NewDecoder(res.Body).Decode(&out)
	return out, err
}

func (m *Module) actionSearch(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Q      *string `json:"q"`
		Artist *string `json:"artist"`
		Album  *string `json:"album"`
		Limit  int     `json:"limit"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	in.Limit = min(max(in.Limit, 1), 50)
	p := api.ListMusicTracksParams{Q: in.Q, Artist: in.Artist, Album: in.Album, Limit: &in.Limit}
	return call(ctx, http.MethodGet, "/music/tracks", map[string]any{}, func(w http.ResponseWriter, r *http.Request) { m.ListMusicTracks(w, r, p) })
}

func (m *Module) actionScan(ctx context.Context, raw json.RawMessage) (any, error) {
	if err := actionInput(raw, &struct{}{}); err != nil {
		return nil, err
	}
	return call(ctx, http.MethodPost, "/music/scan", map[string]any{}, m.ScanMusic)
}

func (m *Module) actionMatch(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		TrackID     *int64 `json:"trackId"`
		Overwrite   bool   `json:"overwrite"`
		RetryFailed bool   `json:"retryFailed"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	if in.TrackID == nil {
		p := api.MatchMusicParams{RetryFailed: &in.RetryFailed}
		return call(ctx, http.MethodPost, "/music/match", map[string]any{}, func(w http.ResponseWriter, r *http.Request) { m.MatchMusic(w, r, p) })
	}
	return call(ctx, http.MethodPost, "/music/tracks/match", map[string]any{"overwrite": in.Overwrite}, func(w http.ResponseWriter, r *http.Request) { m.MatchMusicTrack(w, r, *in.TrackID) })
}

func (m *Module) actionPending(ctx context.Context, raw json.RawMessage) (any, error) {
	if err := actionInput(raw, &struct{}{}); err != nil {
		return nil, err
	}
	return call(ctx, http.MethodGet, "/music/pending", map[string]any{}, m.ListMusicPending)
}

func (m *Module) actionPlaylistCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		Name     string  `json:"name"`
		TrackIDs []int64 `json:"trackIds"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return call(ctx, http.MethodPost, "/music/playlists", api.MusicPlaylistCreate{Name: in.Name, TrackIds: &in.TrackIDs}, m.CreateMusicPlaylist)
}

func (m *Module) actionPlaylistAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		PlaylistID int64   `json:"playlistId"`
		TrackIDs   []int64 `json:"trackIds"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return call(ctx, http.MethodPost, "/music/playlists/items", api.MusicTrackIds{TrackIds: in.TrackIDs}, func(w http.ResponseWriter, r *http.Request) { m.AddMusicPlaylistItems(w, r, in.PlaylistID) })
}
