package music

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
)

func toCandidate(c db.MusicCandidate) api.MusicCandidate {
	return api.MusicCandidate{Source: api.MusicCandidateSource(c.Source), SourceId: c.SourceID, Title: c.Title, Artist: c.Artist, Album: c.Album,
		DurationMs: int(c.DurationMs), Lyrics: c.HasLyrics != 0, Cover: c.HasCover != 0}
}

func (m *Module) ListMusicPending(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := m.q.ListPendingTracks(ctx)
	if fail(w, r, err) {
		return
	}
	out := make([]api.MusicPending, 0, len(rows))
	for _, t := range rows {
		t.LyricsText = ""
		cands, err := m.q.ListCandidates(ctx, t.ID)
		if fail(w, r, err) {
			return
		}
		item := api.MusicPending{Track: toTrack(t), Candidates: make([]api.MusicCandidate, 0, len(cands))}
		for _, c := range cands {
			item.Candidates = append(item.Candidates, toCandidate(c))
		}
		out = append(out, item)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) MatchMusic(w http.ResponseWriter, r *http.Request, p api.MatchMusicParams) {
	m.requestMatch(p.RetryFailed != nil && *p.RetryFailed)
	httpx.JSON(w, http.StatusAccepted, api.MusicScanStatus{Running: true})
}

func (m *Module) MatchMusicTrack(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	var body api.MusicMatchRequest
	if r.ContentLength != 0 {
		if err := httpx.Decode(r, &body); fail(w, r, err) {
			return
		}
	}
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	// A match can take a while (several sources, each with its own timeout).
	ctx, cancel := contextWithTimeout(ctx, 45*time.Second)
	defer cancel()
	if body.Candidate != nil {
		t, err = m.applyCandidate(ctx, t, body.Candidate.Source, body.Candidate.SourceId)
	} else {
		t, err = m.matchTrack(ctx, t, body.Overwrite != nil && *body.Overwrite)
	}
	if fail(w, r, err) {
		return
	}
	t.LyricsText = ""
	m.d.Bus.Publish("music.track_updated", toTrack(t))
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

// applyCandidate uses the candidate the user chose from the pending list.
func (m *Module) applyCandidate(ctx context.Context, t db.MusicTrack, source, sourceID string) (db.MusicTrack, error) {
	row, err := m.q.GetCandidate(ctx, db.GetCandidateParams{TrackID: t.ID, Source: source, SourceID: sourceID})
	if errors.Is(err, sql.ErrNoRows) {
		return t, httpx.Invalid("这个候选已经不在列表里了")
	}
	if err != nil {
		return t, err
	}
	prov := m.providers.get(row.Source)
	if prov == nil {
		return t, httpx.Invalid("不认识的来源")
	}
	c := candidate{Source: row.Source, SourceID: row.SourceID, Title: row.Title, Artist: row.Artist, Album: row.Album, DurationMs: int(row.DurationMs),
		Lyrics: row.HasLyrics != 0, Cover: row.HasCover != 0, LyricsText: row.LyricsText, coverRef: row.CoverRef}

	m.matchMu.Lock()
	defer m.matchMu.Unlock()
	o := m.options(ctx)
	var lyrics string
	var cover []byte
	if c.Lyrics {
		if lyrics, err = prov.FetchLyrics(ctx, c); err != nil {
			lyrics = ""
		}
	}
	if c.Cover {
		if cover, err = prov.FetchCover(ctx, c); err != nil {
			cover = nil
		}
	}
	if strings.TrimSpace(lyrics) == "" && cover == nil {
		return t, httpx.NewError(http.StatusBadGateway, "source_failed", "这个来源现在取不到歌词和封面，可以换一个候选或稍后再试")
	}
	if err := m.applyFound(ctx, o, t, strings.TrimSpace(lyrics), cover, "online"); err != nil {
		return t, err
	}
	if err := m.q.ClearCandidates(ctx, t.ID); err != nil {
		return t, err
	}
	if err := m.q.SetMatchState(ctx, db.SetMatchStateParams{MatchState: "matched", UpdatedAt: time.Now().UTC(), ID: t.ID}); err != nil {
		return t, err
	}
	return m.q.GetTrack(ctx, t.ID)
}

func (m *Module) SkipMusicMatch(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	if _, err := m.track(ctx, id); fail(w, r, err) {
		return
	}
	if fail(w, r, m.q.ClearCandidates(ctx, id)) {
		return
	}
	if fail(w, r, m.q.SetMatchState(ctx, db.SetMatchStateParams{MatchState: "skipped", UpdatedAt: time.Now().UTC(), ID: id})) {
		return
	}
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	t.LyricsText = ""
	m.d.Bus.Publish("music.track_updated", toTrack(t))
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

func (m *Module) PutMusicTrackLyrics(w http.ResponseWriter, r *http.Request, id api.TrackId) {
	ctx := r.Context()
	var body struct {
		Text      string `json:"text"`
		WriteBack *bool  `json:"writeBack"`
	}
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	text := strings.TrimSpace(strings.ReplaceAll(body.Text, "\r\n", "\n"))
	if text == "" || len(text) > 512<<10 {
		httpx.Fail(w, r, httpx.Invalid("歌词不能为空，也不能超过 512 KB"))
		return
	}
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	m.matchMu.Lock()
	o := m.options(ctx)
	if body.WriteBack != nil {
		o.WriteBack = *body.WriteBack
	}
	err = m.applyFound(ctx, o, t, text, nil, "online")
	m.matchMu.Unlock()
	if fail(w, r, err) {
		return
	}
	m.replyTrack(w, r, id)
}

func (m *Module) UploadMusicTrackCover(w http.ResponseWriter, r *http.Request, id api.TrackId, p api.UploadMusicTrackCoverParams) {
	ctx := r.Context()
	t, err := m.track(ctx, id)
	if fail(w, r, err) {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxCoverBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxCoverBytes {
		httpx.Fail(w, r, httpx.Invalid("封面图不能为空，也不能超过 20 MB"))
		return
	}
	if _, err := normalizeCover(data); err != nil {
		httpx.Fail(w, r, httpx.Invalid("这不是能用的图片（要 JPEG、PNG 或 WebP）"))
		return
	}
	m.matchMu.Lock()
	o := m.options(ctx)
	if p.WriteBack != nil {
		o.WriteBack = *p.WriteBack
	}
	err = m.applyFound(ctx, o, t, "", data, "online")
	m.matchMu.Unlock()
	if fail(w, r, err) {
		return
	}
	m.replyTrack(w, r, id)
}

func (m *Module) replyTrack(w http.ResponseWriter, r *http.Request, id int64) {
	t, err := m.track(r.Context(), id)
	if fail(w, r, err) {
		return
	}
	t.LyricsText = ""
	m.d.Bus.Publish("music.track_updated", toTrack(t))
	httpx.JSON(w, http.StatusOK, toTrack(t))
}

func contextWithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
