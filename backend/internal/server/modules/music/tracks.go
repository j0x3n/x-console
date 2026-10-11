package music

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

const (
	defaultTrackLimit = 100
	maxTrackLimit     = 500
	maxTextLen        = 300
)

func toTrack(t db.MusicTrack) api.MusicTrack {
	return api.MusicTrack{
		Id: t.ID, DriveItemId: t.DriveItemID, Title: t.Title, Artist: t.Artist, Album: t.Album, AlbumArtist: t.AlbumArtist,
		TrackNo: int(t.TrackNo), DiscNo: int(t.DiscNo), Year: int(t.Year), DurationMs: int(t.DurationMs), Bitrate: int(t.Bitrate),
		Format: t.Format, HasCover: t.HasCover != 0, CoverSource: api.MusicTrackCoverSource(t.CoverSource),
		LyricsSource: api.MusicTrackLyricsSource(t.LyricsSource), LyricsSynced: t.LyricsSynced != 0,
		Favorite: t.Favorite != 0, PlayCount: int(t.PlayCount), LastPlayedAt: t.LastPlayedAt,
		MatchState: api.MusicTrackMatchState(t.MatchState), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func toTracks(rows []db.MusicTrack) []api.MusicTrack {
	out := make([]api.MusicTrack, 0, len(rows))
	for _, r := range rows {
		out = append(out, toTrack(r))
	}
	return out
}

func (m *Module) track(ctx context.Context, id int64) (db.MusicTrack, error) {
	t, err := m.q.GetTrack(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return t, httpx.ErrNotFound
	}
	return t, err
}

// trackColumns is db.MusicTrack in field order. The lyrics are left out of
// lists because they are the largest column.
const trackColumns = `id, drive_item_id, sha256, companion, title, artist, album, album_artist, manual, track_no, disc_no, year,
 duration_ms, bitrate, format, has_cover, cover_source, cover_key, lyrics_source, lyrics_synced, '' AS lyrics_text, favorite,
 play_count, last_played_at, match_state, created_at, updated_at`

func scanTrack(s interface{ Scan(...any) error }) (db.MusicTrack, error) {
	var t db.MusicTrack
	err := s.Scan(&t.ID, &t.DriveItemID, &t.Sha256, &t.Companion, &t.Title, &t.Artist, &t.Album, &t.AlbumArtist, &t.Manual, &t.TrackNo,
		&t.DiscNo, &t.Year, &t.DurationMs, &t.Bitrate, &t.Format, &t.HasCover, &t.CoverSource, &t.CoverKey, &t.LyricsSource,
		&t.LyricsSynced, &t.LyricsText, &t.Favorite, &t.PlayCount, &t.LastPlayedAt, &t.MatchState, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

// like escapes a search word for LIKE ... ESCAPE '\'.
func like(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(term) + "%"
}

var trackOrder = map[string]string{
	"artist": "artist COLLATE NOCASE, album COLLATE NOCASE, disc_no, track_no, title COLLATE NOCASE, id",
	"title":  "title COLLATE NOCASE, artist COLLATE NOCASE, id",
	"album":  "album COLLATE NOCASE, disc_no, track_no, title COLLATE NOCASE, id",
	"added":  "created_at DESC, id DESC",
	"recent": "last_played_at DESC, id DESC",
	"plays":  "play_count DESC, title COLLATE NOCASE, id",
}

func (m *Module) ListMusicTracks(w http.ResponseWriter, r *http.Request, p api.ListMusicTracksParams) {
	ctx := r.Context()
	var where []string
	var args []any
	if p.Q != nil {
		for _, term := range strings.Fields(*p.Q) {
			if utf8.RuneCountInString(term) >= 3 {
				// The index covers title, artist, album and lyrics.
				where = append(where, "id IN (SELECT rowid FROM music_fts WHERE music_fts MATCH ?)")
				args = append(args, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
				continue
			}
			l := like(term)
			where = append(where, `(title LIKE ? ESCAPE '\' OR artist LIKE ? ESCAPE '\' OR album LIKE ? ESCAPE '\')`)
			args = append(args, l, l, l)
		}
	}
	if p.Artist != nil {
		where, args = append(where, "artist = ?"), append(args, *p.Artist)
	}
	if p.Album != nil {
		where, args = append(where, "album = ?"), append(args, *p.Album)
	}
	if p.Favorite != nil && *p.Favorite {
		where = append(where, "favorite = 1")
	}
	if p.DriveItemId != nil {
		where, args = append(where, "drive_item_id = ?"), append(args, *p.DriveItemId)
	}
	sortKey := "artist"
	if p.Sort != nil {
		sortKey = string(*p.Sort)
	}
	order, ok := trackOrder[sortKey]
	if !ok {
		httpx.Fail(w, r, httpx.Invalid("排序方式不正确"))
		return
	}
	if sortKey == "recent" {
		where = append(where, "last_played_at IS NOT NULL")
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	limit := defaultTrackLimit
	if p.Limit != nil {
		limit = min(max(*p.Limit, 1), maxTrackLimit)
	}
	offset := int64(0)
	if p.Cursor != nil && *p.Cursor != "" {
		o, err := httpx.DecodeIDCursor(p.Cursor)
		if fail(w, r, err) {
			return
		}
		offset = o
	}

	var total int
	if err := m.d.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM music_tracks"+clause, args...).Scan(&total); fail(w, r, err) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT "+trackColumns+" FROM music_tracks"+clause+" ORDER BY "+order+" LIMIT ? OFFSET ?",
		append(args, limit, offset)...)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	items := make([]api.MusicTrack, 0, limit)
	for rows.Next() {
		t, err := scanTrack(rows)
		if fail(w, r, err) {
			return
		}
		items = append(items, toTrack(t))
	}
	if fail(w, r, rows.Err()) {
		return
	}
	page := api.MusicTrackPage{Items: items, Total: total}
	if next := offset + int64(len(items)); next < int64(total) && len(items) > 0 {
		c := httpx.EncodeIDCursor(next)
		page.NextCursor = &c
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (m *Module) GetMusicTrack(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	t, err := m.track(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

func (m *Module) UpdateMusicTrack(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	var body api.MusicTrackPatch
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	now := time.Now().UTC()
	if body.Title != nil || body.Artist != nil || body.Album != nil || body.AlbumArtist != nil {
		title, artist, album, albumArtist := t.Title, t.Artist, t.Album, t.AlbumArtist
		set := func(dst *string, src *string) error {
			if src == nil {
				return nil
			}
			v := cleanTag(*src)
			if len([]rune(v)) > maxTextLen {
				return httpx.Invalid("内容太长")
			}
			*dst = v
			return nil
		}
		for _, e := range []error{set(&title, body.Title), set(&artist, body.Artist), set(&album, body.Album), set(&albumArtist, body.AlbumArtist)} {
			if fail(w, r, e) {
				return
			}
		}
		if title == "" {
			httpx.Fail(w, r, httpx.Invalid("歌名不能为空"))
			return
		}
		if fail(w, r, m.q.SetTrackManual(ctx, db.SetTrackManualParams{Title: title, Artist: artist, Album: album, AlbumArtist: albumArtist, UpdatedAt: now, ID: id})) {
			return
		}
	}
	if body.Favorite != nil {
		fav := int64(0)
		if *body.Favorite {
			fav = 1
		}
		if fail(w, r, m.q.SetTrackFavorite(ctx, db.SetTrackFavoriteParams{Favorite: fav, UpdatedAt: now, ID: id})) {
			return
		}
	}
	t, err = m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("music.track_updated", toTrack(t))
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

func (m *Module) StreamMusicTrack(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	drive, err := m.drive()
	if fail(w, r, err) {
		return
	}
	rc, file, err := drive.Open(ctx, t.DriveItemID)
	if fail(w, r, notFound(err)) {
		return
	}
	defer rc.Close()
	mime := audioMimes[strings.ToLower(filepath.Ext(file.Name))]
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", `"`+file.SHA256+`"`)
	http.ServeContent(w, r, file.Name, file.UpdatedAt, rc)
}

func (m *Module) GetMusicTrackCover(w http.ResponseWriter, r *http.Request, id api.TrackId, p api.GetMusicTrackCoverParams) {
	ctx := r.Context()
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	size := 256
	if p.Size != nil {
		size = int(*p.Size)
	}
	if size != 96 && size != 256 && size != 640 {
		httpx.Fail(w, r, httpx.Invalid("尺寸只能是 96、256 或 640"))
		return
	}
	if t.HasCover == 0 || t.CoverKey == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	rc, info, ok, err := m.openCover(ctx, t.CoverKey, size)
	if fail(w, r, err) {
		return
	}
	if !ok {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", fmt.Sprintf(`"%s-%d"`, t.CoverKey[:16], size))
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, "", info.ModTime, rc)
}

func (m *Module) GetMusicTrackLyrics(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	t, err := m.track(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	out := api.MusicLyrics{Source: api.MusicLyricsSource(t.LyricsSource), Lines: []api.MusicLyricLine{}}
	if t.LyricsText != "" {
		synced, lines := parseLyrics(t.LyricsText)
		out.Synced = synced
		for _, l := range lines {
			out.Lines = append(out.Lines, api.MusicLyricLine{TimeMs: l.TimeMs, Text: l.Text})
		}
	} else {
		out.Source = "none"
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ReportMusicPlayed(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	var body api.ReportMusicPlayedJSONBody
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if body.Seconds < 0 || body.Seconds > 86400 {
		httpx.Fail(w, r, httpx.Invalid("播放秒数不正确"))
		return
	}
	if _, err := m.track(ctx, id); fail(w, r, err) {
		return
	}
	now := time.Now().UTC()
	if fail(w, r, m.q.MarkTrackPlayed(ctx, db.MarkTrackPlayedParams{LastPlayedAt: &now, ID: id})) {
		return
	}
	if played, err := m.track(ctx, id); err == nil {
		_ = m.recordPlay(ctx, played, body.Seconds, now)
	}
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.d.Bus.Publish("music.played", map[string]any{"id": id, "seconds": body.Seconds})
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

func (m *Module) ListMusicAlbums(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListAlbums(r.Context())
	if fail(w, r, err) {
		return
	}
	out := make([]api.MusicAlbum, 0, len(rows))
	for _, a := range rows {
		out = append(out, api.MusicAlbum{Album: a.Album, AlbumArtist: a.AlbumArtist, Year: int(a.Year), TrackCount: int(a.TrackCount),
			DurationMs: int(a.DurationMs), CoverTrackId: a.CoverTrackID})
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ListMusicArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := m.q.ListArtists(r.Context())
	if fail(w, r, err) {
		return
	}
	out := make([]api.MusicArtist, 0, len(rows))
	for _, a := range rows {
		out = append(out, api.MusicArtist{Artist: a.Artist, TrackCount: int(a.TrackCount), AlbumCount: int(a.AlbumCount), CoverTrackId: a.CoverTrackID})
	}
	httpx.JSON(w, http.StatusOK, out)
}
