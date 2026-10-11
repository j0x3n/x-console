package music

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// SleepKey holds the sleep timer, so every page and device sees the same one.
const SleepKey = "music.sleep"

// sleepState is the timer. Mode "time" ends at EndsAt; the server fires it.
// Mode "tracks" stops after TracksLeft more songs; the page that is playing
// counts them and tells the server (PUT) after each one.
type sleepState struct {
	Mode       string     `json:"mode"`
	EndsAt     *time.Time `json:"endsAt,omitempty"`
	TracksLeft int        `json:"tracksLeft,omitempty"`
}

type sleeper struct {
	mu    sync.Mutex
	timer *time.Timer
}

func (m *Module) loadSleep(ctx context.Context) *sleepState {
	var s sleepState
	if err := m.d.Settings.Get(ctx, SleepKey, &s); err != nil {
		if !errors.Is(err, settings.ErrNotSet) {
			slog.Warn("music: cannot read the sleep timer", "err", err)
		}
		return nil
	}
	if s.Mode == "" {
		return nil
	}
	return &s
}

func toSleep(s *sleepState) api.MusicSleep {
	if s == nil {
		return api.MusicSleep{}
	}
	out := api.MusicSleep{Active: true, Mode: (*api.MusicSleepMode)(&s.Mode), EndsAt: s.EndsAt}
	if s.Mode == "tracks" {
		left := s.TracksLeft
		out.TracksLeft = &left
	}
	return out
}

// setSleep saves the state, arms or disarms the server timer and tells every page.
func (m *Module) setSleep(ctx context.Context, s *sleepState) error {
	m.sleep.mu.Lock()
	defer m.sleep.mu.Unlock()
	if m.sleep.timer != nil {
		m.sleep.timer.Stop()
		m.sleep.timer = nil
	}
	if s == nil {
		if err := m.d.Settings.Delete(ctx, SleepKey); err != nil {
			return err
		}
	} else {
		if err := m.d.Settings.Set(ctx, SleepKey, s); err != nil {
			return err
		}
		if s.Mode == "time" && s.EndsAt != nil {
			endsAt := *s.EndsAt
			m.sleep.timer = time.AfterFunc(time.Until(endsAt), func() { m.fireSleep(endsAt) })
		}
	}
	m.d.Bus.Publish("music.sleep_changed", toSleep(s))
	return nil
}

// fireSleep ends a timer that ran out: every page pauses.
func (m *Module) fireSleep(endsAt time.Time) {
	ctx := context.Background()
	cur := m.loadSleep(ctx)
	// A timer that was replaced or extended meanwhile is not this one.
	if cur == nil || cur.Mode != "time" || cur.EndsAt == nil || !cur.EndsAt.Equal(endsAt) {
		return
	}
	if err := m.setSleep(ctx, nil); err != nil {
		slog.Warn("music: cannot clear the sleep timer", "err", err)
	}
	m.d.Bus.Publish("music.sleep_fired", map[string]any{"at": time.Now().UTC()})
}

// armSleepAtStart re-arms a timer saved before a restart. One that ran out
// while the server was down fires if it is recent, otherwise it is dropped.
func (m *Module) armSleepAtStart(ctx context.Context) {
	s := m.loadSleep(ctx)
	if s == nil || s.Mode != "time" || s.EndsAt == nil {
		return
	}
	if s.EndsAt.Before(time.Now().Add(-2 * m.sleepUnit)) {
		_ = m.setSleep(ctx, nil)
		return
	}
	_ = m.setSleep(ctx, s)
}

func (m *Module) GetMusicSleep(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, toSleep(m.loadSleep(r.Context())))
}

func (m *Module) PutMusicSleep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.MusicSleepInput
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	given := 0
	for _, set := range []bool{body.Minutes != nil, body.Tracks != nil, body.ExtendMinutes != nil} {
		if set {
			given++
		}
	}
	if given != 1 {
		httpx.Fail(w, r, httpx.Invalid("minutes、tracks、extendMinutes 要传一个，只能传一个"))
		return
	}
	var next sleepState
	switch {
	case body.Minutes != nil:
		if *body.Minutes < 1 || *body.Minutes > 720 {
			httpx.Fail(w, r, httpx.Invalid("分钟数要在 1 到 720 之间"))
			return
		}
		end := time.Now().UTC().Add(time.Duration(*body.Minutes) * m.sleepUnit)
		next = sleepState{Mode: "time", EndsAt: &end}
	case body.Tracks != nil:
		if *body.Tracks < 1 || *body.Tracks > 20 {
			httpx.Fail(w, r, httpx.Invalid("首数要在 1 到 20 之间"))
			return
		}
		next = sleepState{Mode: "tracks", TracksLeft: *body.Tracks}
	default:
		cur := m.loadSleep(ctx)
		if cur == nil || cur.Mode != "time" || cur.EndsAt == nil {
			httpx.Fail(w, r, httpx.Invalid("没有按分钟的定时，不能延长"))
			return
		}
		if *body.ExtendMinutes < 1 || *body.ExtendMinutes > 120 {
			httpx.Fail(w, r, httpx.Invalid("延长的分钟数要在 1 到 120 之间"))
			return
		}
		end := cur.EndsAt.Add(time.Duration(*body.ExtendMinutes) * m.sleepUnit)
		next = sleepState{Mode: "time", EndsAt: &end}
	}
	if fail(w, r, m.setSleep(ctx, &next)) {
		return
	}
	httpx.JSON(w, http.StatusOK, toSleep(&next))
}

func (m *Module) DeleteMusicSleep(w http.ResponseWriter, r *http.Request) {
	if fail(w, r, m.setSleep(r.Context(), nil)) {
		return
	}
	httpx.NoContent(w)
}
