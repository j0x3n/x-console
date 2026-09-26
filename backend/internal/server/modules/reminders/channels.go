package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// fieldSpec describes one configuration value of a channel. It is stored in
// settings under "<channel>.<key>"; secrets are encrypted.
type fieldSpec struct {
	Key         string
	Secret      bool
	Required    bool
	Default     string
	Placeholder string
}

// channelFields lists the editable configuration of every channel, in the
// order the settings page shows them. Base URLs are editable so self-hosted
// servers (and tests) can replace the public services.
var channelFields = map[string][]fieldSpec{
	"webpush": {
		{Key: "subject", Placeholder: "mailto:you@example.com"},
	},
	"telegram": {
		{Key: "bot_token", Secret: true, Required: true, Placeholder: "123456789:AA..."},
		{Key: "chat_id", Required: true, Placeholder: "123456789"},
		{Key: "api_base", Default: "https://api.telegram.org", Placeholder: "https://api.telegram.org"},
	},
	"bark": {
		{Key: "device_key", Secret: true, Required: true},
		{Key: "server", Default: "https://api.day.app", Placeholder: "https://api.day.app"},
	},
	"serverchan": {
		{Key: "send_key", Secret: true, Required: true, Placeholder: "SCT..."},
		{Key: "api_base", Placeholder: "https://sctapi.ftqq.com"},
	},
}

// channelOrder is the display order.
var channelOrder = []string{"webpush", "telegram", "bark", "serverchan"}

// configurable is implemented by every channel of this module so the router
// can skip channels that are not set up yet.
type configurable interface {
	Configured(ctx context.Context) bool
}

func settingKey(channel, key string) string { return channel + "." + key }

// value reads one channel setting, falling back to its default.
func (m *Module) value(ctx context.Context, channel, key string) (string, error) {
	var v string
	err := m.d.Settings.Get(ctx, settingKey(channel, key), &v)
	if errors.Is(err, settings.ErrNotSet) || (err == nil && v == "") {
		for _, f := range channelFields[channel] {
			if f.Key == key {
				return f.Default, nil
			}
		}
		return "", nil
	}
	return v, err
}

// values reads all fields of a channel.
func (m *Module) values(ctx context.Context, channel string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range channelFields[channel] {
		v, err := m.value(ctx, channel, f.Key)
		if err != nil {
			return nil, err
		}
		out[f.Key] = v
	}
	return out, nil
}

// requiredSet reports whether every required field has a value.
func (m *Module) requiredSet(ctx context.Context, channel string) bool {
	for _, f := range channelFields[channel] {
		if !f.Required {
			continue
		}
		if v, err := m.value(ctx, channel, f.Key); err != nil || v == "" {
			return false
		}
	}
	return true
}

// mask hides a secret, keeping the last 4 characters of long values.
func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return "••••" + s[len(s)-4:]
}

func (m *Module) channelAPI(ctx context.Context, name string) (api.NotifyChannel, error) {
	specs, ok := channelFields[name]
	if !ok {
		return api.NotifyChannel{}, httpx.ErrNotFound
	}
	out := api.NotifyChannel{Name: name, Fields: []api.ChannelField{}}
	for _, f := range specs {
		var raw string
		err := m.d.Settings.Get(ctx, settingKey(name, f.Key), &raw)
		if err != nil && !errors.Is(err, settings.ErrNotSet) {
			return out, err
		}
		field := api.ChannelField{Key: f.Key, Secret: f.Secret, Required: f.Required, Set: raw != "", Value: raw}
		if f.Secret {
			field.Value = mask(raw)
		}
		if f.Placeholder != "" {
			p := f.Placeholder
			field.Placeholder = &p
		}
		out.Fields = append(out.Fields, field)
	}
	if c, ok := m.d.Notify.Channel(name); ok {
		if cc, ok := c.(configurable); ok {
			out.Configured = cc.Configured(ctx)
		}
	}
	switch name {
	case "webpush":
		n, err := m.q.CountPushSubscriptions(ctx)
		if err != nil {
			return out, err
		}
		count := int(n)
		out.Subscriptions = &count
	case "telegram":
		if u := m.telegramWebhookURL(); u != "" {
			out.WebhookUrl = &u
		}
	}
	return out, nil
}

// changesSecret reports whether an update touches a secret field. Changing
// third-party tokens needs a fresh TOTP elevation.
func changesSecret(name string, values map[string]string) bool {
	for _, f := range channelFields[name] {
		if _, ok := values[f.Key]; ok && f.Secret {
			return true
		}
	}
	return false
}

// updateChannel stores the given fields. Unknown keys are rejected. An empty
// value deletes the setting.
func (m *Module) updateChannel(ctx context.Context, name string, values map[string]string) error {
	specs, ok := channelFields[name]
	if !ok {
		return httpx.ErrNotFound
	}
	known := map[string]fieldSpec{}
	for _, f := range specs {
		known[f.Key] = f
	}
	for k, v := range values {
		f, ok := known[k]
		if !ok {
			return httpx.Invalid("未知的配置项: " + k)
		}
		v = strings.TrimSpace(v)
		if v != "" && (f.Key == "api_base" || f.Key == "server") {
			if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return httpx.Invalid("地址格式不对，要以 http:// 或 https:// 开头")
			}
			v = strings.TrimRight(v, "/")
		}
		values[k] = v
	}
	for k, v := range values {
		key := settingKey(name, k)
		var err error
		switch {
		case v == "":
			err = m.d.Settings.Delete(ctx, key)
		case known[k].Secret:
			err = m.d.Settings.SetSecret(ctx, key, v)
		default:
			err = m.d.Settings.Set(ctx, key, v)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// ---- routing ----

var priorityRank = map[string]int{
	notify.PriorityLow: 0, notify.PriorityNormal: 1, notify.PriorityHigh: 2, notify.PriorityUrgent: 3,
}

// route is a parsed notification_routes row.
type route struct {
	ID          int64
	KindPattern string
	MinPriority string
	Channels    []string
	Enabled     bool
}

func routeFromDB(r db.NotificationRoute) route {
	out := route{ID: r.ID, KindPattern: r.KindPattern, MinPriority: r.MinPriority, Enabled: r.Enabled == 1, Channels: []string{}}
	_ = json.Unmarshal([]byte(r.Channels), &out.Channels)
	return out
}

// matchKind matches a kind against a pattern where * means any characters.
func matchKind(pattern, kind string) bool {
	ok, err := path.Match(pattern, kind)
	return err == nil && ok
}

// pickChannels is the routing rule: quiet hours drop everything but urgent
// notifications, then the first enabled route whose pattern and minimum
// priority match decides the channels.
func pickChannels(routes []route, kind, priority string, quiet bool) []string {
	if priority == "" {
		priority = notify.PriorityNormal
	}
	if quiet && priority != notify.PriorityUrgent {
		return nil
	}
	for _, r := range routes {
		if !r.Enabled || !matchKind(r.KindPattern, kind) {
			continue
		}
		if priorityRank[priority] < priorityRank[r.MinPriority] {
			continue
		}
		return r.Channels
	}
	return nil
}

// router implements notify.Router.
type router struct{ m *Module }

func (r *router) Route(ctx context.Context, n notify.Stored) []string {
	m := r.m
	rows, err := m.q.ListRoutes(ctx)
	if err != nil {
		m.d.Log.Warn("load notification routes", "err", err)
		return nil
	}
	routes := make([]route, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, routeFromDB(row))
	}
	quiet := inQuietHours(m.quietHours(ctx), time.Now(), m.d.Config.Location)
	var out []string
	for _, name := range pickChannels(routes, n.Kind, n.Priority, quiet) {
		c, ok := m.d.Notify.Channel(name)
		if !ok {
			continue
		}
		if cc, ok := c.(configurable); ok && !cc.Configured(ctx) {
			continue
		}
		out = append(out, name)
	}
	return out
}

const quietHoursKey = "notify.quiet_hours"

func (m *Module) quietHours(ctx context.Context) quietHours {
	q := quietHours{Start: "23:00", End: "07:30"}
	if err := m.d.Settings.Get(ctx, quietHoursKey, &q); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load quiet hours", "err", err)
	}
	return q
}
