package reminders

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// B127: notification digest. Notifications that are not urgent wait in a
// queue and go out as one message at a few fixed times a day. The in-app
// notification center is not affected, only the external channels are.

const (
	digestKey      = "notify.digest"
	digestStateKey = "notify.digest_state"

	digestMaxQueue   = 1000             // oldest rows beyond this are dropped
	digestStale      = 36 * time.Hour   // older items are marked in the message
	digestRetryAfter = 10 * time.Minute // after every channel failed
	digestMaxItems   = 25               // listed in one message
	digestGroupItems = 5                // listed per category
	digestBodyRunes  = 3000             // Telegram allows 4096 including the title

	modeNow    = "now"
	modeDigest = "digest"
)

var defaultDigestTimes = []string{"11:00", "15:00", "19:00"}

// digestCategory groups kinds for the settings. The first category whose
// pattern matches decides. The last one catches everything else.
type digestCategory struct {
	ID       string
	Label    string // used in the digest message
	Patterns []string
	Default  string
}

var digestCategories = []digestCategory{
	{"alert", "服务器和监控告警", []string{"host.*", "monitor.*", "router.*", "backup.*"}, modeNow},
	{"security", "安全", []string{"security.*", "auth.*", "login.*"}, modeNow},
	{"agent_decision", "Agent 等你决定", []string{"ai_agent.decision"}, modeNow},
	{"reminder", "提醒和习惯", []string{"reminder.*", "habit.*", "workout.*", "focus.*"}, modeNow},
	{"brief", "早报和天气", []string{"brief.*", "weather.*"}, modeNow},
	{"agent_progress", "Agent 和编码任务", []string{"ai_agent.*", "coding_task.*"}, modeDigest},
	{"mail", "邮件", []string{"mail.*"}, modeDigest},
	{"github", "GitHub", []string{"github.*", "repo.*"}, modeDigest},
	{"project", "项目", []string{"issue.*", "subscription.*"}, modeDigest},
	{"other", "其他", []string{"*"}, modeDigest},
}

const digestFallback = "other"

// errDigestBackoff means every channel failed a moment ago. The scheduler
// tries again after digestRetryAfter.
var errDigestBackoff = errors.New("digest: waiting before the next try")

func digestCategoryOf(kind string) string {
	for _, c := range digestCategories {
		for _, p := range c.Patterns {
			if ok, err := path.Match(p, kind); err == nil && ok {
				return c.ID
			}
		}
	}
	return digestFallback
}

func digestCategoryByID(id string) (digestCategory, bool) {
	for _, c := range digestCategories {
		if c.ID == id {
			return c, true
		}
	}
	return digestCategory{}, false
}

// digestConfig is the stored value of notify.digest.
type digestConfig struct {
	Enabled bool              `json:"enabled"`
	Times   []string          `json:"times"`
	Modes   map[string]string `json:"modes"`
}

// mode is the chosen mode of a category, or its default.
func (c digestConfig) mode(id string) string {
	if v := c.Modes[id]; v == modeNow || v == modeDigest {
		return v
	}
	if cat, ok := digestCategoryByID(id); ok {
		return cat.Default
	}
	return modeDigest
}

// digestState is what the scheduler remembers between runs.
type digestState struct {
	Slot   *time.Time `json:"slot,omitempty"`   // the last time point that was handled
	SentAt *time.Time `json:"sentAt,omitempty"` // the last digest that went out
}

func (m *Module) digestSettings(ctx context.Context) digestConfig {
	c := digestConfig{}
	if err := m.d.Settings.Get(ctx, digestKey, &c); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load digest settings", "err", err)
	}
	if len(c.Times) == 0 {
		c.Times = append([]string(nil), defaultDigestTimes...)
	}
	return c
}

func (m *Module) digestState(ctx context.Context) digestState {
	var s digestState
	if err := m.d.Settings.Get(ctx, digestStateKey, &s); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.d.Log.Warn("load digest state", "err", err)
	}
	return s
}

// digestDecision says which category a notification belongs to and whether
// it waits for the digest. Urgent notifications, notifications marked
// Immediate and notifications with buttons are always sent at once: the
// buttons cannot be kept in a merged message.
func digestDecision(cfg digestConfig, n notify.Stored) (category string, hold bool) {
	category = digestCategoryOf(n.Kind)
	switch {
	case !cfg.Enabled, n.Immediate, n.Priority == notify.PriorityUrgent, len(n.Actions) > 0:
		return category, false
	case category == digestFallback && n.Priority == notify.PriorityHigh:
		return category, false
	}
	return category, cfg.mode(category) == modeDigest
}

// digestHolder plugs the digest into notify.Service.
type digestHolder struct{ m *Module }

func (h *digestHolder) Hold(ctx context.Context, n notify.Stored) bool { return h.m.hold(ctx, n) }

// hold queues n when the digest wants it. Any error means "do not hold",
// so a broken queue never swallows a notification.
func (m *Module) hold(ctx context.Context, n notify.Stored) bool {
	cfg := m.digestSettings(ctx)
	category, hold := digestDecision(cfg, n)
	if !hold {
		return false
	}
	if m.mutedEverywhere(ctx, n) {
		return false // B113: nothing would be sent, so there is nothing to queue
	}
	created := n.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	err := m.q.InsertDigestItem(ctx, db.InsertDigestItemParams{
		NotificationID: n.ID, CreatedAt: created.UTC(), Kind: n.Kind, Title: n.Title, Body: n.Body, Link: n.Link,
		Priority: n.Priority, Source: n.Source, Category: category,
	})
	if err != nil {
		m.d.Log.Warn("queue notification for digest", "err", err)
		return false
	}
	if err := m.q.TrimDigestItems(ctx, digestMaxQueue); err != nil {
		m.d.Log.Warn("trim digest queue", "err", err)
	}
	m.d.Bus.Publish("notify.digest_queued", map[string]any{"id": n.ID, "category": category})
	return true
}

// ---- time points ----

// digestSlots turns "HH:MM" strings into minutes after midnight.
func digestSlots(times []string) []int {
	var out []int
	for _, s := range times {
		if v, ok := parseClock(s); ok {
			out = append(out, v)
		}
	}
	sort.Ints(out)
	return out
}

func slotTime(day time.Time, offset, minutes int, loc *time.Location) time.Time {
	l := day.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day()+offset, minutes/60, minutes%60, 0, 0, loc)
}

// latestSlot is the most recent time point at or before now.
func latestSlot(times []string, now time.Time, loc *time.Location) (time.Time, bool) {
	var best time.Time
	for _, off := range []int{-1, 0} {
		for _, s := range digestSlots(times) {
			t := slotTime(now, off, s, loc)
			if !t.After(now) && t.After(best) {
				best = t
			}
		}
	}
	return best, !best.IsZero()
}

// nextSlot is the first time point after now.
func nextSlot(times []string, now time.Time, loc *time.Location) (time.Time, bool) {
	var best time.Time
	for _, off := range []int{0, 1} {
		for _, s := range digestSlots(times) {
			t := slotTime(now, off, s, loc)
			if t.After(now) && (best.IsZero() || t.Before(best)) {
				best = t
			}
		}
	}
	return best, !best.IsZero()
}

func (m *Module) quietNow(ctx context.Context, now time.Time) bool {
	return inQuietHours(m.quietHours(ctx), now, m.d.Config.Location)
}

// digestTick runs every minute. It sends the queue once per time point,
// after quiet hours, and sends what is left when the digest is turned off.
func (m *Module) digestTick(ctx context.Context, now time.Time) error {
	cfg := m.digestSettings(ctx)
	if m.quietNow(ctx, now) {
		return nil
	}
	pending, err := m.q.CountDigestItems(ctx)
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		if pending == 0 {
			return nil
		}
		if _, err := m.flushDigest(ctx, now, false); err != nil && !errors.Is(err, errDigestBackoff) {
			return err
		}
		return nil
	}
	slot, ok := latestSlot(cfg.Times, now, m.d.Config.Location)
	if !ok {
		return nil
	}
	state := m.digestState(ctx)
	if state.Slot == nil {
		// First run: nothing is due yet. Wait for the next time point.
		return m.saveDigestState(ctx, state, &slot, nil)
	}
	if !slot.After(*state.Slot) {
		return nil
	}
	if pending > 0 {
		if _, err := m.flushDigest(ctx, now, false); err != nil {
			if errors.Is(err, errDigestBackoff) {
				return nil
			}
			return err
		}
	}
	return m.saveDigestState(ctx, m.digestState(ctx), &slot, nil)
}

func (m *Module) saveDigestState(ctx context.Context, s digestState, slot, sentAt *time.Time) error {
	if slot != nil {
		s.Slot = slot
	}
	if sentAt != nil {
		s.SentAt = sentAt
	}
	return m.d.Settings.Set(ctx, digestStateKey, s)
}

// ---- sending ----

// flushDigest merges the queue into one notification and sends it through
// the normal channels. Rows are removed only after a channel accepted it, or
// when no channel is set up (the notification center has them anyway).
func (m *Module) flushDigest(ctx context.Context, now time.Time, manual bool) (api.DigestFlushResult, error) {
	m.digestMu.Lock()
	defer m.digestMu.Unlock()
	if manual && m.quietNow(ctx, now) {
		return api.DigestFlushResult{}, httpx.NewError(http.StatusConflict, "quiet_hours", "现在是免打扰时段。免打扰结束后会自动发送")
	}
	if !manual && now.Before(m.digestRetryAt) {
		return api.DigestFlushResult{}, errDigestBackoff
	}
	items, err := m.q.ListDigestItems(ctx)
	if err != nil {
		return api.DigestFlushResult{}, err
	}
	if len(items) == 0 {
		return api.DigestFlushResult{}, nil
	}
	msg := buildDigest(items, now, m.d.Config.Location, m.publicLink)
	ok, failed := m.d.Notify.DeliverNow(ctx, notify.Stored{CreatedAt: now.UTC(), Notification: msg})
	if ok == 0 && failed > 0 {
		m.digestRetryAt = now.Add(digestRetryAfter)
		return api.DigestFlushResult{}, httpx.NewError(http.StatusBadGateway, "delivery_failed", "合并推送发送失败，积压的通知还在，稍后会重试")
	}
	if err := m.q.DeleteDigestItemsUpTo(ctx, items[len(items)-1].ID); err != nil {
		return api.DigestFlushResult{}, err
	}
	sent := now.UTC()
	if err := m.saveDigestState(ctx, m.digestState(ctx), nil, &sent); err != nil {
		m.d.Log.Warn("save digest state", "err", err)
	}
	m.d.Bus.Publish("notify.digest_sent", map[string]any{"count": len(items), "delivered": ok})
	return api.DigestFlushResult{Count: len(items), Delivered: ok}, nil
}

// buildDigest writes the merged notification: one block per category with
// the newest few titles and links, and a count for the rest.
func buildDigest(items []db.NotifyDigestQueue, now time.Time, loc *time.Location, link func(string) string) notify.Notification {
	groups := map[string][]db.NotifyDigestQueue{}
	priority := notify.PriorityNormal
	stale := 0
	for _, it := range items {
		groups[it.Category] = append(groups[it.Category], it)
		if it.Priority == notify.PriorityHigh {
			priority = notify.PriorityHigh
		}
		if now.Sub(it.CreatedAt) > digestStale {
			stale++
		}
	}
	var b strings.Builder
	if stale > 0 {
		fmt.Fprintf(&b, "其中 %d 条已经积压超过 36 小时，后面标了原来的时间。\n", stale)
	}
	budget := digestMaxItems
	for _, cat := range digestCategories {
		list := groups[cat.ID]
		if len(list) == 0 {
			continue
		}
		show := min(digestGroupItems, budget, len(list))
		budget -= show
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "【%s】%d 条\n", cat.Label, len(list))
		for _, it := range list[len(list)-show:] {
			b.WriteString("· " + strings.TrimSpace(it.Title))
			if now.Sub(it.CreatedAt) > digestStale {
				b.WriteString("（" + it.CreatedAt.In(loc).Format("01-02 15:04") + "）")
			}
			b.WriteString("\n")
			if u := link(it.Link); u != "" {
				b.WriteString("  " + u + "\n")
			}
		}
		if rest := len(list) - show; rest > 0 {
			fmt.Fprintf(&b, "另有 %d 条没有列出，在通知中心查看\n", rest)
		}
	}
	body := strings.TrimRight(b.String(), "\n")
	if r := []rune(body); len(r) > digestBodyRunes {
		body = string(r[:digestBodyRunes]) + "…"
	}
	return notify.Notification{
		Kind: "notify.digest", Title: fmt.Sprintf("通知汇总：%d 条", len(items)), Body: body, Link: "/",
		Priority: priority, Source: "reminders", Data: map[string]any{"digest": true, "count": len(items)},
	}
}

// ---- API ----

func (m *Module) digestStatus(ctx context.Context, now time.Time) (api.DigestStatus, error) {
	cfg := m.digestSettings(ctx)
	out := api.DigestStatus{Enabled: cfg.Enabled, Times: digestTimesSorted(cfg.Times), Categories: []api.DigestCategory{}}
	for _, c := range digestCategories {
		out.Categories = append(out.Categories, api.DigestCategory{Id: c.ID, Mode: api.DigestMode(cfg.mode(c.ID)), DefaultMode: api.DigestMode(c.Default)})
	}
	n, err := m.q.CountDigestItems(ctx)
	if err != nil {
		return out, err
	}
	out.Pending = int(n)
	if n > 0 {
		if at, err := m.q.OldestDigestItemAt(ctx); err == nil {
			out.OldestAt = &at
		}
	}
	if st := m.digestState(ctx); st.SentAt != nil {
		out.LastSentAt = st.SentAt
	}
	if cfg.Enabled {
		if t, ok := nextSlot(cfg.Times, now, m.d.Config.Location); ok {
			t = t.UTC()
			out.NextAt = &t
		}
	}
	return out, nil
}

func digestTimesSorted(times []string) []string {
	out := []string{}
	for _, s := range digestSlots(times) {
		out = append(out, fmt.Sprintf("%02d:%02d", s/60, s%60))
	}
	return out
}

// normalizeDigest validates a settings update.
func normalizeDigest(in api.DigestSettings) (digestConfig, error) {
	seen := map[string]bool{}
	var times []string
	for _, s := range in.Times {
		v, ok := parseClock(s)
		if !ok {
			return digestConfig{}, httpx.Invalid("时间格式要写成 11:00 这样")
		}
		key := fmt.Sprintf("%02d:%02d", v/60, v%60)
		if seen[key] {
			return digestConfig{}, httpx.Invalid("时间点不能重复")
		}
		seen[key] = true
		times = append(times, key)
	}
	if len(times) == 0 || len(times) > 6 {
		return digestConfig{}, httpx.Invalid("时间点要有 1 到 6 个")
	}
	modes := map[string]string{}
	for id, mode := range in.Modes {
		if _, ok := digestCategoryByID(id); !ok {
			return digestConfig{}, httpx.Invalid("没有这个分类：" + id)
		}
		if !mode.Valid() {
			return digestConfig{}, httpx.Invalid("模式只能是 now 或 digest")
		}
		modes[id] = string(mode)
	}
	return digestConfig{Enabled: in.Enabled, Times: digestTimesSorted(times), Modes: modes}, nil
}

func (m *Module) GetNotifyDigest(w http.ResponseWriter, r *http.Request) {
	out, err := m.digestStatus(r.Context(), time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) UpdateNotifyDigest(w http.ResponseWriter, r *http.Request) {
	var body api.UpdateNotifyDigestJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg, err := normalizeDigest(body)
	if err == nil {
		err = m.d.Settings.Set(r.Context(), digestKey, cfg)
	}
	m.d.Audit.Record(r.Context(), "notify.digest.update", "", map[string]any{"enabled": body.Enabled, "times": body.Times}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("notify.digest_updated", map[string]any{"enabled": cfg.Enabled})
	out, err := m.digestStatus(r.Context(), time.Now())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) FlushNotifyDigest(w http.ResponseWriter, r *http.Request) {
	out, err := m.flushDigest(r.Context(), time.Now(), true)
	m.d.Audit.Record(r.Context(), "notify.digest.flush", "", map[string]any{"count": out.Count}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// mutedEverywhere reports whether B113 mute rules keep n from every channel
// its route would use (quiet hours aside: the digest sends after them). A
// digest line must not bring back a notification the user muted.
func (m *Module) mutedEverywhere(ctx context.Context, n notify.Stored) bool {
	rows, err := m.q.ListRoutes(ctx)
	if err != nil {
		return false
	}
	routes := make([]route, 0, len(rows))
	for _, row := range rows {
		routes = append(routes, routeFromDB(row))
	}
	names := pickChannels(routes, n.Kind, n.Priority, false)
	if len(names) == 0 {
		return false
	}
	mutes := m.loadMutes(ctx)
	reachable := 0
	for _, name := range names {
		c, ok := m.d.Notify.Channel(name)
		if !ok {
			continue
		}
		if cc, ok := c.(configurable); ok && !cc.Configured(ctx) {
			continue
		}
		reachable++
		if !muted(mutes, n, name) {
			return false
		}
	}
	return reachable > 0
}
