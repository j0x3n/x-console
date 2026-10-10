package readlater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Reading X posts (B143). X pages need a script to show anything, so the
// text is read from the interfaces behind them, in this order:
//  1. the public syndication interface (no login),
//  2. the web interface with the user's own cookie (full long posts),
//  3. a third party converter, only when the user turned it on.
//
// None of these is an official, stable interface. Each step stands alone, so
// one breaking does not stop the others.

const (
	syndicationBase = "https://cdn.syndication.twimg.com"
	graphqlBase     = "https://x.com/i/api/graphql"
	fxtwitterBase   = "https://api.fxtwitter.com"

	// defaultQueryID is the id of TweetResultByRestId in the web client at the
	// time of writing. X changes it from time to time, so the settings can
	// override it.
	defaultQueryID = "Xl5pC_lBk_gcO2ItU39DQw"
	// webBearer is the public token every x.com page sends.
	webBearer = "AAAAAAAAAAAAAAAAAAAAANRILgAAAAAAnNwIzUejRCOuH5E6I8xnZz4puTs%3D1Zv7ttfk8LF81IUq16cHjhLTvJu4FA33AGWWjCpTnA"

	maxTweetBody = 2 << 20
	testTweetID  = "20"
)

var (
	// errXAuth means X refused the cookie.
	errXAuth = errors.New("X 拒绝了登录 Cookie")
	// errTweetGone means the post is deleted, protected or hidden.
	errTweetGone = errors.New("推文已删除，或者要登录才能看")

	tweetHosts = map[string]bool{
		"x.com": true, "twitter.com": true, "mobile.twitter.com": true, "fxtwitter.com": true, "vxtwitter.com": true,
		"fixupx.com": true, "fixvx.com": true,
	}
	tweetPath = regexp.MustCompile(`^/(?:i/(?:web/)?|([A-Za-z0-9_]{1,15})/)status(?:es)?/(\d{1,20})(?:/|$)`)
	linkRe    = regexp.MustCompile(`https?://[^\s<>"']+`)
)

// parseTweetURL recognises the address of one X post.
func parseTweetURL(u *url.URL) (handle, id string, ok bool) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if !tweetHosts[host] {
		return "", "", false
	}
	m := tweetPath.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// canonicalTweetURL is the one address a post is saved under.
func canonicalTweetURL(handle, id string) string {
	if handle == "" {
		handle = "i"
	}
	return "https://x.com/" + handle + "/status/" + id
}

// ---------- the common shape ----------

type tweetMedia struct {
	Kind string `json:"kind"` // photo, video, gif
	URL  string `json:"url"`  // the picture, or the cover of a video
	Alt  string `json:"alt,omitempty"`
}

type tweetData struct {
	ID         string
	Handle     string
	Name       string
	Avatar     string
	Text       string
	CreatedAt  time.Time
	Media      []tweetMedia
	Quote      *tweetData
	Likes      int
	Replies    int
	Reposts    int
	Incomplete bool
	Via        string
}

// ---------- step 1: syndication ----------

type synURL struct {
	URL      string `json:"url"`
	Expanded string `json:"expanded_url"`
}

type synMedia struct {
	Type string `json:"type"`
	URL  string `json:"media_url_https"`
	Alt  string `json:"ext_alt_text"`
	TCo  string `json:"url"`
}

type synTweet struct {
	Typename string    `json:"__typename"`
	ID       string    `json:"id_str"`
	Text     string    `json:"text"`
	Created  string    `json:"created_at"`
	Range    []int     `json:"display_text_range"`
	Likes    int       `json:"favorite_count"`
	Replies  int       `json:"conversation_count"`
	Note     *struct{} `json:"note_tweet"`
	Quoted   *synTweet `json:"quoted_tweet"`
	User     struct {
		Name   string `json:"name"`
		Handle string `json:"screen_name"`
		Avatar string `json:"profile_image_url_https"`
	} `json:"user"`
	Entities struct {
		URLs  []synURL   `json:"urls"`
		Media []synMedia `json:"media"`
	} `json:"entities"`
	MediaDetails []synMedia `json:"mediaDetails"`
}

// syndicationToken is the value the page widget sends. The server does not
// seem to check it closely, but it wants one.
func syndicationToken(id string) string {
	n, err := strconv.ParseFloat(id, 64)
	if err != nil {
		return "0"
	}
	f := n / 1e15 * math.Pi
	whole := math.Floor(f)
	frac := f - whole
	out := strconv.FormatInt(int64(whole), 36)
	for i := 0; i < 11 && frac > 0; i++ {
		frac *= 36
		d := int(frac)
		out += strconv.FormatInt(int64(d), 36)
		frac -= float64(d)
	}
	return strings.ReplaceAll(out, "0", "")
}

func (m *Module) tweetSyndication(ctx context.Context, id string) (*tweetData, error) {
	q := url.Values{"id": {id}, "lang": {"en"}, "token": {syndicationToken(id)}}
	body, err := m.xGet(ctx, m.xURL("syndication")+"/tweet-result?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	var t synTweet
	if err := json.Unmarshal(body, &t); err != nil {
		return nil, errors.New("返回的内容读不懂")
	}
	if t.ID == "" || strings.Contains(t.Typename, "Tombstone") {
		return nil, errTweetGone
	}
	return t.toData(), nil
}

func (t *synTweet) toData() *tweetData {
	d := &tweetData{
		ID: t.ID, Handle: t.User.Handle, Name: t.User.Name, Avatar: t.User.Avatar, Likes: t.Likes, Replies: t.Replies,
		Incomplete: t.Note != nil, Via: "syndication",
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339, t.Created)
	text := []rune(t.Text)
	if len(t.Range) == 2 && t.Range[0] >= 0 && t.Range[1] <= len(text) && t.Range[0] <= t.Range[1] {
		// the range drops the leading mentions of a reply and the picture links at the end
		text = text[t.Range[0]:t.Range[1]]
	}
	s := string(text)
	for _, u := range t.Entities.URLs {
		if u.URL != "" && u.Expanded != "" {
			s = strings.ReplaceAll(s, u.URL, u.Expanded)
		}
	}
	for _, md := range append(append([]synMedia{}, t.Entities.Media...), t.MediaDetails...) {
		if md.TCo != "" {
			s = strings.ReplaceAll(s, md.TCo, "")
		}
	}
	d.Text = strings.TrimSpace(s)
	for _, md := range t.MediaDetails {
		d.Media = append(d.Media, mediaFrom(md.Type, md.URL, md.Alt))
	}
	if t.Quoted != nil && t.Quoted.ID != "" {
		d.Quote = t.Quoted.toData()
	}
	return d
}

func mediaFrom(kind, pic, alt string) tweetMedia {
	switch kind {
	case "animated_gif", "gif":
		kind = "gif"
	case "video":
	default:
		kind = "photo"
	}
	return tweetMedia{Kind: kind, URL: pic, Alt: alt}
}

// ---------- step 2: the web interface with the user's cookie ----------

// graphqlFeatures are the switches the web client sends along. X answers a
// request that leaves one out with an error that names it.
const graphqlFeatures = `{"creator_subscriptions_tweet_preview_api_enabled":true,"premium_content_api_read_enabled":false,"communities_web_enable_tweet_community_results_fetch":true,"c9s_tweet_anatomy_moderator_badge_enabled":true,"responsive_web_grok_analyze_button_fetch_trends_enabled":false,"responsive_web_grok_analyze_post_followups_enabled":false,"responsive_web_jetfuel_frame":false,"responsive_web_grok_share_attachment_enabled":true,"articles_preview_enabled":true,"responsive_web_edit_tweet_api_enabled":true,"graphql_is_translatable_rweb_tweet_is_translatable_enabled":true,"view_counts_everywhere_api_enabled":true,"longform_notetweets_consumption_enabled":true,"responsive_web_twitter_article_tweet_consumption_enabled":true,"tweet_awards_web_tipping_enabled":false,"responsive_web_grok_show_grok_translated_post":false,"responsive_web_grok_analysis_button_from_backend":false,"creator_subscriptions_quote_tweet_preview_enabled":false,"freedom_of_speech_not_reach_fetch_enabled":true,"standardized_nudges_misinfo":true,"tweet_with_visibility_results_prefer_gql_limited_actions_policy_enabled":true,"longform_notetweets_rich_text_read_enabled":true,"longform_notetweets_inline_media_enabled":true,"profile_label_improvements_pcf_label_in_post_enabled":true,"rweb_tipjar_consumption_enabled":true,"verified_phone_label_enabled":false,"responsive_web_grok_image_annotation_enabled":true,"responsive_web_graphql_skip_user_profile_image_extensions_enabled":false,"responsive_web_graphql_timeline_navigation_enabled":true,"responsive_web_enhance_cards_enabled":false}`

func (m *Module) tweetGraphQL(ctx context.Context, id string, c xCookie, queryID string) (*tweetData, error) {
	if queryID == "" {
		queryID = defaultQueryID
	}
	if strings.Trim(queryID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") != "" {
		return nil, errors.New("queryId 里有不能用的字符")
	}
	vars, _ := json.Marshal(map[string]any{"tweetId": id, "withCommunity": false, "includePromotedContent": false, "withVoice": false})
	q := url.Values{
		"variables":    {string(vars)},
		"features":     {graphqlFeatures},
		"fieldToggles": {`{"withArticleRichContentState":true,"withArticlePlainText":false,"withGrokAnalyze":false,"withDisallowedReplyControls":false}`},
	}
	target := m.xURL("graphql") + "/" + queryID + "/TweetResultByRestId?" + q.Encode()
	if err := m.cookieHostOK(target); err != nil {
		return nil, err
	}
	head := http.Header{}
	head.Set("Authorization", "Bearer "+webBearer)
	head.Set("X-Csrf-Token", c.CT0)
	head.Set("Cookie", "auth_token="+c.AuthToken+"; ct0="+c.CT0)
	head.Set("X-Twitter-Auth-Type", "OAuth2Session")
	head.Set("X-Twitter-Active-User", "yes")
	head.Set("X-Twitter-Client-Language", "en")
	body, err := m.xGet(ctx, target, head)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, errors.New("返回的内容读不懂")
	}
	if errs, ok := doc["errors"].([]any); ok && len(errs) > 0 && dig(doc, "data", "tweetResult", "result") == nil {
		msg, _ := dig(errs[0], "message").(string)
		return nil, fmt.Errorf("X 返回了错误：%s", clipRunes(msg, 120))
	}
	res, _ := dig(doc, "data", "tweetResult", "result").(map[string]any)
	d, err := fromGraphQL(res)
	if err != nil {
		return nil, err
	}
	d.Via = "cookie"
	return d, nil
}

// dig walks nested maps. A missing key gives nil.
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

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func fromGraphQL(res map[string]any) (*tweetData, error) {
	if res == nil {
		return nil, errTweetGone
	}
	if t := str(res["__typename"]); t == "TweetWithVisibilityResults" {
		inner, _ := res["tweet"].(map[string]any)
		return fromGraphQL(inner)
	} else if t != "" && t != "Tweet" {
		return nil, errTweetGone
	}
	legacy, _ := res["legacy"].(map[string]any)
	if legacy == nil {
		return nil, errTweetGone
	}
	d := &tweetData{
		ID: str(res["rest_id"]), Likes: num(legacy["favorite_count"]), Replies: num(legacy["reply_count"]), Reposts: num(legacy["retweet_count"]),
	}
	d.CreatedAt, _ = time.Parse(time.RubyDate, str(legacy["created_at"]))
	user := dig(res, "core", "user_results", "result")
	d.Name = str(dig(user, "core", "name"))
	d.Handle = str(dig(user, "core", "screen_name"))
	d.Avatar = str(dig(user, "avatar", "image_url"))
	if d.Name == "" {
		d.Name = str(dig(user, "legacy", "name"))
		d.Handle = str(dig(user, "legacy", "screen_name"))
		d.Avatar = str(dig(user, "legacy", "profile_image_url_https"))
	}

	text := str(legacy["full_text"])
	urls, _ := dig(legacy, "entities", "urls").([]any)
	if long := str(dig(res, "note_tweet", "note_tweet_results", "result", "text")); long != "" {
		// a long post: the text is whole here, and the links are in another list
		text = long
		urls, _ = dig(res, "note_tweet", "note_tweet_results", "result", "entity_set", "urls").([]any)
	} else if r, ok := legacy["display_text_range"].([]any); ok && len(r) == 2 {
		runes := []rune(text)
		if a, b := num(r[0]), num(r[1]); a >= 0 && b <= len(runes) && a <= b {
			text = string(runes[a:b])
		}
	}
	for _, u := range urls {
		if short, full := str(dig(u, "url")), str(dig(u, "expanded_url")); short != "" && full != "" {
			text = strings.ReplaceAll(text, short, full)
		}
	}
	media, _ := dig(legacy, "extended_entities", "media").([]any)
	if len(media) == 0 {
		media, _ = dig(legacy, "entities", "media").([]any)
	}
	for _, md := range media {
		if short := str(dig(md, "url")); short != "" {
			text = strings.ReplaceAll(text, short, "")
		}
		d.Media = append(d.Media, mediaFrom(str(dig(md, "type")), str(dig(md, "media_url_https")), str(dig(md, "ext_alt_text"))))
	}
	d.Text = strings.TrimSpace(html.UnescapeString(text))
	if q, ok := dig(res, "quoted_status_result", "result").(map[string]any); ok {
		if qd, err := fromGraphQL(q); err == nil {
			d.Quote = qd
		}
	}
	return d, nil
}

// ---------- step 3: the third party converter ----------

type fxMedia struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	Thumbnail string `json:"thumbnail_url"`
	Alt       string `json:"altText"`
}

type fxTweet struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Likes  int    `json:"likes"`
	Reply  int    `json:"replies"`
	Repost int    `json:"retweets"`
	Time   int64  `json:"created_timestamp"`
	Author struct {
		Name   string `json:"name"`
		Handle string `json:"screen_name"`
		Avatar string `json:"avatar_url"`
	} `json:"author"`
	Media struct {
		All []fxMedia `json:"all"`
	} `json:"media"`
	Quote *fxTweet `json:"quote"`
}

func (m *Module) tweetFx(ctx context.Context, handle, id string) (*tweetData, error) {
	if handle == "" {
		handle = "i"
	}
	body, err := m.xGet(ctx, m.xURL("fxtwitter")+"/"+handle+"/status/"+id, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tweet *fxTweet `json:"tweet"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Tweet == nil {
		return nil, errors.New("返回的内容读不懂")
	}
	d := out.Tweet.toData()
	d.Via = "fxtwitter"
	return d, nil
}

func (t *fxTweet) toData() *tweetData {
	d := &tweetData{ID: t.ID, Text: strings.TrimSpace(t.Text), Handle: t.Author.Handle, Name: t.Author.Name, Avatar: t.Author.Avatar, Likes: t.Likes, Replies: t.Reply, Reposts: t.Repost}
	if t.Time > 0 {
		d.CreatedAt = time.Unix(t.Time, 0).UTC()
	}
	for _, md := range t.Media.All {
		pic := md.URL
		if md.Type == "video" || md.Type == "gif" {
			pic = md.Thumbnail
		}
		if pic != "" {
			d.Media = append(d.Media, mediaFrom(md.Type, pic, md.Alt))
		}
	}
	if t.Quote != nil {
		d.Quote = t.Quote.toData()
	}
	return d
}

// ---------- calling X ----------

func (m *Module) xURL(which string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v := m.xBases[which]; v != "" {
		return v
	}
	switch which {
	case "syndication":
		return syndicationBase
	case "graphql":
		return graphqlBase
	}
	return fxtwitterBase
}

// cookieHostOK makes sure the cookie only goes to X.
func (m *Module) cookieHostOK(target string) error {
	if m.allowPrivate {
		return nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return errors.New("网址不对")
	}
	if h := strings.ToLower(u.Hostname()); h != "x.com" && h != "api.x.com" {
		return errors.New("Cookie 只能发给 x.com")
	}
	return nil
}

// xGet reads one answer from X. It never follows a redirect, so a header with
// the cookie cannot be sent anywhere else.
func (m *Module) xGet(ctx context.Context, target string, head http.Header) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, errors.New("网址不对")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	for k, v := range head {
		req.Header[k] = v
	}
	m.mu.Lock()
	client := m.xclient
	m.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		// the error text of net/http holds the address with the query, keep it out
		return nil, errors.New(fetchError(err))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || (resp.StatusCode == http.StatusForbidden && head.Get("Cookie") != ""):
		return nil, errXAuth
	case resp.StatusCode == http.StatusNotFound:
		return nil, errTweetGone
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("X 限制了请求次数，稍后再试")
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("状态码 %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxTweetBody))
}

// newXClient is the client for X. It refuses internal addresses like the page
// client does and does not follow redirects.
func newXClient(allowPrivate bool) *http.Client {
	c := newFetchClient(allowPrivate)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// ---------- putting it together ----------

// fetchTweet reads one post and turns it into a page to archive.
func (m *Module) fetchTweet(ctx context.Context, handle, id string) (page, error) {
	var why []string
	var best *tweetData
	take := func(d *tweetData, err error, step string) {
		if err != nil {
			why = append(why, step+"："+err.Error())
			return
		}
		if best == nil || (best.Incomplete && !d.Incomplete) {
			best = d
		}
	}
	d, err := m.tweetSyndication(ctx, id)
	take(d, err, "公开接口")
	cfg := m.xConfig(ctx)
	if best == nil || best.Incomplete {
		if cookie, ok := m.xCookie(ctx); ok {
			d, err := m.tweetGraphQL(ctx, id, cookie, cfg.QueryID)
			if errors.Is(err, errXAuth) {
				m.xExpired(ctx, cfg)
			}
			take(d, err, "登录 Cookie")
		}
	}
	if (best == nil || best.Incomplete) && cfg.Fxtwitter {
		d, err := m.tweetFx(ctx, handle, id)
		take(d, err, "第三方转换")
	}
	if best == nil {
		hint := ""
		if _, ok := m.xCookie(ctx); !ok {
			hint = "。可以在设置里填 X 登录 Cookie"
		}
		return page{}, errors.New(strings.Join(why, "；") + hint)
	}
	if best.Handle == "" {
		best.Handle = handle
	}
	return m.tweetPage(best, canonicalTweetURL(best.Handle, id)), nil
}

// tweetPage renders a post as a page: the HTML for reading, the plain text
// for searching, and the details for the list.
func (m *Module) tweetPage(d *tweetData, link string) page {
	loc := m.location()
	var text, h strings.Builder
	h.WriteString(`<div class="xc-tweet">`)
	writeTweet(&h, &text, d, link, loc)
	h.WriteString(`<p class="xc-tweet-link"><a href="` + html.EscapeString(link) + `">在 X 上查看原文</a></p></div>`)
	title := strings.Join(strings.Fields(d.Text), " ")
	if title == "" {
		title = "推文"
	}
	if d.Name != "" {
		title = d.Name + "：" + title
	}
	title = clipRunes(title, maxTitle)
	meta := map[string]any{"via": d.Via, "handle": d.Handle, "name": d.Name, "likes": d.Likes, "replies": d.Replies, "reposts": d.Reposts}
	if !d.CreatedAt.IsZero() {
		meta["postedAt"] = d.CreatedAt.UTC().Format(time.RFC3339)
	}
	if d.Incomplete {
		meta["incomplete"] = true
	}
	return page{
		Title: title, Site: "X", Excerpt: clipRunes(strings.Join(strings.Fields(d.Text), " "), 500),
		Content: clipRunes(tidyText(text.String()), maxContent), HTML: h.String(), Kind: "tweet", Meta: meta,
	}
}

func (m *Module) location() *time.Location {
	if m.d.Scheduler != nil {
		if loc := m.d.Scheduler.Location(); loc != nil {
			return loc
		}
	}
	return time.Local
}

func writeTweet(h, plain *strings.Builder, d *tweetData, link string, loc *time.Location) {
	h.WriteString(`<div class="xc-tweet-head">`)
	if d.Avatar != "" {
		h.WriteString(`<img class="xc-avatar" alt="" src="` + html.EscapeString(d.Avatar) + `">`)
	}
	h.WriteString(`<span class="xc-tweet-who"><strong>` + html.EscapeString(d.Name) + `</strong>`)
	if d.Handle != "" {
		h.WriteString(` <span class="xc-tweet-handle">@` + html.EscapeString(d.Handle) + `</span>`)
	}
	h.WriteString(`</span>`)
	if !d.CreatedAt.IsZero() {
		h.WriteString(` <span class="xc-tweet-time">` + d.CreatedAt.In(loc).Format("2006-01-02 15:04") + `</span>`)
	}
	h.WriteString(`</div>`)
	if d.Name != "" || d.Handle != "" {
		fmt.Fprintf(plain, "%s @%s\n", d.Name, d.Handle)
	}
	plain.WriteString(d.Text + "\n")
	h.WriteString(`<p class="xc-tweet-text">` + linkify(d.Text) + `</p>`)
	for _, md := range d.Media {
		if md.URL == "" {
			continue
		}
		src := md.URL
		if md.Kind == "photo" && strings.Contains(src, "pbs.twimg.com/media/") && !strings.Contains(src, "name=") {
			src += "?name=orig"
		}
		h.WriteString(`<figure class="xc-tweet-media"><img alt="` + html.EscapeString(md.Alt) + `" src="` + html.EscapeString(src) + `">`)
		switch md.Kind {
		case "video":
			h.WriteString(`<figcaption>视频，只存了封面，<a href="` + html.EscapeString(link) + `">在 X 上看</a></figcaption>`)
		case "gif":
			h.WriteString(`<figcaption>动图，只存了封面，<a href="` + html.EscapeString(link) + `">在 X 上看</a></figcaption>`)
		}
		h.WriteString(`</figure>`)
		if md.Alt != "" {
			plain.WriteString(md.Alt + "\n")
		}
	}
	if d.Quote != nil {
		h.WriteString(`<blockquote class="xc-tweet-quote">`)
		plain.WriteString("\n引用：")
		writeTweet(h, plain, d.Quote, canonicalTweetURL(d.Quote.Handle, d.Quote.ID), loc)
		h.WriteString(`</blockquote>`)
	}
}

// linkify escapes a text and turns its addresses into links.
func linkify(s string) string {
	var b strings.Builder
	last := 0
	for _, loc := range linkRe.FindAllStringIndex(s, -1) {
		raw := strings.TrimRight(s[loc[0]:loc[1]], ".,;:!?)）。，；：！？”")
		end := loc[0] + len(raw)
		b.WriteString(escapeLines(s[last:loc[0]]))
		b.WriteString(`<a href="` + html.EscapeString(raw) + `">` + html.EscapeString(raw) + `</a>`)
		last = end
	}
	b.WriteString(escapeLines(s[last:]))
	return b.String()
}

func escapeLines(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}
