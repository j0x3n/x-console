package me

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/me/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

const (
	settingsKey = "me.settings"
	manualKey   = "me.manual"
	maxHosts    = 100
)

var (
	homeEntityRe  = regexp.MustCompile(`^(person|device_tracker)\.[a-z0-9_]+$`)
	sleepEntityRe = regexp.MustCompile(`^(binary_sensor|input_boolean)\.[a-z0-9_]+$`)
)

// config is what the settings page saves.
type config struct {
	WorkHosts  []string `json:"workHosts"`
	GuessSleep bool     `json:"guessSleep"`
	HAHome     string   `json:"haHome"`
	HASleep    string   `json:"haSleep"`
}

func (m *Module) loadConfig(ctx context.Context) (config, error) {
	c := config{WorkHosts: []string{}, GuessSleep: true}
	if err := m.d.Settings.Get(ctx, settingsKey, &c); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return c, err
	}
	if c.WorkHosts == nil {
		c.WorkHosts = []string{}
	}
	return c, nil
}

func (m *Module) loadManual(ctx context.Context) (*manualSetting, error) {
	var s manualSetting
	if err := m.d.Settings.Get(ctx, manualKey, &s); err != nil {
		if errors.Is(err, settings.ErrNotSet) {
			return nil, nil
		}
		return nil, err
	}
	if !validState(s.State) {
		return nil, nil
	}
	return &s, nil
}

func validState(s string) bool {
	switch s {
	case contracts.MeWorking, contracts.MeAway, contracts.MeSleeping, contracts.MeOut, contracts.MeHome, contracts.MeUnknown:
		return true
	}
	return false
}

func (m *Module) settingsView(ctx context.Context) (api.MeSettings, error) {
	m.mu.Lock()
	err := m.ensureLoaded(ctx)
	cfg := m.cfg
	list := m.listHosts
	m.mu.Unlock()
	if err != nil {
		return api.MeSettings{}, err
	}
	hosts := []api.MeHost{}
	for _, h := range list(ctx) {
		hosts = append(hosts, api.MeHost{Id: h.ID, Name: h.Name, Online: h.Online, State: &h.State})
	}
	return api.MeSettings{WorkHosts: append([]string{}, cfg.WorkHosts...), Hosts: hosts, GuessSleep: cfg.GuessSleep, HaHome: cfg.HAHome, HaSleep: cfg.HASleep}, nil
}

// GetMeSettings implements api.ServerInterface.
func (m *Module) GetMeSettings(w http.ResponseWriter, r *http.Request) {
	v, err := m.settingsView(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}

// SetMeSettings implements api.ServerInterface.
func (m *Module) SetMeSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.SetMeSettingsJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	next := config{WorkHosts: []string{}, GuessSleep: body.GuessSleep, HAHome: strings.TrimSpace(body.HaHome), HASleep: strings.TrimSpace(body.HaSleep)}
	known := map[string]bool{}
	m.mu.Lock()
	list := m.listHosts
	m.mu.Unlock()
	for _, h := range list(ctx) {
		known[h.ID] = true
	}
	for _, id := range body.WorkHosts {
		if !known[id] {
			httpx.Fail(w, r, httpx.Invalid("只能选能报空闲时间的电脑"))
			return
		}
		if !slices.Contains(next.WorkHosts, id) {
			next.WorkHosts = append(next.WorkHosts, id)
		}
	}
	switch {
	case next.HAHome != "" && !homeEntityRe.MatchString(next.HAHome):
		httpx.Fail(w, r, httpx.Invalid("在家的实体要写成 person.xxx 或 device_tracker.xxx"))
		return
	case next.HASleep != "" && !sleepEntityRe.MatchString(next.HASleep):
		httpx.Fail(w, r, httpx.Invalid("睡眠专注的实体要写成 binary_sensor.xxx 或 input_boolean.xxx"))
		return
	}
	m.mu.Lock()
	err := m.ensureLoaded(ctx)
	old := m.cfg
	if err == nil {
		err = m.d.Settings.Set(ctx, settingsKey, next)
	}
	if err == nil {
		m.cfg = next
		err = m.recomputeLocked(ctx)
	}
	m.mu.Unlock()
	m.d.Audit.Record(ctx, "me.settings", "", map[string]any{"workHosts": len(next.WorkHosts), "guessSleep": next.GuessSleep, "haHome": next.HAHome, "haSleep": next.HASleep}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if old.HAHome != next.HAHome || old.HASleep != next.HASleep {
		go m.watchHA(context.WithoutCancel(ctx), next)
	}
	m.GetMeSettings(w, r)
}

// ---------- the state and the manual setting ----------

// GetMe implements api.ServerInterface.
func (m *Module) GetMe(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, m.fresh(r.Context()))
}

// fresh decides again and returns the state.
func (m *Module) fresh(ctx context.Context) api.MeState {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.recomputeLocked(ctx); err != nil {
		m.d.Log.Warn("me: work out the state", "err", err)
	}
	return m.currentLocked()
}

// currentLocked is the state; before the first decision it is unknown.
func (m *Module) currentLocked() api.MeState {
	if m.view.State == "" {
		return api.MeState{State: api.MeStateName(contracts.MeUnknown), Since: m.nowLocked()}
	}
	return m.view
}

// SetMeManual implements api.ServerInterface.
func (m *Module) SetMeManual(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.SetMeManualJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	state := string(body.State)
	if !validState(state) || !body.Duration.Valid() {
		httpx.Fail(w, r, httpx.Invalid("状态或时长不对"))
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.ensureLoaded(ctx)
	var set manualSetting
	if err == nil {
		now := m.nowLocked()
		set.State = state
		switch body.Duration {
		case api.N1h:
			set.Until = now.Add(time.Hour)
		case api.N2h:
			set.Until = now.Add(2 * time.Hour)
		case api.Wake:
			if s := m.loadSchedule(ctx); s != nil {
				set.Until = nextWake(now, *s)
			} else {
				set.Until = nextWake(now, schedule{Wake: 7 * 60, Loc: m.d.Config.Location})
			}
		}
		err = m.d.Settings.Set(ctx, manualKey, set)
	}
	if err == nil {
		m.manual = &set
		// "I am up" also ends a sleep the phone reported; otherwise the
		// state would fall back to sleeping when the setting runs out.
		if state != contracts.MeSleeping {
			err = m.endReportedSleep(ctx, srcManual)
		}
	}
	if err == nil {
		err = m.recomputeLocked(ctx)
	}
	m.d.Audit.Record(ctx, "me.manual", state, map[string]any{"duration": string(body.Duration)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.currentLocked())
}

// endReportedSleep adds a "sleep ended" report when the latest sleep report
// says the sleep began.
func (m *Module) endReportedSleep(ctx context.Context, source string) error {
	sleep, err := m.latestSleep(ctx)
	if err != nil || sleep.Event != evSleepStart {
		return err
	}
	now := m.nowLocked()
	_, err = m.insertReport(ctx, evSleepEnd, source, now, now)
	return err
}

// ClearMeManual implements api.ServerInterface.
func (m *Module) ClearMeManual(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.ensureLoaded(ctx)
	if err == nil {
		err = m.d.Settings.Delete(ctx, manualKey)
	}
	if err == nil {
		m.manual = nil
		err = m.recomputeLocked(ctx)
	}
	m.d.Audit.Record(ctx, "me.manual_clear", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m.currentLocked())
}
