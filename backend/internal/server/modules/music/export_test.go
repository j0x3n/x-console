package music

import "context"

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
