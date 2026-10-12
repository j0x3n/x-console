package music

import (
	"bytes"
	"context"
	"errors"
	"image"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
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
	metadata   trackMetadata
	lyrics     string
	cover      []byte
	candidates []candidate // for the user to choose from, when nothing was accepted
}

// matchParts 指定这次匹配允许读取和替换的部分。
type matchParts struct {
	lyrics bool
	cover  bool
}

// collect asks every enabled source for hits. A source that fails is skipped.
func (m *Module) collect(ctx context.Context, o options, q query) []candidate {
	var all []candidate
	for _, p := range o.enabled(m.providers) {
		cands, err := p.Search(ctx, q)
		if err != nil {
			slog.Warn("音乐来源搜索失败", "source", p.Name(), "err", err)
			continue
		}
		all = append(all, cands...)
	}
	return all
}

// checkSources 同时检查所有来源，整体最多等待 20 秒。
// 只搜索一次，不下载歌词和封面，也不修改歌曲或候选。
func (m *Module) checkSources(ctx context.Context, q query) []api.MusicSourceCheck {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	o := m.options(ctx)
	out := make([]api.MusicSourceCheck, len(AllSources))
	var wg sync.WaitGroup
	for i, source := range AllSources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			route := &sourceRoute{forceDirect: true}
			ctx := context.WithValue(ctx, sourceRouteKey{}, route)
			cands, err := m.providers.get(source).Search(ctx, q)
			row := api.MusicSourceCheck{Source: api.MusicSourceCheckSource(source), Enabled: o.Providers[source], Ok: err == nil, Ms: int(time.Since(start).Milliseconds())}
			row.Via = "direct"
			if route.usedAgent {
				row.Via = "agent"
				row.AgentName = &route.agent.Name
			}
			if route.directError != nil {
				message := sourceCheckError(route.directError)
				row.DirectError = &message
			}
			if err != nil {
				row.Error = sourceCheckError(err)
			} else {
				row.Hits = len(cands)
				for _, c := range cands {
					if c.Lyrics {
						row.LyricsHits++
					}
					if c.Cover {
						row.CoverHits++
					}
				}
			}
			out[i] = row
		}()
	}
	wg.Wait()
	return out
}

func sourceCheckError(err error) string {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netErr) && netErr.Timeout() {
		return "超时"
	}
	if errors.Is(err, context.Canceled) {
		return "请求已取消"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return "连接来源失败"
	}
	if errors.Is(err, errSourceFormat) || strings.HasPrefix(err.Error(), "网易云") || strings.HasPrefix(err.Error(), "HTTP ") {
		return err.Error()
	}
	return "读取来源失败"
}

// partCandidates 分别挑选歌词和封面候选，按来源和来源编号去重。
func partCandidates(q query, all []candidate, parts matchParts, limit int) []candidate {
	var lyrics, covers []candidate
	for _, c := range all {
		if parts.lyrics && c.Lyrics {
			lyrics = append(lyrics, c)
		}
		if parts.cover && c.Cover {
			covers = append(covers, c)
		}
	}
	seen := map[[2]string]bool{}
	var out []candidate
	for _, list := range [][]candidate{rank(q, lyrics, limit), rank(q, covers, limit)} {
		for _, c := range list {
			key := [2]string{c.Source, c.SourceID}
			if !seen[key] {
				seen[key] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// chooseCandidates 把指定部分的候选放进待确认，交给用户选择。
func (m *Module) chooseCandidates(ctx context.Context, t db.MusicTrack, parts matchParts) (db.MusicTrack, error) {
	m.matchMu.Lock()
	defer m.matchMu.Unlock()
	o := m.options(ctx)
	cands := partCandidates(queryOf(t), m.collect(ctx, o, queryOf(t)), parts, 8)
	state := "pending"
	if len(cands) == 0 {
		state = "failed"
	}
	if err := m.saveCandidates(ctx, t.ID, cands); err != nil {
		return t, err
	}
	if err := m.q.SetMatchState(ctx, db.SetMatchStateParams{MatchState: state, UpdatedAt: time.Now().UTC(), ID: t.ID}); err != nil {
		return t, err
	}
	return m.q.GetTrack(ctx, t.ID)
}

// search asks every provider and fills in what is wanted from the candidates
// that pass the rules. It never fails because of one provider.
func (m *Module) search(ctx context.Context, o options, q query, wantLyrics, wantCover bool) found {
	all := m.collect(ctx, o, q)
	var res found
	// 确定歌手后，其他来源必须与这个歌手匹配。
	if q.Artist == "" {
		for _, c := range all {
			if accepts(q, c) {
				q.Artist = cleanTag(c.Artist)
				break
			}
		}
	}
	for _, c := range all {
		if q.Artist != "" && accepts(q, c) {
			res.metadata.merge(c)
		}
	}
	var plain string
	if !wantLyrics && !wantCover {
		return res
	}
	if wantLyrics {
		for _, c := range all {
			if !c.Lyrics || q.Artist == "" || !accepts(q, c) {
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
			if !c.Cover || q.Artist == "" || !accepts(q, c) {
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
	// 待确认只保留本次请求中没有找到的部分。
	res.candidates = partCandidates(q, all, matchParts{lyrics: wantLyrics && res.lyrics == "", cover: wantCover && res.cover == nil}, 5)
	return res
}

// applyFound stores what was found, writes it into the file when that is on,
// and preserves fields that already have a value or were manually edited.
func (m *Module) applyFound(ctx context.Context, o options, t db.MusicTrack, lyrics string, cover []byte, metadata trackMetadata, source string) error {
	now := time.Now().UTC()
	metadata = metadata.missingFrom(t)
	if !metadata.empty() {
		changed, err := m.q.FillTrackMetadata(ctx, db.FillTrackMetadataParams{Artist: metadata.Artist, Album: metadata.Album, AlbumArtist: metadata.AlbumArtist, Year: int64(metadata.Year), UpdatedAt: now, ID: t.ID})
		if err != nil {
			return err
		}
		if changed == 0 {
			metadata = trackMetadata{}
		}
	}
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
	if o.WriteBack && (lyrics != "" || len(cover) > 0 || !metadata.empty()) {
		sha, err := m.writeBack(ctx, t, lyrics, cover, metadata)
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
	if err := m.clearCandidates(ctx, trackID); err != nil {
		return err
	}
	var extra []candidateMetadata
	for i, c := range cands {
		if err := m.q.InsertCandidate(ctx, db.InsertCandidateParams{TrackID: trackID, Source: c.Source, SourceID: c.SourceID, Title: c.Title, Artist: c.Artist,
			Album: c.Album, DurationMs: int64(c.DurationMs), HasLyrics: b2i(c.Lyrics), HasCover: b2i(c.Cover), LyricsText: c.LyricsText, CoverRef: c.coverRef, Position: int64(i)}); err != nil {
			return err
		}
		if c.Year > 0 || c.AlbumArtist != "" {
			extra = append(extra, candidateMetadata{Source: c.Source, SourceID: c.SourceID, Year: c.Year, AlbumArtist: c.AlbumArtist})
		}
	}
	if len(extra) > 0 {
		return m.d.Settings.Set(ctx, candidateMetadataKey(trackID), extra)
	}
	return nil
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// matchTrack 用于后台匹配，默认补齐歌词和封面。
func (m *Module) matchTrack(ctx context.Context, t db.MusicTrack, force bool) (db.MusicTrack, error) {
	return m.matchTrackParts(ctx, t, force, matchParts{lyrics: true, cover: true})
}

// matchTrackParts 只查找指定部分。force 也只替换指定部分。
func (m *Module) matchTrackParts(ctx context.Context, t db.MusicTrack, force bool, parts matchParts) (db.MusicTrack, error) {
	m.matchMu.Lock()
	defer m.matchMu.Unlock()
	var err error
	if t, err = m.q.GetTrack(ctx, t.ID); err != nil {
		return t, err
	}
	o := m.options(ctx)
	wantLyrics := parts.lyrics && (force || t.LyricsSource == "none")
	wantCover := parts.cover && (force || t.CoverSource == "none")
	if !wantLyrics && !wantCover && !needsMetadata(t) {
		return t, nil
	}
	res := m.search(ctx, o, queryOf(t), wantLyrics, wantCover)
	res.metadata = res.metadata.missingFrom(t)
	if err := m.applyFound(ctx, o, t, res.lyrics, res.cover, res.metadata, "online"); err != nil {
		return t, err
	}
	now := time.Now().UTC()
	state := "matched"
	switch {
	case res.lyrics != "" || res.cover != nil || !res.metadata.empty():
		if err := m.clearCandidates(ctx, t.ID); err != nil {
			return t, err
		}
		if len(res.candidates) > 0 {
			state = "pending"
			if err := m.saveCandidates(ctx, t.ID, res.candidates); err != nil {
				return t, err
			}
		}
	case len(res.candidates) > 0:
		state = "pending"
		if err := m.saveCandidates(ctx, t.ID, res.candidates); err != nil {
			return t, err
		}
	default:
		state = "failed"
		if err := m.clearCandidates(ctx, t.ID); err != nil {
			return t, err
		}
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
