package music

import (
	"context"
	"time"
)

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
