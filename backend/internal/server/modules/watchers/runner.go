package watchers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// runState is one run in progress.
type runState struct {
	id      int64
	kind    string
	answers chan string
	cancel  context.CancelFunc

	mu      sync.Mutex
	waiting bool
}

// errors a run ends with.
var (
	errCancelled = errors.New("用户取消了")
	errNoAnswer  = errors.New("等了 30 分钟没人处理")
	errNeedLogin = errors.New("网站要登录，但是没有存用户名和密码")
	errNoAI      = errors.New("没有配置 AI 模型，去设置 → AI 里配一个能看图的模型")
	errBudget    = errors.New("今天的 AI 花费到上限了，明天再试，或者在网页盯梢的设置里调高上限")
	errAskLimit  = errors.New("今天已经提醒过你 3 次，这次先不打扰")
)

func (m *Module) busy(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[id] != nil
}

// startRun starts a run in the background. It returns false when the
// watcher is already running.
func (m *Module) startRun(id int64, kind string) bool {
	m.mu.Lock()
	if m.running[id] != nil {
		m.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(m.ctx)
	rs := &runState{id: id, kind: kind, answers: make(chan string, 1), cancel: cancel}
	m.running[id] = rs
	m.mu.Unlock()
	m.bg.Add(1)
	go func() {
		defer m.bg.Done()
		defer cancel()
		defer func() {
			m.mu.Lock()
			delete(m.running, id)
			m.mu.Unlock()
			m.publish(id)
		}()
		if err := m.run(ctx, rs); err != nil {
			m.d.Log.Warn("watcher run", "id", id, "kind", kind, "err", err)
		}
	}()
	return true
}

func (m *Module) cancelRun(id int64) {
	m.mu.Lock()
	rs := m.running[id]
	m.mu.Unlock()
	if rs != nil {
		rs.cancel()
	}
}

// answer hands the user's answer to a waiting run.
func (m *Module) answer(id int64, a string) bool {
	m.mu.Lock()
	rs := m.running[id]
	m.mu.Unlock()
	if rs == nil {
		return false
	}
	rs.mu.Lock()
	waiting := rs.waiting
	rs.mu.Unlock()
	if !waiting {
		return false
	}
	select {
	case rs.answers <- a:
		return true
	default:
		return false
	}
}

func (m *Module) onNotifyAction(ctx context.Context, actionID string) error {
	// watcher.approve:3, watcher.deny:3, watcher.save:<change id>
	name, idText, ok := strings.Cut(strings.TrimPrefix(actionID, "watcher."), ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if !ok || err != nil || (name != "approve" && name != "deny" && name != "save") {
		return fmt.Errorf("不认识的按钮 %s", actionID)
	}
	if name == "save" {
		return m.saveChange(ctx, id)
	}
	if !m.answer(id, name) {
		return errors.New("这个任务现在没有在等你")
	}
	return nil
}

// tick starts the checks that are due.
func (m *Module) tick(ctx context.Context) error {
	rows, err := m.q.ListWatchers(ctx)
	if err != nil {
		return err
	}
	now := m.now()
	for _, w := range rows {
		if w.Enabled == 0 || m.busy(w.ID) {
			continue
		}
		switch {
		case w.NextCheckAt == nil || !now.Before(*w.NextCheckAt):
			m.startRun(w.ID, "check")
		case keepaliveDue(w, now):
			m.startRun(w.ID, "keepalive")
		}
	}
	return nil
}

func keepaliveDue(w db.Watcher, now time.Time) bool {
	if w.KeepaliveMinutes <= 0 || w.StateSealed == "" || w.Recipe == "" {
		return false
	}
	every := time.Duration(w.KeepaliveMinutes) * time.Minute
	last := w.LastKeepaliveAt
	if w.LastCheckedAt != nil && (last == nil || w.LastCheckedAt.After(*last)) {
		last = w.LastCheckedAt
	}
	return last == nil || now.Sub(*last) >= every
}

// recorder collects the steps of a run for the history.
type recorder struct {
	m      *Module
	wid    int64
	runID  int64
	mu     sync.Mutex
	steps  []step
	tokens int64
}

type step struct {
	At     time.Time `json:"at"`
	Text   string    `json:"text"`
	ShotID *int64    `json:"shotId,omitempty"`
}

func (r *recorder) add(text string) {
	r.mu.Lock()
	r.steps = append(r.steps, step{At: r.m.now(), Text: clip(text, 500)})
	r.mu.Unlock()
	r.save("running", "")
}

// shot stores a screenshot of the page with the last step.
func (r *recorder) shot(ctx context.Context, x *browser.Session) *int64 {
	img, err := x.Screenshot(ctx, 60)
	if err != nil || len(img) == 0 {
		return nil
	}
	id, err := r.m.q.CreateShot(ctx, db.CreateShotParams{WatcherID: r.wid, Data: img, CreatedAt: r.m.now()})
	if err != nil {
		return nil
	}
	r.mu.Lock()
	if n := len(r.steps); n > 0 {
		r.steps[n-1].ShotID = &id
	}
	r.mu.Unlock()
	return &id
}

func (r *recorder) save(outcome, detail string) {
	r.mu.Lock()
	b, _ := json.Marshal(r.steps)
	tokens := r.tokens
	r.mu.Unlock()
	var finished *time.Time
	if outcome != "running" {
		now := r.m.now()
		finished = &now
	}
	_ = r.m.q.UpdateRun(context.Background(), db.UpdateRunParams{Outcome: outcome, Detail: clip(detail, 1000), Steps: string(b),
		Tokens: tokens, FinishedAt: finished, ID: r.runID})
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// run does one check, exploration, manual login or keep-alive visit.
func (m *Module) run(ctx context.Context, rs *runState) error {
	w, err := m.q.GetWatcher(ctx, rs.id)
	if err != nil {
		return err
	}
	kind := rs.kind
	if w.Kind == "x" {
		if kind == "keepalive" {
			return nil
		}
		kind = "check"
	}
	if kind == "check" && w.Kind != "x" && (w.Recipe == "" || w.Mode == "ai") {
		kind = "explore"
	}
	runKind := kind
	if rs.kind == "check" && kind == "explore" && w.Recipe != "" {
		runKind = "check" // ai mode: still a check in the history
	}
	if w.EgressAgentID != "" && !m.d.Agents.Online(w.EgressAgentID) {
		return m.egressOffline(ctx, w, kind)
	}
	started := time.Now()
	runID, err := m.q.CreateRun(ctx, db.CreateRunParams{WatcherID: w.ID, Kind: runKind, StartedAt: m.now()})
	if err != nil {
		return err
	}
	rec := &recorder{m: m, wid: w.ID, runID: runID}
	status := "checking"
	if kind == "explore" {
		status = "exploring"
	}
	if kind != "keepalive" {
		_ = m.q.SetWatcherStatus(ctx, db.SetWatcherStatusParams{Status: status, StatusDetail: "", UpdatedAt: m.now(), ID: w.ID})
		m.publish(w.ID)
	}

	if w.Kind == "x" {
		return m.finish(ctx, w, rec, kind, m.runX(ctx, w, rec))
	}

	if (kind == "explore") && m.chat() == nil {
		return m.finish(ctx, w, rec, kind, result{err: errNoAI})
	}
	if kind == "explore" && !m.chat().Available(ctx, "agent") {
		return m.finish(ctx, w, rec, kind, result{err: errNoAI})
	}

	proxy := m.proxy(w)
	var sess *browser.Session
	if w.EgressAgentID != "" {
		proxy, err = m.egress.proxyURL(w.ID) // B208: out through the chosen machine
	}
	if err == nil {
		sess, err = m.browser.Open(ctx, browser.Options{Key: sessionKey(w.ID), Proxy: proxy, State: m.state(w),
			AllowedHosts: parseHosts(w.AllowedHosts), Timezone: m.timezone(), Locale: "zh-CN"})
	}
	if err != nil {
		if kind == "keepalive" {
			rec.save("failed", err.Error())
			return nil
		}
		return m.finish(ctx, w, rec, kind, result{err: err})
	}
	defer sess.Close()

	var res result
	switch kind {
	case "manual":
		res = m.runManual(ctx, rs, w, sess, rec)
	case "keepalive":
		res = m.runKeepalive(ctx, w, sess, rec)
	case "explore":
		res = m.runExplore(ctx, rs, w, sess, rec)
	default:
		res = m.runCheck(ctx, rs, w, sess, rec)
	}
	if res.err != nil && w.EgressAgentID != "" {
		// Say why the machine could not reach the site, not only that the page failed.
		if msg := m.egress.failedSince(w.ID, started); msg != "" {
			res.err = fmt.Errorf("%s：%w", msg, res.err)
		}
	}
	// Keep the login whatever happened, unless the page is a login form:
	// then the stored state is no better than before.
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if st, err := sess.SaveState(sctx); err == nil && len(st) > 0 {
		if sealed, err := m.seal(string(st)); err == nil {
			_ = m.q.SetWatcherState(sctx, db.SetWatcherStateParams{StateSealed: sealed, ID: w.ID})
		}
	}
	if kind == "keepalive" {
		_ = m.q.SetWatcherKeepalive(sctx, db.SetWatcherKeepaliveParams{LastKeepaliveAt: ptr(m.now()), ID: w.ID})
		outcome := "ok"
		if res.err != nil {
			outcome = "failed"
		}
		rec.save(outcome, res.detail)
		return nil
	}
	return m.finish(sctx, w, rec, kind, res)
}

// egressOffline is a run that cannot start because the machine the browser
// goes out through is not connected. It is not a failure: the status stays,
// the detail says why, and the next check comes after the usual interval.
func (m *Module) egressOffline(ctx context.Context, w db.Watcher, kind string) error {
	if kind == "keepalive" {
		return nil
	}
	name := w.EgressAgentID
	if a, err := m.d.Agents.Get(ctx, w.EgressAgentID); err == nil && a.Name != "" {
		name = a.Name
	}
	now := m.now()
	next := now.Add(time.Duration(w.IntervalMinutes) * time.Minute)
	if err := m.q.SetWatcherNextCheck(ctx, db.SetWatcherNextCheckParams{StatusDetail: "出口机器 " + name + " 不在线，等它上线再检查",
		NextCheckAt: &next, UpdatedAt: now, ID: w.ID}); err != nil {
		return err
	}
	m.publish(w.ID)
	return nil
}

// result is how a run ended.
type result struct {
	value   string
	found   bool // value was read
	recipe  *Recipe
	detail  string
	err     error
	login   bool // ended on a login page or captcha nobody solved
	shot    *int64
	noRetry bool // a failure that AI cannot fix by exploring again
	xState  *xState
	xFresh  []xPost
}

func (m *Module) runManual(ctx context.Context, rs *runState, w db.Watcher, sess *browser.Session, rec *recorder) result {
	rec.add("打开 " + w.Url)
	if err := sess.Navigate(ctx, w.Url); err != nil {
		return result{err: err}
	}
	ans := m.ask(ctx, rs, w, sess, rec, "manual", "浏览器已经打开，登录或者处理好以后点“我弄好了”。", false)
	if ans != "done" {
		return result{err: errCancelled, noRetry: true}
	}
	rec.add("用户处理完了，保存登录状态")
	return result{detail: "已保存登录状态"}
}

func (m *Module) runKeepalive(ctx context.Context, w db.Watcher, sess *browser.Session, rec *recorder) result {
	rec.add("访问 " + w.Url + " 保持登录")
	if err := sess.Navigate(ctx, w.Url); err != nil {
		return result{err: err, detail: err.Error()}
	}
	if st, err := sess.Check(ctx); err == nil && st.Password {
		return result{err: errors.New("已经掉线"), detail: "已经掉线，下次检查时重新登录"}
	}
	return result{detail: "还在登录状态"}
}

func (m *Module) runCheck(ctx context.Context, rs *runState, w db.Watcher, sess *browser.Session, rec *recorder) result {
	recipe, ok := parseRecipe(w.Recipe)
	if !ok {
		return m.runExplore(ctx, rs, w, sess, rec)
	}
	res := m.replay(ctx, rs, w, sess, rec, recipe)
	if res.err == nil || res.noRetry || res.login || ctx.Err() != nil {
		return res
	}
	// The steps broke. Let AI repair them, unless it already failed too often.
	if w.FailCount+1 >= failLimit {
		res.detail = res.err.Error() + "。连续失败，先不让 AI 修，点“重新摸索”再试"
		return res
	}
	rec.add("按记下的步骤走不通：" + res.err.Error() + "。让 AI 接着找")
	return m.runExplore(ctx, rs, w, sess, rec)
}

// ask pauses the run until the user answers, at most m.waitFor. notify
// says whether to send a notification (not for a run the user started by
// hand and is watching).
func (m *Module) ask(ctx context.Context, rs *runState, w db.Watcher, sess *browser.Session, rec *recorder, kind, message string, notifyUser bool) string {
	rec.add("等你处理：" + message)
	shot := rec.shot(ctx, sess)
	if notifyUser && !m.allowAsk(ctx, w) {
		rec.add(errAskLimit.Error())
		return "limit"
	}
	p := Pending{Kind: kind, Message: message, Since: m.now()}
	raw, _ := json.Marshal(p)
	_ = m.q.SetWatcherPending(ctx, db.SetWatcherPendingParams{Pending: string(raw), UpdatedAt: m.now(), ID: w.ID})
	_ = m.q.SetWatcherStatus(ctx, db.SetWatcherStatusParams{Status: "waiting", StatusDetail: message, UpdatedAt: m.now(), ID: w.ID})
	m.publish(w.ID)
	if notifyUser {
		n := notify.Notification{Kind: "watcher.ask", Title: w.Name + "：等你处理", Body: message,
			Link: fmt.Sprintf("/monitoring/watchers/%d/live", w.ID), Priority: notify.PriorityUrgent, Source: "watchers",
			Scope: "watcher:" + strconv.FormatInt(w.ID, 10), Immediate: true}
		if kind == "approve" {
			n.Actions = []notify.Action{{ID: fmt.Sprintf("watcher.approve:%d", w.ID), Label: "批准"},
				{ID: fmt.Sprintf("watcher.deny:%d", w.ID), Label: "拒绝"}}
		}
		if shot != nil {
			n.Data = map[string]any{"shotId": *shot}
		}
		if _, err := m.d.Notify.Send(ctx, n); err != nil {
			m.d.Log.Warn("watcher notify", "err", err)
		}
	}
	sess.Hold(m.waitFor + 5*time.Minute)
	rs.mu.Lock()
	rs.waiting = true
	rs.mu.Unlock()
	defer func() {
		rs.mu.Lock()
		rs.waiting = false
		rs.mu.Unlock()
		_ = m.q.SetWatcherPending(context.Background(), db.SetWatcherPendingParams{Pending: "", UpdatedAt: m.now(), ID: w.ID})
		status := "checking"
		if rs.kind == "explore" {
			status = "exploring"
		}
		_ = m.q.SetWatcherStatus(context.Background(), db.SetWatcherStatusParams{Status: status, StatusDetail: "", UpdatedAt: m.now(), ID: w.ID})
		m.publish(w.ID)
	}()
	timer := time.NewTimer(m.waitFor)
	defer timer.Stop()
	select {
	case a := <-rs.answers:
		switch a {
		case "done", "approve":
			rec.add("你点了“" + map[string]string{"done": "我弄好了", "approve": "批准"}[a] + "”")
		case "deny":
			rec.add("你拒绝了")
		default:
			rec.add("你取消了")
		}
		sess.Hold(browser.DefaultMaxUse)
		return a
	case <-timer.C:
		rec.add("等了 30 分钟没人处理")
		return "timeout"
	case <-ctx.Done():
		return "cancel"
	case <-sess.Done():
		rec.add("浏览器关了")
		return "cancel"
	}
}

// allowAsk counts the reminders of today and says whether one more is ok.
func (m *Module) allowAsk(ctx context.Context, w db.Watcher) bool {
	day := m.startOfDay().Format("2006-01-02")
	count := w.AsksCount
	if w.AsksDay != day {
		count = 0
	}
	if count >= askLimit {
		return false
	}
	_ = m.q.SetWatcherAsks(ctx, db.SetWatcherAsksParams{AsksDay: day, AsksCount: count + 1, ID: w.ID})
	return true
}

// finish stores the outcome of a run, records a change and notifies.
func (m *Module) finish(ctx context.Context, w db.Watcher, rec *recorder, kind string, res result) error {
	now := m.now()
	next := now.Add(time.Duration(w.IntervalMinutes) * time.Minute)
	fresh, err := m.q.GetWatcher(ctx, w.ID)
	if err != nil {
		return err // deleted while running
	}
	if res.recipe != nil {
		b, _ := json.Marshal(res.recipe)
		_ = m.q.SetWatcherRecipe(ctx, db.SetWatcherRecipeParams{Recipe: string(b), UpdatedAt: now, ID: w.ID})
	}
	if res.xState != nil {
		b, _ := json.Marshal(res.xState)
		_ = m.q.SetWatcherRecipe(ctx, db.SetWatcherRecipeParams{Recipe: string(b), UpdatedAt: now, ID: w.ID})
	}
	params := db.FinishWatcherCheckParams{CurrentValue: fresh.CurrentValue, LastCheckedAt: &now, NextCheckAt: &next,
		LastChangeAt: fresh.LastChangeAt, UpdatedAt: now, ID: w.ID}
	switch {
	case kind == "manual" && res.err == nil:
		params.Status, params.StatusDetail = "ok", res.detail
		if fresh.Recipe == "" && res.recipe == nil {
			params.Status = "new"
			params.NextCheckAt = &now // explore now that the user is logged in
		}
		params.LastCheckedAt = fresh.LastCheckedAt
		rec.save("ok", res.detail)
	case res.err != nil:
		params.FailCount = fresh.FailCount + 1
		params.Status, params.StatusDetail = "error", res.err.Error()
		if res.detail != "" {
			params.StatusDetail = res.detail
		}
		if res.login {
			params.Status = "login"
			params.FailCount = fresh.FailCount
		}
		if errors.Is(res.err, errCancelled) {
			params.Status, params.FailCount = fresh.Status, fresh.FailCount
			if params.Status == "exploring" || params.Status == "checking" || params.Status == "waiting" {
				params.Status = "ok"
				if fresh.Recipe == "" {
					params.Status = "new"
				}
			}
		}
		outcome := "failed"
		if errors.Is(res.err, errCancelled) {
			outcome = "cancelled"
		}
		rec.save(outcome, params.StatusDetail)
		if params.Status == "error" && params.FailCount == failLimit {
			m.sendNotice(ctx, w, "watcher.failed", w.Name+"：连续 3 次没查到", params.StatusDetail, notify.PriorityHigh)
		}
	default:
		params.Status, params.StatusDetail = "ok", ""
		old := strings.TrimSpace(fresh.CurrentValue)
		value := strings.TrimSpace(res.value)
		params.CurrentValue = value
		outcome := "same"
		if kind == "explore" && fresh.Recipe == "" {
			outcome = "ok"
		}
		switch {
		case len(res.xFresh) > 0:
			outcome = "changed"
			params.LastChangeAt = &now
			m.recordXPosts(ctx, w, res.xFresh)
		case w.Kind != "x" && old != "" && value != old:
			outcome = "changed"
			params.LastChangeAt = &now
			m.recordChange(ctx, w, old, value, res.shot)
		}
		detail := res.detail
		if detail == "" {
			detail = "当前值：" + clip(value, 200)
		}
		rec.save(outcome, detail)
	}
	if err := m.q.FinishWatcherCheck(ctx, params); err != nil {
		return err
	}
	m.publish(w.ID)
	return nil
}

func (m *Module) sendNotice(ctx context.Context, w db.Watcher, kind, title, body, priority string) {
	_, err := m.d.Notify.Send(ctx, notify.Notification{Kind: kind, Title: title, Body: body,
		Link: fmt.Sprintf("/monitoring/watchers/%d", w.ID), Priority: priority, Source: "watchers",
		Scope: "watcher:" + strconv.FormatInt(w.ID, 10)})
	if err != nil {
		m.d.Log.Warn("watcher notify", "err", err)
	}
}

// recordChange asks AI whether the change matters, stores it and notifies.
func (m *Module) recordChange(ctx context.Context, w db.Watcher, old, value string, shot *int64) {
	summary, important := m.judge(ctx, w, old, value)
	_, _ = m.q.CreateChange(ctx, db.CreateChangeParams{WatcherID: w.ID, OldValue: old, NewValue: value, Summary: summary,
		Important: boolInt(important), ShotID: shot, CreatedAt: m.now()})
	if !important {
		return
	}
	body := fmt.Sprintf("从“%s”变成“%s”", clip(old, 200), clip(value, 300))
	m.sendNotice(ctx, w, "watcher.changed", w.Name+"："+summary, body, notify.PriorityHigh)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

const judgeSystem = `你帮用户盯一个网页上的内容。用户告诉你他关心什么，你拿到这个内容变化前后的文字。
判断这次变化是不是用户关心的变化。只是时间戳、访问次数、广告、排序这类无关紧要的变化，important 填 false。
summary 用一句中文说清楚变了什么，不超过 30 个字，比如“办理状态变成已受理”。`

var judgeSchema = json.RawMessage(`{"type":"object","properties":{"important":{"type":"boolean"},"summary":{"type":"string"}},"required":["important","summary"],"additionalProperties":false}`)

func (m *Module) judge(ctx context.Context, w db.Watcher, old, value string) (string, bool) {
	fallback := "内容有变化"
	l := m.llm()
	if l == nil || !l.Available(ctx) || m.overBudget(ctx) {
		return fallback, true
	}
	var out struct {
		Important bool   `json:"important"`
		Summary   string `json:"summary"`
	}
	ctx = contracts.WithAIUsage(ctx, "watchers", strconv.FormatInt(w.ID, 10))
	user := fmt.Sprintf("用户关心的：%s\n\n变化前：\n%s\n\n变化后：\n%s", w.Goal, clip(old, 3000), clip(value, 3000))
	if err := l.CompleteJSON(ctx, "fast", judgeSystem, user, judgeSchema, &out); err != nil || strings.TrimSpace(out.Summary) == "" {
		return fallback, true
	}
	return clip(strings.TrimSpace(out.Summary), 60), out.Important
}

// overBudget reports whether today's AI cost reached the limit.
func (m *Module) overBudget(ctx context.Context) bool {
	budget, _ := m.options(ctx)
	if budget <= 0 {
		return true
	}
	c := m.chat()
	if c == nil {
		return false
	}
	spent, err := c.Spent(ctx, "watchers", "", m.startOfDay())
	return err == nil && spent >= budget
}
