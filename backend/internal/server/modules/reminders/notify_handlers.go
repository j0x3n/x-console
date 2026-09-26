package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

func (m *Module) ListNotifyChannels(w http.ResponseWriter, r *http.Request) {
	out := make([]api.NotifyChannel, 0, len(channelOrder))
	for _, name := range channelOrder {
		c, err := m.channelAPI(r.Context(), name)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		out = append(out, c)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) GetNotifyChannel(w http.ResponseWriter, r *http.Request, channel api.Channel) {
	c, err := m.channelAPI(r.Context(), string(channel))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

func (m *Module) UpdateNotifyChannel(w http.ResponseWriter, r *http.Request, channel api.Channel) {
	var body api.UpdateNotifyChannelJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := string(channel)
	if changesSecret(name, body.Values) {
		if err := auth.RequireElevated(r.Context()); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	keys := make([]string, 0, len(body.Values))
	for k := range body.Values {
		keys = append(keys, k)
	}
	err := m.updateChannel(r.Context(), name, body.Values)
	m.d.Audit.Record(r.Context(), "notify.channel.update", name, map[string]any{"fields": keys}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	c, err := m.channelAPI(r.Context(), name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.channel_updated", map[string]string{"name": name})
	httpx.JSON(w, http.StatusOK, c)
}

// sendTest delivers a test message straight to one channel, bypassing
// routes and quiet hours.
func (m *Module) sendTest(ctx context.Context, name string) error {
	c, ok := m.d.Notify.Channel(name)
	if !ok {
		return httpx.ErrNotFound
	}
	if cc, ok := c.(configurable); ok && !cc.Configured(ctx) {
		return httpx.ErrIntegrationMissing
	}
	err := c.Send(ctx, notify.Stored{CreatedAt: time.Now().UTC(), Notification: notify.Notification{
		Kind: "notify.test", Title: "X Console 测试通知", Body: "收到这条消息，说明渠道配置正确。",
		Link: "/settings/notifications", Priority: notify.PriorityNormal, Source: "reminders",
	}})
	if err != nil {
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) {
			return err
		}
		return httpx.NewError(http.StatusBadGateway, "delivery_failed", "发送失败："+err.Error())
	}
	return nil
}

func (m *Module) TestNotifyChannel(w http.ResponseWriter, r *http.Request, channel api.Channel) {
	err := m.sendTest(r.Context(), string(channel))
	m.d.Audit.Record(r.Context(), "notify.channel.test", string(channel), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---- routes ----

func routeToAPI(rt route) api.NotifyRoute {
	id := rt.ID
	return api.NotifyRoute{Id: &id, KindPattern: rt.KindPattern, MinPriority: api.NotifyPriority(rt.MinPriority),
		Channels: rt.Channels, Enabled: rt.Enabled}
}

func (m *Module) listRoutes(ctx context.Context) ([]api.NotifyRoute, error) {
	rows, err := m.q.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.NotifyRoute, 0, len(rows))
	for _, row := range rows {
		out = append(out, routeToAPI(routeFromDB(row)))
	}
	return out, nil
}

func (m *Module) replaceRoutes(ctx context.Context, in []api.NotifyRoute) error {
	for _, rt := range in {
		if strings.TrimSpace(rt.KindPattern) == "" {
			return httpx.Invalid("匹配规则不能为空")
		}
		if !rt.MinPriority.Valid() {
			return httpx.Invalid("优先级无效: " + string(rt.MinPriority))
		}
		for _, c := range rt.Channels {
			if _, ok := channelFields[c]; !ok {
				return httpx.Invalid("没有这个渠道: " + c)
			}
		}
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := m.q.WithTx(tx)
	if err := q.DeleteRoutes(ctx); err != nil {
		return err
	}
	for i, rt := range in {
		channels := rt.Channels
		if channels == nil {
			channels = []string{}
		}
		raw, _ := json.Marshal(channels)
		var enabled int64
		if rt.Enabled {
			enabled = 1
		}
		if _, err := q.InsertRoute(ctx, db.InsertRouteParams{
			KindPattern: strings.TrimSpace(rt.KindPattern), MinPriority: string(rt.MinPriority),
			Channels: string(raw), Enabled: enabled, SortOrder: int64(i),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (m *Module) ListNotifyRoutes(w http.ResponseWriter, r *http.Request) {
	out, err := m.listRoutes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ReplaceNotifyRoutes(w http.ResponseWriter, r *http.Request) {
	var body api.ReplaceNotifyRoutesJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.replaceRoutes(r.Context(), body)
	m.d.Audit.Record(r.Context(), "notify.routes.update", "", map[string]any{"count": len(body)}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out, err := m.listRoutes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---- quiet hours ----

func (m *Module) GetQuietHours(w http.ResponseWriter, r *http.Request) {
	q := m.quietHours(r.Context())
	httpx.JSON(w, http.StatusOK, api.QuietHours{Enabled: q.Enabled, Start: q.Start, End: q.End})
}

func (m *Module) UpdateQuietHours(w http.ResponseWriter, r *http.Request) {
	var body api.UpdateQuietHoursJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, ok1 := parseClock(body.Start)
	_, ok2 := parseClock(body.End)
	if !ok1 || !ok2 {
		httpx.Fail(w, r, httpx.Invalid("时间格式要写成 23:00 这样"))
		return
	}
	q := quietHours{Enabled: body.Enabled, Start: body.Start, End: body.End}
	err := m.d.Settings.Set(r.Context(), quietHoursKey, q)
	m.d.Audit.Record(r.Context(), "notify.quiet_hours.update", "", map[string]any{"enabled": q.Enabled, "start": q.Start, "end": q.End}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, body)
}

// ---- actions from notification buttons ----

func (m *Module) RunNotifyAction(w http.ResponseWriter, r *http.Request) {
	var body api.RunNotifyActionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// The actor tells handlers where the press came from (habits records it
	// as the check-in source).
	ctx := audit.WithActor(r.Context(), "webpush:"+audit.Actor(r.Context()))
	err := m.d.Notify.HandleAction(ctx, body.ActionId)
	m.d.Audit.Record(ctx, "notify.action", body.ActionId, map[string]any{"channel": "webpush"}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// ---- Web Push subscriptions ----

func (m *Module) GetVapidPublicKey(w http.ResponseWriter, r *http.Request) {
	k, err := m.push.keys(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"publicKey": k.Public})
}

func (m *Module) AddPushSubscription(w http.ResponseWriter, r *http.Request) {
	var body api.AddPushSubscriptionJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !strings.HasPrefix(body.Endpoint, "https://") && !strings.HasPrefix(body.Endpoint, "http://") ||
		body.Keys.P256dh == "" || body.Keys.Auth == "" {
		httpx.Fail(w, r, httpx.Invalid("订阅信息不完整"))
		return
	}
	ua := ""
	if body.UserAgent != nil {
		ua = *body.UserAgent
	}
	if ua == "" {
		ua = r.UserAgent()
	}
	if len(ua) > 300 {
		ua = ua[:300]
	}
	err := m.q.UpsertPushSubscription(r.Context(), db.UpsertPushSubscriptionParams{
		Endpoint: body.Endpoint, P256dh: body.Keys.P256dh, Auth: body.Keys.Auth, UserAgent: ua, CreatedAt: time.Now().UTC(),
	})
	m.d.Audit.Record(r.Context(), "notify.webpush.subscribe", ua, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.channel_updated", map[string]string{"name": "webpush"})
	httpx.NoContent(w)
}

func (m *Module) DeletePushSubscription(w http.ResponseWriter, r *http.Request, params api.DeletePushSubscriptionParams) {
	n, err := m.q.DeletePushSubscription(r.Context(), params.Endpoint)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(r.Context(), "notify.webpush.unsubscribe", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.channel_updated", map[string]string{"name": "webpush"})
	httpx.NoContent(w)
}

// ---- Telegram ----

func (m *Module) RegisterTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	hook, err := m.registerTelegramWebhook(r.Context())
	m.d.Audit.Record(r.Context(), "telegram.register_webhook", hook, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"url": hook})
}

func (m *Module) TelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if err := m.handleTelegramUpdate(r.Context(), r.Header.Get("X-Telegram-Bot-Api-Secret-Token"), r.Body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
