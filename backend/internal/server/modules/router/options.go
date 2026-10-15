package router

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B228, B230, B232: settings keys.
const (
	keyDeviceNotify    = "router.device_notify"     // master switch of the online and offline notifications
	keyNewDeviceNotify = "router.notify_new_device" // B232
	keyNotifyCooldown  = "router.notify_cooldown"   // default minutes between two notifications of one device
	keyLogDays         = "router.log_days"          // B230
	keyDevicesSeeded   = "router.devices_seeded"    // B232: the first report has been taken in
)

// cooldownChoices are the minutes a device may wait before it is announced again.
var cooldownChoices = []int{0, 5, 15, 30, 60}

const (
	defaultCooldown = 15
	defaultLogDays  = 7
	maxLogDays      = 15
)

// options are the settings of the device notifications and the logs.
type options struct {
	DeviceNotify    bool
	NewDeviceNotify bool
	Cooldown        int
	LogDays         int
}

func defaultOptions() options {
	return options{NewDeviceNotify: true, Cooldown: defaultCooldown, LogDays: defaultLogDays}
}

// options returns the settings. They are read at every report, so they are
// kept until a change.
func (m *Module) options(ctx context.Context) (options, error) {
	m.mu.Lock()
	cached := m.opts
	m.mu.Unlock()
	if cached != nil {
		return *cached, nil
	}
	o := defaultOptions()
	read := func(key string, dst any) error {
		if err := m.d.Settings.Get(ctx, key, dst); err != nil && !errors.Is(err, settings.ErrNotSet) {
			return err
		}
		return nil
	}
	if err := read(keyDeviceNotify, &o.DeviceNotify); err != nil {
		return o, err
	}
	if err := read(keyNewDeviceNotify, &o.NewDeviceNotify); err != nil {
		return o, err
	}
	if err := read(keyNotifyCooldown, &o.Cooldown); err != nil {
		return o, err
	}
	if err := read(keyLogDays, &o.LogDays); err != nil {
		return o, err
	}
	if !slices.Contains(cooldownChoices, o.Cooldown) {
		o.Cooldown = defaultCooldown
	}
	if o.LogDays < 1 || o.LogDays > maxLogDays {
		o.LogDays = defaultLogDays
	}
	m.mu.Lock()
	m.opts = &o
	m.mu.Unlock()
	return o, nil
}

func (o options) toAPI() api.RouterOptions {
	return api.RouterOptions{DeviceNotify: o.DeviceNotify, NewDeviceNotify: o.NewDeviceNotify, NotifyCooldownMinutes: o.Cooldown, LogDays: o.LogDays}
}

// GetRouterOptions returns the settings.
func (m *Module) GetRouterOptions(w http.ResponseWriter, r *http.Request) {
	o, err := m.options(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, o.toAPI())
}

// PutRouterOptions saves all four settings at once.
func (m *Module) PutRouterOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in api.RouterOptions
	if err := decodeAll(w, r, &in, "deviceNotify", "newDeviceNotify", "notifyCooldownMinutes", "logDays"); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !slices.Contains(cooldownChoices, in.NotifyCooldownMinutes) {
		httpx.Fail(w, r, httpx.Invalid("再次通知间隔只能是 0、5、15、30、60 分钟"))
		return
	}
	if in.LogDays < 1 || in.LogDays > maxLogDays {
		httpx.Fail(w, r, httpx.Invalid("日志保存天数要在 1 到 15 之间"))
		return
	}
	old, err := m.options(ctx)
	if err == nil {
		for key, v := range map[string]any{keyDeviceNotify: in.DeviceNotify, keyNewDeviceNotify: in.NewDeviceNotify, keyNotifyCooldown: in.NotifyCooldownMinutes, keyLogDays: in.LogDays} {
			if err = m.d.Settings.Set(ctx, key, v); err != nil {
				break
			}
		}
	}
	m.d.Audit.Record(ctx, "router.options", "", map[string]any{"deviceNotify": in.DeviceNotify, "newDeviceNotify": in.NewDeviceNotify,
		"notifyCooldownMinutes": in.NotifyCooldownMinutes, "logDays": in.LogDays}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	m.opts = nil
	m.mu.Unlock()
	if in.DeviceNotify && !old.DeviceNotify {
		// What changed while the switch was off is old news.
		m.forgetNotified(ctx)
	}
	httpx.JSON(w, http.StatusOK, in)
}
