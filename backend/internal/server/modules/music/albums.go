package music

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

func (m *Module) FillMusicAlbums(w http.ResponseWriter, r *http.Request) {
	if !m.matching.CompareAndSwap(false, true) {
		httpx.Fail(w, r, httpx.NewError(http.StatusConflict, "music_busy", "正在匹配或补全信息，请等完成后再试"))
		return
	}
	tracks, err := m.q.ListTracksToFillAlbums(r.Context())
	if err != nil || len(tracks) == 0 {
		m.matching.Store(false)
		if fail(w, r, err) {
			return
		}
	} else {
		m.albumReq <- tracks
	}
	m.d.Audit.Record(r.Context(), "music.fill_albums", "", map[string]any{"total": len(tracks)}, nil)
	httpx.JSON(w, http.StatusAccepted, map[string]int{"total": len(tracks)})
}

func (m *Module) fillAlbums(ctx context.Context, tracks []db.MusicTrack) {
	done := 0
	defer func() {
		m.matching.Store(false)
		m.d.Bus.Publish("music.album_progress", matchProgress{Done: done, Total: len(tracks)})
		m.d.Bus.Publish("music.library_changed", map[string]int{"albums": done})
	}()
	m.d.Bus.Publish("music.album_progress", matchProgress{Running: true, Total: len(tracks)})
	for _, t := range tracks {
		if ctx.Err() != nil {
			return
		}
		m.d.Bus.Publish("music.album_progress", matchProgress{Running: true, Done: done, Total: len(tracks), Current: t.Title})
		if err := m.fillAlbum(ctx, t.ID); err != nil && ctx.Err() == nil {
			slog.Warn("补全歌曲信息失败", "track", t.ID, "err", err)
		}
		done++
		select {
		case <-time.After(m.matchPause):
		case <-ctx.Done():
			return
		}
	}
}

func (m *Module) fillAlbum(ctx context.Context, id int64) error {
	m.matchMu.Lock()
	defer m.matchMu.Unlock()
	t, err := m.q.GetTrack(ctx, id)
	if err != nil {
		return err
	}
	if t.Manual != 0 || t.MatchState == "skipped" || (t.Artist != "" && t.Album != "" && t.Year != 0) {
		return nil
	}
	o := m.options(ctx)
	res := m.search(ctx, o, queryOf(t), false, false)
	if res.metadata.empty() {
		return nil
	}
	return m.applyFound(ctx, o, t, "", nil, res.metadata, "online")
}
