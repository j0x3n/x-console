package music

import (
	"context"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

const (
	defaultStatsDays = 30
	maxStatsDays     = 365
	topLimit         = 10
)

// recordPlay stores one listen. When a pomodoro is running it is marked with
// the session, so the focus statistics can be told apart.
func (m *Module) recordPlay(ctx context.Context, t db.MusicTrack, seconds int, now time.Time) error {
	var focusID *int64
	if f, ok := module.Lookup[contracts.FocusState](m.d.Registry, contracts.FocusStateKey); ok {
		if id, running := f.CurrentSessionID(ctx); running {
			focusID = &id
		}
	}
	track := t.ID
	return m.q.InsertPlay(ctx, db.InsertPlayParams{TrackID: &track, Title: t.Title, Artist: t.Artist,
		PlayedAt: now.Add(-time.Duration(seconds) * time.Second), Seconds: int64(seconds), FocusSessionID: focusID})
}

func (m *Module) GetMusicStats(w http.ResponseWriter, r *http.Request, p api.GetMusicStatsParams) {
	ctx := r.Context()
	days := defaultStatsDays
	if p.Days != nil {
		days = *p.Days
	}
	if days < 1 || days > maxStatsDays {
		httpx.Fail(w, r, httpx.Invalid("天数要在 1 到 365 之间"))
		return
	}
	loc := m.d.Config.Location
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(days - 1)).UTC()

	out := api.MusicStats{Days: days, TopTracks: []api.MusicPlayStat{}, TopArtists: []api.MusicArtistStat{}, FocusTopTracks: []api.MusicPlayStat{}}
	err := m.d.DB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(seconds),0),
 COALESCE(SUM(CASE WHEN focus_session_id IS NOT NULL THEN seconds END),0), COUNT(focus_session_id)
 FROM music_plays WHERE played_at >= ?`, start).Scan(&out.Plays, &out.TotalSeconds, &out.FocusSeconds, &out.FocusPlays)
	if fail(w, r, err) {
		return
	}
	tracks := func(focusOnly bool) ([]api.MusicPlayStat, error) {
		where := ""
		if focusOnly {
			where = " AND focus_session_id IS NOT NULL"
		}
		rows, err := m.d.DB.QueryContext(ctx, `SELECT track_id, title, artist, COUNT(*), COALESCE(SUM(seconds),0)
 FROM music_plays WHERE played_at >= ?`+where+`
 GROUP BY COALESCE(track_id, -1), title, artist
 ORDER BY COUNT(*) DESC, SUM(seconds) DESC, title LIMIT ?`, start, topLimit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		list := []api.MusicPlayStat{}
		for rows.Next() {
			var s api.MusicPlayStat
			var id *int64
			if err := rows.Scan(&id, &s.Title, &s.Artist, &s.Plays, &s.Seconds); err != nil {
				return nil, err
			}
			s.TrackId = id
			list = append(list, s)
		}
		return list, rows.Err()
	}
	if out.TopTracks, err = tracks(false); fail(w, r, err) {
		return
	}
	if out.FocusTopTracks, err = tracks(true); fail(w, r, err) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, `SELECT artist, COUNT(*), COALESCE(SUM(seconds),0) FROM music_plays WHERE played_at >= ?
 GROUP BY artist ORDER BY COUNT(*) DESC, SUM(seconds) DESC, artist LIMIT ?`, start, topLimit)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var a api.MusicArtistStat
		if err := rows.Scan(&a.Artist, &a.Plays, &a.Seconds); fail(w, r, err) {
			return
		}
		out.TopArtists = append(out.TopArtists, a)
	}
	if fail(w, r, rows.Err()) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
