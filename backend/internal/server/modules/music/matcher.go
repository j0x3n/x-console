package music

import (
	"bytes"
	"context"
	"errors"
	"image"
	"log/slog"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// OptionsKey holds the matching options.
const OptionsKey = "music.options"

// options are the user's choices for online matching.
type options struct {
	AutoMatch bool            `json:"autoMatch"`
	WriteBack bool            `json:"writeBack"`
	Providers map[string]bool `json:"providers"`
	Focus     focusOptions    `json:"focus"`
}

// focusOptions tie the music to the pomodoro (B150). The browser does the
// playing; these are only stored here so every device agrees.
type focusOptions struct {
	AutoPlay      bool  `json:"autoPlay"`
	PlaylistID    int64 `json:"playlistId"`
	AutoPause     bool  `json:"autoPause"`
	OnlyFocusList bool  `json:"onlyFocusList"`
}

func defaultOptions() options {
	p := map[string]bool{}
	for _, s := range AllSources {
		p[s] = true
	}
	return options{AutoMatch: true, WriteBack: true, Providers: p}
}

func (m *Module) options(ctx context.Context) options {
	o := defaultOptions()
	var saved options
	if err := m.d.Settings.Get(ctx, OptionsKey, &saved); err != nil {
		if !errors.Is(err, settings.ErrNotSet) {
			slog.Warn("music: cannot read options", "err", err)
		}
		return o
	}
	o.AutoMatch, o.WriteBack, o.Focus = saved.AutoMatch, saved.WriteBack, saved.Focus
	for k, v := range saved.Providers {
		if _, known := o.Providers[k]; known {
			o.Providers[k] = v
		}
	}
	return o
}

// enabled returns the providers in the order they are tried.
func (o options) enabled(set *providerSet) []provider {
	var out []provider
	for _, name := range AllSources {
		if o.Providers[name] {
			out = append(out, set.get(name))
		}
	}
	return out
}

const minCoverEdge = 500

// coverBigEnough reports whether the short side is at least 500 pixels.
func coverBigEnough(data []byte) bool {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	return err == nil && min(cfg.Width, cfg.Height) >= minCoverEdge
}

func queryOf(t db.MusicTrack) query {
	return query{Title: t.Title, Artist: t.Artist, Album: t.Album, DurationMs: int(t.DurationMs)}
}

// found is what a match run learned about a song.
type found struct {
	lyrics     string
	cover      []byte
	candidates []candidate // for the user to choose from, when nothing was accepted
}

// search asks every provider and fills in what is wanted from the candidates
// that pass the rules. It never fails because of one provider.
func (m *Module) search(ctx context.Context, o options, q query, wantLyrics, wantCover bool) found {
	var all []candidate
	for _, p := range o.enabled(m.providers) {
		cands, err := p.Search(ctx, q)
		if err != nil {
			slog.Debug("music: source search failed", "source", p.Name(), "err", err)
			continue
		}
		all = append(all, cands...)
	}
	var res found
	var plain string
	if wantLyrics {
		for _, c := range all {
			if !c.Lyrics || !accepts(q, c) {
				continue
			}
			text, err := m.providers.get(c.Source).FetchLyrics(ctx, c)
			if err != nil || strings.TrimSpace(text) == "" {
				continue
			}
			if isSyncedLyrics(text) {
				res.lyrics = text
				break
			}
			if plain == "" {
				plain = text
			}
		}
		if res.lyrics == "" {
			res.lyrics = plain
		}
	}
	if wantCover {
		for _, c := range all {
			if !c.Cover || !accepts(q, c) {
				continue
			}
			data, err := m.providers.get(c.Source).FetchCover(ctx, c)
			if err != nil || !coverBigEnough(data) {
				continue
			}
			res.cover = data
			break
		}
	}
	if res.lyrics == "" && res.cover == nil {
		res.candidates = rank(q, all, 5)
	}
	return res
}

// applyFound stores what was found, writes it into the file when that is on,
// and returns the new match state.
func (m *Module) applyFound(ctx context.Context, o options, t db.MusicTrack, lyrics string, cover []byte, source string) error {
	now := time.Now().UTC()
	lyricsSource, coverSource := source, source
	var coverKey string
	if len(cover) > 0 {
		key, err := m.saveCover(ctx, cover)
		if err != nil {
			cover = nil
		} else {
			coverKey = key
		}
	}
	// Write into the file first: when it works the library says "embedded",
	// when it does not (or is off) the data still lives in the library.
	if o.WriteBack && (lyrics != "" || len(cover) > 0) {
		sha, err := m.writeBack(ctx, t, lyrics, cover)
		if err != nil {
			slog.Warn("music: cannot write tags back", "track", t.ID, "err", err)
		} else {
			lyricsSource, coverSource = "embedded", "embedded"
			if err := m.q.SetTrackSha(ctx, db.SetTrackShaParams{Sha256: sha, ID: t.ID}); err != nil {
				return err
			}
		}
	}
	if lyrics != "" {
		synced := int64(0)
		if isSyncedLyrics(lyrics) {
			synced = 1
		}
		if err := m.q.SetTrackLyrics(ctx, db.SetTrackLyricsParams{LyricsText: lyrics, LyricsSource: lyricsSource, LyricsSynced: synced, UpdatedAt: now, ID: t.ID}); err != nil {
			return err
		}
	}
	if coverKey != "" {
		if err := m.q.SetTrackCover(ctx, db.SetTrackCoverParams{CoverKey: coverKey, CoverSource: coverSource, UpdatedAt: now, ID: t.ID}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) saveCandidates(ctx context.Context, trackID int64, cands []candidate) error {
	if err := m.q.ClearCandidates(ctx, trackID); err != nil {
		return err
	}
	for i, c := range cands {
		if err := m.q.InsertCandidate(ctx, db.InsertCandidateParams{TrackID: trackID, Source: c.Source, SourceID: c.SourceID, Title: c.Title, Artist: c.Artist,
			Album: c.Album, DurationMs: int64(c.DurationMs), HasLyrics: b2i(c.Lyrics), HasCover: b2i(c.Cover), LyricsText: c.LyricsText, CoverRef: c.coverRef, Position: int64(i)}); err != nil {
			return err
		}
	}
	return nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// matchTrack runs one automatic match. force (the user asked) also looks for
// lyrics and covers the song already has and replaces them.
func (m *Module) matchTrack(ctx context.Context, t db.MusicTrack, force bool) (db.MusicTrack, error) {
	m.matchMu.Lock()
	defer m.matchMu.Unlock()
	o := m.options(ctx)
	wantLyrics := force || t.LyricsSource == "none"
	wantCover := force || t.CoverSource == "none"
	if !wantLyrics && !wantCover {
		return t, nil
	}
	res := m.search(ctx, o, queryOf(t), wantLyrics, wantCover)
	now := time.Now().UTC()
	state := "matched"
	switch {
	case res.lyrics != "" || res.cover != nil:
		if err := m.applyFound(ctx, o, t, res.lyrics, res.cover, "online"); err != nil {
			return t, err
		}
		if err := m.q.ClearCandidates(ctx, t.ID); err != nil {
			return t, err
		}
	case len(res.candidates) > 0:
		state = "pending"
		if err := m.saveCandidates(ctx, t.ID, res.candidates); err != nil {
			return t, err
		}
	default:
		state = "failed"
	}
	if err := m.q.SetMatchState(ctx, db.SetMatchStateParams{MatchState: state, UpdatedAt: now, ID: t.ID}); err != nil {
		return t, err
	}
	return m.q.GetTrack(ctx, t.ID)
}

// matchProgress is published while a batch runs.
type matchProgress struct {
	Running bool   `json:"running"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current,omitempty"`
}

// runMatches matches the songs that have not been tried, one by one. With
// retryFailed it also tries the ones that failed before.
func (m *Module) runMatches(ctx context.Context, retryFailed bool) {
	if !m.matching.CompareAndSwap(false, true) {
		return
	}
	defer m.matching.Store(false)
	retry := int64(0)
	if retryFailed {
		retry = 1
	}
	total, err := m.q.CountTracksToMatch(ctx, retry)
	if err != nil || total == 0 {
		return
	}
	done := 0
	m.d.Bus.Publish("music.match_progress", matchProgress{Running: true, Total: int(total)})
	defer func() {
		m.d.Bus.Publish("music.match_progress", matchProgress{Running: false, Done: done, Total: int(total)})
		m.d.Bus.Publish("music.library_changed", map[string]int{"matched": done})
	}()
	for ctx.Err() == nil {
		rows, err := m.q.ListTracksToMatch(ctx, db.ListTracksToMatchParams{RetryFailed: retry, MaxRows: 20})
		if err != nil || len(rows) == 0 {
			return
		}
		for _, t := range rows {
			if ctx.Err() != nil {
				return
			}
			m.d.Bus.Publish("music.match_progress", matchProgress{Running: true, Done: done, Total: int(total), Current: t.Title})
			if _, err := m.matchTrack(ctx, t, false); err != nil {
				// Never try the same song twice in one run.
				slog.Warn("music: match failed", "track", t.ID, "err", err)
				_ = m.q.SetMatchState(ctx, db.SetMatchStateParams{MatchState: "failed", UpdatedAt: time.Now().UTC(), ID: t.ID})
			}
			done++
			select {
			case <-time.After(m.matchPause):
			case <-ctx.Done():
				return
			}
		}
	}
}
