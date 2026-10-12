package music

import (
	"context"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
)

// 匹配规则只通过测试文件暴露。
type MatchQuery = query
type MatchCandidate = candidate

var (
	TitlesMatch      = titlesMatch
	ArtistsOverlap   = artistsOverlap
	DurationsMatch   = durationsMatch
	AcceptsCandidate = accepts
	RankCandidates   = rank
)

// SourceLyrics 在测试中直接检查来源的歌词响应。
func (m *Module) SourceLyrics(ctx context.Context, source, id string) (string, error) {
	return m.providers.get(source).FetchLyrics(ctx, candidate{Source: source, SourceID: id, Lyrics: true})
}

func (m *Module) SourceSearch(ctx context.Context, source string) ([]MatchCandidate, error) {
	return m.providers.get(source).Search(ctx, query{Title: "晴天", Artist: "周杰伦"})
}
func (m *Module) SourceSearchQuery(ctx context.Context, source string, q MatchQuery) ([]MatchCandidate, error) {
	return m.providers.get(source).Search(ctx, q)
}
func (m *Module) SourceCover(ctx context.Context, source, ref string) ([]byte, error) {
	return m.providers.get(source).FetchCover(ctx, candidate{Source: source, Cover: true, coverRef: ref})
}
func (m *Module) SetSourceHTTPClient(client *http.Client) { m.providers.client = client }
func (m *Module) SetSourceClock(now func() time.Time) {
	m.providers.routeMu.Lock()
	defer m.providers.routeMu.Unlock()
	m.providers.now = now
}
func (m *Module) SelectedFetchAgent(ctx context.Context) string {
	if agent := m.providers.fetchAgent(ctx); agent != nil {
		return agent.ID
	}
	return ""
}

// CheckSources 在测试中用较短的请求期限验证取消行为。
func (m *Module) CheckSources(ctx context.Context, q MatchQuery) []api.MusicSourceCheck {
	return m.checkSources(ctx, q)
}

// SetSourceURLs points the lyric and cover sources at a test server and
// removes the pauses between requests.
func (m *Module) SetSourceURLs(urls map[string]string) {
	for k, v := range urls {
		m.providers.urls[k] = v
	}
	m.providers.mbInterval = 0
	m.matchPause = 0
}

// RunMatches runs the automatic match now and waits for it.
func (m *Module) RunMatches(ctx context.Context, retryFailed bool) { m.runMatches(ctx, retryFailed) }

// SetSleepUnit makes one "minute" of the sleep timer last d.
func (m *Module) SetSleepUnit(d time.Duration) { m.sleepUnit = d }

// ArmSleepAtStart re-arms a saved timer, as after a restart.
func (m *Module) ArmSleepAtStart(ctx context.Context) { m.armSleepAtStart(ctx) }
