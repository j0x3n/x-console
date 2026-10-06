package reminders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// publicLink turns an in-app path into an absolute URL when XC_PUBLIC_URL is
// set. Absolute links pass through; without a public URL it returns "".
func (m *Module) publicLink(link string) string {
	switch {
	case strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://"):
		return link
	case m.d.Config.PublicURL == "":
		return ""
	case link == "":
		return m.d.Config.PublicURL + "/"
	case strings.HasPrefix(link, "/"):
		return m.d.Config.PublicURL + link
	default:
		return m.d.Config.PublicURL + "/" + link
	}
}

// postJSON sends body and decodes a JSON reply into out. Transport errors are
// stripped of the URL so keys in paths never reach logs.
func (m *Module) post(ctx context.Context, name, endpoint, contentType string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: bad address", name)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := m.http.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 && len(raw) == 0 {
		return fmt.Errorf("%s: %s", name, resp.Status)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%s: %s", name, resp.Status)
		}
	}
	return nil
}

// ---- Bark ----

type barkChannel struct{ m *Module }

func (c *barkChannel) Name() string { return "bark" }

func (c *barkChannel) Configured(ctx context.Context) bool { return c.m.requiredSet(ctx, "bark") }

func (c *barkChannel) Send(ctx context.Context, n notify.Stored) error {
	cfg, err := c.m.values(ctx, "bark")
	if err != nil {
		return err
	}
	if cfg["device_key"] == "" {
		return httpx.ErrIntegrationMissing
	}
	msg := map[string]any{
		"device_key": cfg["device_key"],
		"title":      n.Title,
		"body":       n.Body,
		"group":      "X Console",
	}
	if msg["body"] == "" {
		msg["body"] = n.Title
	}
	if link := c.m.publicLink(n.Link); link != "" {
		msg["url"] = link
	}
	if n.Priority == notify.PriorityUrgent || n.Priority == notify.PriorityHigh {
		msg["level"] = "timeSensitive"
	}
	raw, _ := json.Marshal(msg)
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := c.m.post(ctx, "bark", strings.TrimRight(cfg["server"], "/")+"/push", "application/json", raw, &out); err != nil {
		return err
	}
	if out.Code != http.StatusOK {
		return fmt.Errorf("bark: %d %s", out.Code, out.Message)
	}
	return nil
}

// ---- ServerChan ----

type serverChanChannel struct{ m *Module }

func (c *serverChanChannel) Name() string { return "serverchan" }

func (c *serverChanChannel) Configured(ctx context.Context) bool {
	return c.m.requiredSet(ctx, "serverchan")
}

var sctpKey = regexp.MustCompile(`^sctp(\d+)t`)

// serverChanURL picks the API address. Server酱³ keys ("sctp<uid>t...") use
// their own host; older SendKeys use sctapi.ftqq.com.
func serverChanURL(base, key string) string {
	if base != "" {
		return strings.TrimRight(base, "/") + "/" + url.PathEscape(key) + ".send"
	}
	if m := sctpKey.FindStringSubmatch(key); m != nil {
		return "https://" + m[1] + ".push.ft07.com/send/" + url.PathEscape(key) + ".send"
	}
	return "https://sctapi.ftqq.com/" + url.PathEscape(key) + ".send"
}

func (c *serverChanChannel) Send(ctx context.Context, n notify.Stored) error {
	cfg, err := c.m.values(ctx, "serverchan")
	if err != nil {
		return err
	}
	if cfg["send_key"] == "" {
		return httpx.ErrIntegrationMissing
	}
	desp := n.Body
	if link := c.m.publicLink(n.Link); link != "" {
		desp += "\n\n[打开](" + link + ")"
	}
	form := url.Values{"title": {n.Title}, "desp": {desp}}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	endpoint := serverChanURL(cfg["api_base"], cfg["send_key"])
	if err := c.m.post(ctx, "serverchan", endpoint, "application/x-www-form-urlencoded", []byte(form.Encode()), &out); err != nil {
		return err
	}
	if out.Code != 0 {
		return fmt.Errorf("serverchan: %d %s", out.Code, out.Message)
	}
	return nil
}

// ---- Web Push ----

const vapidKey = "webpush.vapid"

type vapidKeys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

type webPushChannel struct {
	m  *Module
	mu sync.Mutex // guards key generation
}

func (c *webPushChannel) Name() string { return "webpush" }

func (c *webPushChannel) Configured(ctx context.Context) bool {
	n, err := c.m.q.CountPushSubscriptions(ctx)
	return err == nil && n > 0
}

// keys returns the VAPID key pair, generating and storing it on first use.
func (c *webPushChannel) keys(ctx context.Context) (vapidKeys, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var k vapidKeys
	err := c.m.d.Settings.Get(ctx, vapidKey, &k)
	if err == nil && k.Public != "" && k.Private != "" {
		return k, nil
	}
	if err != nil && !errors.Is(err, settings.ErrNotSet) {
		return k, err
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return k, err
	}
	k = vapidKeys{Public: pub, Private: priv}
	return k, c.m.d.Settings.SetSecret(ctx, vapidKey, k)
}

const pushTTL = 3 * 24 * 3600

// pushPayload is what web/public/sw.js receives.
type pushPayload struct {
	Icon     string       `json:"icon,omitempty"`
	Badge    string       `json:"badge,omitempty"`
	Image    string       `json:"image,omitempty"`
	ID       int64        `json:"id"`
	Kind     string       `json:"kind"`
	Title    string       `json:"title"`
	Body     string       `json:"body,omitempty"`
	Link     string       `json:"link,omitempty"`
	Priority string       `json:"priority"`
	SentAt   string       `json:"sentAt,omitempty"`
	Actions  []pushAction `json:"actions,omitempty"`
}

func withSentAt(p pushPayload, now time.Time) pushPayload {
	p.SentAt = now.UTC().Format(time.RFC3339)
	return p
}

func testPushBody(now time.Time, loc *time.Location) string {
	return "收到这条说明浏览器推送能用。发送时间 " + now.In(loc).Format("15:04:05")
}

type pushAction struct {
	Action string `json:"action"`
	Title  string `json:"title"`
}

// pushSettings holds what every push of one message shares.
type pushSettings struct {
	keys    vapidKeys
	subject string
	urgency webpush.Urgency
}

func (c *webPushChannel) settings(ctx context.Context, priority string) (pushSettings, error) {
	keys, err := c.keys(ctx)
	if err != nil {
		return pushSettings{}, err
	}
	subject, _ := c.m.value(ctx, "webpush", "subject")
	subject = strings.TrimPrefix(subject, "mailto:")
	if subject == "" {
		subject = "x-console@example.com"
		if strings.HasPrefix(c.m.d.Config.PublicURL, "https://") {
			subject = c.m.d.Config.PublicURL
		}
	}
	urgency := webpush.UrgencyNormal
	if priority == notify.PriorityUrgent || priority == notify.PriorityHigh {
		urgency = webpush.UrgencyHigh
	}
	return pushSettings{keys: keys, subject: subject, urgency: urgency}, nil
}

func (c *webPushChannel) Send(ctx context.Context, n notify.Stored) error {
	subs, err := c.m.q.ListPushSubscriptions(ctx)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return httpx.ErrIntegrationMissing
	}
	set, err := c.settings(ctx, n.Priority)
	if err != nil {
		return err
	}
	p := withSentAt(pushPayload{ID: n.ID, Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link, Priority: n.Priority}, time.Now())
	p.Icon, p.Badge = c.m.pushIcons(ctx, n)
	for _, a := range n.Actions {
		p.Actions = append(p.Actions, pushAction{Action: a.ID, Title: a.Label})
	}
	payload, _ := json.Marshal(p)
	var errs []error
	mutes := c.m.loadMutes(ctx)
	for _, s := range subs {
		// B113: a mute rule can keep this kind from one device.
		if muted(mutes, n, devicePrefix+strconv.FormatInt(s.ID, 10)) {
			continue
		}
		// A subscription the browser dropped is cleaned up, not a failure.
		if out := c.sendOne(ctx, s, payload, set); out.err != "" && !out.removed {
			errs = append(errs, errors.New("webpush: "+out.err))
		}
	}
	return errors.Join(errs...)
}

// pushOutcome is the result of one push to one browser.
type pushOutcome struct {
	status  int    // HTTP status of the push service, 0 when it could not be reached
	err     string // short reason in Chinese, empty on success
	removed bool   // the push service said the subscription is gone
}

// maxPushError is the longest error kept per subscription.
const maxPushError = 200

// sendOne pushes payload to one browser and records the result on the
// subscription: last_ok_at on 2xx, last_error otherwise. The write ignores
// cancellation so a push that went out is recorded even if the request ended.
func (c *webPushChannel) sendOne(ctx context.Context, s db.WebpushSubscription, payload []byte, set pushSettings) pushOutcome {
	// webpush-go appends to the message buffer, so parallel pushes need a copy each.
	resp, err := webpush.SendNotificationWithContext(ctx, slices.Clone(payload),
		&webpush.Subscription{Endpoint: s.Endpoint, Keys: webpush.Keys{P256dh: s.P256dh, Auth: s.Auth}},
		&webpush.Options{
			HTTPClient: c.m.http, Subscriber: set.subject, TTL: pushTTL, Urgency: set.urgency,
			VAPIDPublicKey: set.keys.Public, VAPIDPrivateKey: set.keys.Private,
		})
	var out pushOutcome
	if err != nil {
		out.err = pushErrorText(err, s.Endpoint)
	} else {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		out.status = resp.StatusCode
		switch {
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			out.removed = true
			out.err = resp.Status
		case resp.StatusCode >= 300:
			out.err = resp.Status
		}
	}
	c.record(ctx, s, out)
	return out
}

func (c *webPushChannel) record(ctx context.Context, s db.WebpushSubscription, out pushOutcome) {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	var err error
	switch {
	case out.removed:
		// The browser dropped the subscription.
		if _, err = c.m.q.DeletePushSubscription(ctx, s.Endpoint); err == nil {
			c.m.pruneMutes(ctx)
			c.m.d.Bus.Publish("notify.subscription_removed", map[string]any{"endpoint": s.Endpoint})
			c.m.d.Bus.Publish("notify.channel_updated", map[string]string{"name": "webpush"})
		}
	case out.err == "":
		err = c.m.q.MarkPushOK(ctx, db.MarkPushOKParams{ID: s.ID, LastOkAt: &now})
	default:
		msg := truncateRunes(out.err, maxPushError)
		err = c.m.q.MarkPushError(ctx, db.MarkPushErrorParams{ID: s.ID, LastError: &msg, LastErrorAt: &now})
	}
	if err != nil {
		c.m.d.Log.Warn("record webpush result", "subscription", s.ID, "err", err)
	}
}

func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// pushErrorText turns a transport error into a short Chinese sentence naming
// the push service host. Other errors keep their original text.
func pushErrorText(err error, endpoint string) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	host := endpoint
	if u, perr := url.Parse(endpoint); perr == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	var dns *net.DNSError
	var nerr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &nerr) && nerr.Timeout()):
		return "连接 " + host + " 超时"
	case errors.As(err, &dns):
		return "解析不了 " + host
	case errors.Is(err, syscall.ECONNREFUSED):
		return host + " 拒绝连接"
	}
	return err.Error()
}

// pushService tells which browser vendor's push service an endpoint belongs
// to. The rules match web/src/features/reminders/push.ts.
func pushService(endpoint string) api.WebPushService {
	u, err := url.Parse(endpoint)
	if err != nil {
		return api.WebPushServiceOther
	}
	host := u.Hostname()
	switch {
	case host == "fcm.googleapis.com" || strings.HasSuffix(host, ".googleapis.com"):
		return api.WebPushServiceGoogle
	case strings.HasSuffix(host, ".notify.windows.com"):
		return api.WebPushServiceMicrosoft
	case strings.HasSuffix(host, ".mozilla.com") || strings.HasSuffix(host, ".mozaws.net"):
		return api.WebPushServiceMozilla
	case strings.HasSuffix(host, ".push.apple.com"):
		return api.WebPushServiceApple
	}
	return api.WebPushServiceOther
}
