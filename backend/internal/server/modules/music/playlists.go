package music

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

const (
	maxPlaylistName  = 100
	maxPlaylistItems = 10000
)

func playlistName(s string) (string, error) {
	name := cleanTag(s)
	if name == "" {
		return "", httpx.Invalid("名字不能为空")
	}
	if len([]rune(name)) > maxPlaylistName {
		return "", httpx.Invalid("名字太长")
	}
	return name, nil
}

func (m *Module) playlistSummary(ctx context.Context, q *db.Queries, id int64) (api.MusicPlaylist, error) {
	p, err := q.GetPlaylist(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return api.MusicPlaylist{}, httpx.ErrNotFound
	}
	if err != nil {
		return api.MusicPlaylist{}, err
	}
	covers, err := q.ListPlaylistCovers(ctx, id)
	if err != nil {
		return api.MusicPlaylist{}, err
	}
	if covers == nil {
		covers = []int64{}
	}
	return api.MusicPlaylist{Id: p.ID, Name: p.Name, TrackCount: int(p.TrackCount), DurationMs: int(p.DurationMs), CoverTrackIds: covers,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}, nil
}

func (m *Module) playlistDetail(ctx context.Context, id int64) (api.MusicPlaylistDetail, error) {
	sum, err := m.playlistSummary(ctx, m.q, id)
	if err != nil {
		return api.MusicPlaylistDetail{}, err
	}
	rows, err := m.q.ListPlaylistTracks(ctx, id)
	if err != nil {
		return api.MusicPlaylistDetail{}, err
	}
	for i := range rows {
		rows[i].LyricsText = "" // never sent in lists
	}
	return api.MusicPlaylistDetail{
		Id: sum.Id, Name: sum.Name, TrackCount: sum.TrackCount, DurationMs: sum.DurationMs, CoverTrackIds: sum.CoverTrackIds,
		CreatedAt: sum.CreatedAt, UpdatedAt: sum.UpdatedAt, Tracks: toTracks(rows),
	}, nil
}

func (m *Module) ListMusicPlaylists(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListPlaylists(ctx)
	if fail(w, r, err) {
		return
	}
	out := make([]api.MusicPlaylist, 0, len(rows))
	for _, p := range rows {
		covers, err := m.q.ListPlaylistCovers(ctx, p.ID)
		if fail(w, r, err) {
			return
		}
		if covers == nil {
			covers = []int64{}
		}
		out = append(out, api.MusicPlaylist{Id: p.ID, Name: p.Name, TrackCount: int(p.TrackCount), DurationMs: int(p.DurationMs),
			CoverTrackIds: covers, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) CreateMusicPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.MusicPlaylistCreate
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	name, err := playlistName(body.Name)
	if fail(w, r, err) {
		return
	}
	var ids []int64
	if body.TrackIds != nil {
		ids = *body.TrackIds
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	now := time.Now().UTC()
	id, err := q.InsertPlaylist(ctx, db.InsertPlaylistParams{Name: name, CreatedAt: now, UpdatedAt: now})
	if fail(w, r, err) {
		return
	}
	if fail(w, r, m.fillPlaylist(ctx, q, id, ids, 0)) {
		return
	}
	if fail(w, r, tx.Commit()) {
		return
	}
	d, err := m.playlistDetail(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("music.playlist_changed", map[string]int64{"id": id})
	httpx.JSON(w, http.StatusCreated, d)
}

// fillPlaylist appends tracks from position start. Duplicates (also those
// already in the list) are skipped, unknown tracks are an error.
func (m *Module) fillPlaylist(ctx context.Context, q *db.Queries, id int64, tracks []int64, start int64) error {
	if len(tracks) > maxPlaylistItems {
		return httpx.Invalid("一次加的歌太多")
	}
	seen := map[int64]bool{}
	var unique []int64
	for _, t := range tracks {
		if !seen[t] {
			seen[t] = true
			unique = append(unique, t)
		}
	}
	if len(unique) == 0 {
		return nil
	}
	n, err := q.CountTracksByIDs(ctx, unique)
	if err != nil {
		return err
	}
	if int(n) != len(unique) {
		return httpx.Invalid("有的歌不存在")
	}
	pos := start
	for _, t := range unique {
		if err := q.AddPlaylistItem(ctx, db.AddPlaylistItemParams{PlaylistID: id, TrackID: t, Position: pos}); err != nil {
			return err
		}
		pos++
	}
	return nil
}

func (m *Module) GetMusicPlaylist(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	d, err := m.playlistDetail(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (m *Module) UpdateMusicPlaylist(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	ctx := r.Context()
	var body api.UpdateMusicPlaylistJSONBody
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if _, err := m.playlistSummary(ctx, m.q, id); fail(w, r, err) {
		return
	}
	if body.Name != nil {
		name, err := playlistName(*body.Name)
		if fail(w, r, err) {
			return
		}
		if fail(w, r, m.q.RenamePlaylist(ctx, db.RenamePlaylistParams{Name: name, UpdatedAt: time.Now().UTC(), ID: id})) {
			return
		}
	}
	d, err := m.playlistDetail(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("music.playlist_changed", map[string]int64{"id": id})
	httpx.JSON(w, http.StatusOK, d)
}

func (m *Module) DeleteMusicPlaylist(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	ctx := r.Context()
	if _, err := m.playlistSummary(ctx, m.q, id); fail(w, r, err) {
		return
	}
	if fail(w, r, m.q.DeletePlaylist(ctx, id)) {
		return
	}
	m.d.Bus.Publish("music.playlist_changed", map[string]int64{"id": id})
	httpx.NoContent(w)
}

func (m *Module) SetMusicPlaylistItems(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	m.changeItems(w, r, id, true)
}

func (m *Module) AddMusicPlaylistItems(w http.ResponseWriter, r *http.Request, id api.PlaylistId) {
	m.changeItems(w, r, id, false)
}

func (m *Module) changeItems(w http.ResponseWriter, r *http.Request, id int64, replace bool) {
	ctx := r.Context()
	var body api.MusicTrackIds
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if _, err := q.GetPlaylist(ctx, id); fail(w, r, mapNoRows(err)) {
		return
	}
	var start int64
	if replace {
		if fail(w, r, q.ClearPlaylistItems(ctx, id)) {
			return
		}
	} else if start, err = q.NextPlaylistPosition(ctx, id); fail(w, r, err) {
		return
	}
	if fail(w, r, m.fillPlaylist(ctx, q, id, body.TrackIds, start)) {
		return
	}
	if fail(w, r, q.TouchPlaylist(ctx, db.TouchPlaylistParams{UpdatedAt: time.Now().UTC(), ID: id})) {
		return
	}
	if fail(w, r, tx.Commit()) {
		return
	}
	d, err := m.playlistDetail(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("music.playlist_changed", map[string]int64{"id": id})
	httpx.JSON(w, http.StatusOK, d)
}

func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return httpx.ErrNotFound
	}
	return err
}
