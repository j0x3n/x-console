package reminders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
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

// pushPayload is what web/public/sw.js receives.
type pushPayload struct {
	ID       int64        `json:"id"`
	Kind     string       `json:"kind"`
	Title    string       `json:"title"`
	Body     string       `json:"body,omitempty"`
	Link     string       `json:"link,omitempty"`
	Priority string       `json:"priority"`
	Actions  []pushAction `json:"actions,omitempty"`
}

type pushAction struct {
	Action string `json:"action"`
	Title  string `json:"title"`
}

func (c *webPushChannel) Send(ctx context.Context, n notify.Stored) error {
	keys, err := c.keys(ctx)
	if err != nil {
		return err
	}
	subs, err := c.m.q.ListPushSubscriptions(ctx)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return httpx.ErrIntegrationMissing
	}
	p := pushPayload{ID: n.ID, Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link, Priority: n.Priority}
	for _, a := range n.Actions {
		p.Actions = append(p.Actions, pushAction{Action: a.ID, Title: a.Label})
	}
	payload, _ := json.Marshal(p)
	subject, _ := c.m.value(ctx, "webpush", "subject")
	subject = strings.TrimPrefix(subject, "mailto:")
	if subject == "" {
		subject = "x-console@example.com"
		if strings.HasPrefix(c.m.d.Config.PublicURL, "https://") {
			subject = c.m.d.Config.PublicURL
		}
	}
	urgency := webpush.UrgencyNormal
	if n.Priority == notify.PriorityUrgent || n.Priority == notify.PriorityHigh {
		urgency = webpush.UrgencyHigh
	}
	var errs []error
	for _, s := range subs {
		errs = append(errs, c.sendOne(ctx, s, payload, keys, subject, urgency))
	}
	return errors.Join(errs...)
}

func (c *webPushChannel) sendOne(ctx context.Context, s db.WebpushSubscription, payload []byte, keys vapidKeys, subject string, urgency webpush.Urgency) error {
	resp, err := webpush.SendNotificationWithContext(ctx, payload,
		&webpush.Subscription{Endpoint: s.Endpoint, Keys: webpush.Keys{P256dh: s.P256dh, Auth: s.Auth}},
		&webpush.Options{
			HTTPClient: c.m.http, Subscriber: subject, TTL: 12 * 3600, Urgency: urgency,
			VAPIDPublicKey: keys.Public, VAPIDPrivateKey: keys.Private,
		})
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("webpush: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The browser dropped the subscription.
		if _, err := c.m.q.DeletePushSubscription(context.WithoutCancel(ctx), s.Endpoint); err != nil {
			return err
		}
		c.m.d.Bus.Publish("notify.subscription_removed", map[string]any{"endpoint": s.Endpoint})
		return nil
	case resp.StatusCode >= 300:
		return fmt.Errorf("webpush: %s", resp.Status)
	}
	return nil
}
