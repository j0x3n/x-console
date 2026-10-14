package watchers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/db"
	"github.com/j0x3n/x-console/backend/internal/server/notify"
)

// B207: watching X. No browser: the readlater module calls the web
// interface of X with the user's cookie (contracts.XGraphQL). Three targets:
// new posts of an account, new results of a search, new replies to a post.

const (
	xDefaultInterval = 30
	xSeenKeep        = 300
	// xPerMinute is how many calls to X all watchers make in a minute.
	xPerMinute = 10
)

// defaultXQueryIDs are the ids of the web client at the time of writing. X
// changes them now and then; the user can override them in the settings.
var defaultXQueryIDs = map[string]string{
	"UserByScreenName": "xmU6X_CKVnQ5lSrCbAmJsg",
	"UserTweets":       "E3opETHurmVJflFsUBVuUQ",
	"SearchTimeline":   "AIdc203rPpK_k_2KWSdm7g",
	"TweetDetail":      "QuBlQ6SxNAQCt6-kBiCXCQ",
}

// xFeatures are the switches the web client sends along. X answers a request
// that leaves out one it needs with an error that names it.
const xFeatures = `{"rweb_tipjar_consumption_enabled":true,"responsive_web_graphql_exclude_directive_enabled":true,"verified_phone_label_enabled":false,"creator_subscriptions_tweet_preview_api_enabled":true,"responsive_web_graphql_timeline_navigation_enabled":true,"responsive_web_graphql_skip_user_profile_image_extensions_enabled":false,"communities_web_enable_tweet_community_results_fetch":true,"c9s_tweet_anatomy_moderator_badge_enabled":true,"articles_preview_enabled":true,"responsive_web_edit_tweet_api_enabled":true,"graphql_is_translatable_rweb_tweet_is_translatable_enabled":true,"view_counts_everywhere_api_enabled":true,"longform_notetweets_consumption_enabled":true,"responsive_web_twitter_article_tweet_consumption_enabled":true,"tweet_awards_web_tipping_enabled":false,"creator_subscriptions_quote_tweet_preview_enabled":false,"freedom_of_speech_not_reach_fetch_enabled":true,"standardized_nudges_misinfo":true,"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled":true,"rweb_video_timestamps_enabled":true,"longform_notetweets_rich_text_read_enabled":true,"longform_notetweets_inline_media_enabled":true,"responsive_web_enhance_cards_enabled":false,"hidden_profile_subscriptions_enabled":true,"subscriptions_verification_info_is_identity_verified_enabled":true,"subscriptions_verification_info_verified_since_enabled":true,"highlights_tweets_tab_ui_enabled":true,"responsive_web_twitter_article_notes_tab_enabled":true,"subscriptions_feature_can_gift_premium":true,"profile_label_improvements_pcf_label_in_post_enabled":true}`

// xTarget is what an X watcher follows.
type xTarget struct {
	Kind  string // user, search, replies
	Value string // handle, query or post id
}

var (
	handleRe = regexp.MustCompile(`^@?([A-Za-z0-9_]{1,15})$`)
	postRe   = regexp.MustCompile(`^/(?:([A-Za-z0-9_]{1,15})|i(?:/web)?)/status(?:es)?/(\d{1,25})`)
	userRe   = regexp.MustCompile(`^/([A-Za-z0-9_]{1,15})/?$`)
)

// reservedPaths are x.com pages that look like an account but are not.
var reservedPaths = map[string]bool{"home": true, "explore": true, "search": true, "notifications": true, "messages": true,
	"settings": true, "i": true, "login": true, "signup": true, "tos": true, "privacy": true}

// parseXTarget understands @handle, an account or post link, a search link
// or plain search words, and returns the canonical x.com address.
func parseXTarget(in string) (xTarget, string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return xTarget{}, "", httpx.Invalid("要写 @账号、账号或推文的链接、或者搜索词")
	}
	if m := handleRe.FindStringSubmatch(in); m != nil && !reservedPaths[strings.ToLower(m[1])] {
		return xTarget{"user", m[1]}, "https://x.com/" + m[1], nil
	}
	if u, err := url.Parse(in); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
		host = strings.TrimPrefix(host, "mobile.")
		if host != "x.com" && host != "twitter.com" {
			return xTarget{}, "", httpx.Invalid("只认 x.com 或 twitter.com 的链接")
		}
		if m := postRe.FindStringSubmatch(u.Path); m != nil {
			return xTarget{"replies", m[2]}, "https://x.com/i/status/" + m[2], nil
		}
		if u.Path == "/search" {
			q := strings.TrimSpace(u.Query().Get("q"))
			if q == "" {
				return xTarget{}, "", httpx.Invalid("搜索链接里没有搜索词")
			}
			return searchTarget(q)
		}
		if m := userRe.FindStringSubmatch(u.Path); m != nil && !reservedPaths[strings.ToLower(m[1])] {
			return xTarget{"user", m[1]}, "https://x.com/" + m[1], nil
		}
		return xTarget{}, "", httpx.Invalid("认不出这个链接，写账号主页、推文或搜索的链接")
	}
	return searchTarget(in)
}

func searchTarget(q string) (xTarget, string, error) {
	if len([]rune(q)) > 500 {
		return xTarget{}, "", httpx.Invalid("搜索词最多 500 个字")
	}
	return xTarget{"search", q}, "https://x.com/search?" + url.Values{"q": {q}, "f": {"live"}}.Encode(), nil
}

// targetOf reads the target back from the stored address.
func targetOf(stored string) (xTarget, error) {
	t, _, err := parseXTarget(stored)
	return t, err
}

// xState is what an X watcher remembers between checks; it lives in the
// recipe column.
type xState struct {
	UserID string   `json:"userId,omitempty"`
	Handle string   `json:"handle,omitempty"`
	Seen   []string `json:"seen"`
}

// xPost is one post read from X.
type xPost struct {
	ID        string
	Handle    string
	Name      string
	Text      string
	CreatedAt time.Time
	Repost    bool
}

func (p xPost) URL() string {
	h := p.Handle
	if h == "" {
		h = "i"
	}
	return "https://x.com/" + h + "/status/" + p.ID
}

func (p xPost) line() string {
	return "@" + p.Handle + "：" + clip(strings.Join(strings.Fields(p.Text), " "), 200) + "\n" + p.URL()
}

// xLimiter spaces the calls to X of all watchers.
type xLimiter struct {
	mu    sync.Mutex
	calls []time.Time
}

func (l *xLimiter) wait(ctx context.Context, now func() time.Time) error {
	for {
		l.mu.Lock()
		cut := now().Add(-time.Minute)
		kept := l.calls[:0]
		for _, c := range l.calls {
			if c.After(cut) {
				kept = append(kept, c)
			}
		}
		l.calls = kept
		if len(l.calls) < xPerMinute {
			l.calls = append(l.calls, now())
			l.mu.Unlock()
			return nil
		}
		next := l.calls[0].Add(time.Minute).Sub(now())
		l.mu.Unlock()
		select {
		case <-time.After(next):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Module) xClient() contracts.XGraphQL {
	if m.xOverride != nil {
		return m.xOverride
	}
	c, _ := module.Lookup[contracts.XGraphQL](m.d.Registry, contracts.XGraphQLKey)
	return c
}

func (m *Module) xQuery(ctx context.Context, operation string, vars map[string]any) (any, error) {
	c := m.xClient()
	if c == nil {
		return nil, errors.New("稍后读模块没有打开，读不了 X")
	}
	ids := m.xQueryIDs(ctx)
	if err := m.xLimit.wait(ctx, m.now); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(vars)
	body, err := c.Query(ctx, ids[operation], operation, string(raw), xFeatures)
	if err != nil {
		return nil, err
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, errors.New("X 返回的内容读不懂")
	}
	if errs, ok := dig(doc, "errors").([]any); ok && len(errs) > 0 && dig(doc, "data") == nil {
		return nil, fmt.Errorf("X 返回了错误：%s（%s 的 queryId 可能要换）", clip(str(dig(errs[0], "message")), 120), operation)
	}
	return doc, nil
}

func (m *Module) xQueryIDs(ctx context.Context) map[string]string {
	var o Options
	_ = m.d.Settings.Get(ctx, OptionsKey, &o)
	out := map[string]string{}
	for k, v := range defaultXQueryIDs {
		out[k] = v
	}
	for k, v := range o.XQueryIDs {
		if _, ok := out[k]; ok && strings.TrimSpace(v) != "" {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// fetchX reads the newest posts of the target, newest first.
func (m *Module) fetchX(ctx context.Context, t xTarget, st *xState) ([]xPost, error) {
	switch t.Kind {
	case "user":
		if st.UserID == "" || !strings.EqualFold(st.Handle, t.Value) {
			doc, err := m.xQuery(ctx, "UserByScreenName", map[string]any{"screen_name": t.Value, "withSafetyModeUserFields": true})
			if err != nil {
				return nil, err
			}
			id := str(dig(doc, "data", "user", "result", "rest_id"))
			if id == "" {
				return nil, fmt.Errorf("X 上找不到账号 @%s", t.Value)
			}
			st.UserID, st.Handle = id, t.Value
		}
		doc, err := m.xQuery(ctx, "UserTweets", map[string]any{"userId": st.UserID, "count": 20, "includePromotedContent": false,
			"withQuickPromoteEligibilityTweetFields": false, "withVoice": true, "withV2Timeline": true})
		if err != nil {
			return nil, err
		}
		var own []xPost
		for _, p := range collectPosts(doc) {
			if strings.EqualFold(p.Handle, t.Value) && !p.Repost {
				own = append(own, p)
			}
		}
		return own, nil
	case "search":
		doc, err := m.xQuery(ctx, "SearchTimeline", map[string]any{"rawQuery": t.Value, "count": 20, "querySource": "typed_query", "product": "Latest"})
		if err != nil {
			return nil, err
		}
		return collectPosts(doc), nil
	default:
		doc, err := m.xQuery(ctx, "TweetDetail", map[string]any{"focalTweetId": t.Value, "with_rux_injections": false, "rankingMode": "Recency",
			"includePromotedContent": false, "withCommunity": true, "withQuickPromoteEligibilityTweetFields": true, "withBirdwatchNotes": true, "withVoice": true})
		if err != nil {
			return nil, err
		}
		var replies []xPost
		for _, p := range collectPosts(doc) {
			if p.ID != t.Value {
				replies = append(replies, p)
			}
		}
		return replies, nil
	}
}

// collectPosts finds every post in an answer of X. Timelines nest posts at
// different depths and X moves them around; every post sits under a
// "tweet_results" object, so the walk looks for that key only.
func collectPosts(doc any) []xPost {
	var out []xPost
	seen := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if tr, ok := x["tweet_results"].(map[string]any); ok {
				if p, ok := parsePost(tr["result"]); ok && !seen[p.ID] {
					seen[p.ID] = true
					out = append(out, p)
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(doc)
	// Newest first; X ids grow with time.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && idLess(out[j-1].ID, out[j].ID); j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

func idLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

func parsePost(v any) (xPost, bool) {
	res, _ := v.(map[string]any)
	if res == nil {
		return xPost{}, false
	}
	if str(res["__typename"]) == "TweetWithVisibilityResults" {
		res, _ = res["tweet"].(map[string]any)
		if res == nil {
			return xPost{}, false
		}
	}
	legacy, _ := res["legacy"].(map[string]any)
	id := str(res["rest_id"])
	if legacy == nil || id == "" {
		return xPost{}, false
	}
	p := xPost{ID: id, Repost: legacy["retweeted_status_result"] != nil}
	p.CreatedAt, _ = time.Parse(time.RubyDate, str(legacy["created_at"]))
	user := dig(res, "core", "user_results", "result")
	p.Handle = str(dig(user, "core", "screen_name"))
	p.Name = str(dig(user, "core", "name"))
	if p.Handle == "" {
		p.Handle = str(dig(user, "legacy", "screen_name"))
		p.Name = str(dig(user, "legacy", "name"))
	}
	text := str(legacy["full_text"])
	if long := str(dig(res, "note_tweet", "note_tweet_results", "result", "text")); long != "" {
		text = long
	}
	if urls, ok := dig(legacy, "entities", "urls").([]any); ok {
		for _, u := range urls {
			if short, full := str(dig(u, "url")), str(dig(u, "expanded_url")); short != "" && full != "" {
				text = strings.ReplaceAll(text, short, full)
			}
		}
	}
	p.Text = strings.TrimSpace(html.UnescapeString(text))
	return p, true
}

func dig(v any, keys ...string) any {
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

func str(v any) string { s, _ := v.(string); return s }

// runX checks an X watcher.
func (m *Module) runX(ctx context.Context, w db.Watcher, rec *recorder) result {
	c := m.xClient()
	if c == nil || !c.Ready(ctx) {
		rec.add(contracts.ErrXAuth.Error())
		return result{err: contracts.ErrXAuth, login: true, noRetry: true}
	}
	t, err := targetOf(w.Url)
	if err != nil {
		return result{err: err, noRetry: true}
	}
	var st xState
	_ = json.Unmarshal([]byte(w.Recipe), &st)
	first := w.Recipe == ""
	rec.add(map[string]string{"user": "读 @" + t.Value + " 的推文", "search": "搜索“" + clip(t.Value, 60) + "”",
		"replies": "读推文 " + t.Value + " 的回复"}[t.Kind])
	posts, err := m.fetchX(ctx, t, &st)
	if errors.Is(err, contracts.ErrXAuth) {
		return result{err: err, login: true, noRetry: true}
	}
	if err != nil {
		return result{err: err, noRetry: true}
	}
	seen := map[string]bool{}
	for _, id := range st.Seen {
		seen[id] = true
	}
	var fresh []xPost
	for _, p := range posts {
		if !seen[p.ID] {
			fresh = append(fresh, p)
		}
	}
	rec.add(fmt.Sprintf("读到 %d 条，新的 %d 条", len(posts), len(fresh)))
	// Remember the newest ids first so the list stays short.
	ids := make([]string, 0, len(posts)+len(st.Seen))
	for _, p := range posts {
		ids = append(ids, p.ID)
	}
	for _, id := range st.Seen {
		if !containsID(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > xSeenKeep {
		ids = ids[:xSeenKeep]
	}
	st.Seen = ids
	value := w.CurrentValue
	if len(posts) > 0 {
		value = posts[0].line()
	}
	res := result{value: value, found: true, xState: &st, detail: fmt.Sprintf("新推文 %d 条", len(fresh))}
	if first {
		res.detail = fmt.Sprintf("开始盯，现有的 %d 条不提醒", len(posts))
		return res
	}
	if len(fresh) > 0 {
		res.xFresh = fresh
	}
	return res
}

func containsID(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

const xFilterSystem = `你帮用户盯 X（推特）上的新推文。用户给了一句话说他关心什么，你拿到一批新推文，每条前面有编号。
挑出用户关心的推文，把编号放进 keep。一条都不相关就给空数组。
summary 用一句中文概括挑出来的推文说了什么，不超过 40 个字。没有挑出来就写“没有相关的”。`

var xFilterSchema = json.RawMessage(`{"type":"object","properties":{"keep":{"type":"array","items":{"type":"integer"}},"summary":{"type":"string"}},"required":["keep","summary"],"additionalProperties":false}`)

// recordXPosts filters the new posts with the user's sentence, stores them as
// one change and notifies.
func (m *Module) recordXPosts(ctx context.Context, w db.Watcher, fresh []xPost) {
	keep, summary := fresh, fmt.Sprintf("%d 条新推文", len(fresh))
	if goal := strings.TrimSpace(w.Goal); goal != "" {
		if l := m.llm(); l != nil && l.Available(ctx) && !m.overBudget(ctx) {
			var b strings.Builder
			for i, p := range fresh {
				fmt.Fprintf(&b, "[%d] @%s：%s\n", i, p.Handle, clip(p.Text, 500))
			}
			var out struct {
				Keep    []int  `json:"keep"`
				Summary string `json:"summary"`
			}
			actx := contracts.WithAIUsage(ctx, "watchers", strconv.FormatInt(w.ID, 10))
			if err := l.CompleteJSON(actx, "fast", xFilterSystem, "用户关心的："+goal+"\n\n新推文：\n"+b.String(), xFilterSchema, &out); err == nil {
				keep = nil
				for _, i := range out.Keep {
					if i >= 0 && i < len(fresh) {
						keep = append(keep, fresh[i])
					}
				}
				if s := strings.TrimSpace(out.Summary); s != "" {
					summary = clip(s, 60)
				}
			}
		}
	}
	var lines []string
	for _, p := range keep {
		lines = append(lines, p.line())
	}
	important := len(keep) > 0
	if !important {
		summary = fmt.Sprintf("%d 条新推文，都和“%s”无关", len(fresh), clip(w.Goal, 30))
		for _, p := range fresh {
			lines = append(lines, p.line())
		}
	}
	changeID, err := m.q.CreateChange(ctx, db.CreateChangeParams{WatcherID: w.ID, OldValue: "", NewValue: strings.Join(lines, "\n\n"),
		Summary: summary, Important: boolInt(important), CreatedAt: m.now()})
	if err != nil || !important {
		return
	}
	var body strings.Builder
	for i, p := range keep {
		if i == 3 {
			fmt.Fprintf(&body, "还有 %d 条，打开看全部", len(keep)-3)
			break
		}
		body.WriteString(p.line() + "\n")
	}
	title := w.Name + "：" + summary
	if len(keep) > 1 && !strings.Contains(summary, strconv.Itoa(len(keep))) {
		title = fmt.Sprintf("%s：%d 条新推文 · %s", w.Name, len(keep), summary)
	}
	_, err = m.d.Notify.Send(ctx, notify.Notification{Kind: "watcher.x", Title: title, Body: strings.TrimSpace(body.String()),
		Link: fmt.Sprintf("/monitoring/watchers/%d", w.ID), Priority: notify.PriorityNormal, Source: "watchers",
		Scope:   "watcher:" + strconv.FormatInt(w.ID, 10),
		Actions: []notify.Action{{ID: fmt.Sprintf("watcher.save:%d", changeID), Label: "存进稍后读"}}})
	if err != nil {
		m.d.Log.Warn("watcher notify", "err", err)
	}
}

var postLink = regexp.MustCompile(`https://x\.com/[A-Za-z0-9_]+/status/\d+`)

// saveChange puts the posts of a change into read later.
func (m *Module) saveChange(ctx context.Context, changeID int64) error {
	c, err := m.q.GetChange(ctx, changeID)
	if err != nil {
		return errors.New("找不到这条记录")
	}
	links := postLink.FindAllString(c.NewValue, 10)
	if len(links) == 0 {
		return errors.New("这条记录里没有推文链接")
	}
	for _, link := range links {
		raw, _ := json.Marshal(map[string]string{"url": link})
		if _, err := m.d.Actions.Run(contracts.IgnoreHidden(ctx), "readlater.add", raw); err != nil {
			return err
		}
	}
	return nil
}
