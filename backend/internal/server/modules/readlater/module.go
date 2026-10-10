// Package readlater keeps the links the user saves to read later (B117). It
// fetches each page in the background, stores the text, asks the AI for a short
// summary and tags, and pushes the unread list once a week. See
// docs/specs/B117.md.
package readlater

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/db"
)

const (
	maxURL       = 2000
	maxTitle     = 300
	maxNote      = 2000
	maxTags      = 8
	maxTagRunes  = 20
	maxFetchTry  = 3
	fetchWorkers = 3
	topTags      = 30
)

// ServiceKey is where the module registers itself, for tests.
const ServiceKey = "readlater.module"

// Module implements api.ServerInterface.
type Module struct {
	d     *module.Deps
	q     *db.Queries
	nowFn func() time.Time

	mu     sync.Mutex
	ctx    context.Context
	client *http.Client
	sem    chan struct{}
	// allowPrivate lets tests save addresses on this machine.
	allowPrivate bool
}

var (
	_ api.ServerInterface = (*Module)(nil)
	_ module.Starter      = (*Module)(nil)
)

// New builds the module.
func New(d *module.Deps) (module.Module, error) {
	m := &Module{
		d: d, q: db.New(d.DB), nowFn: time.Now, ctx: context.Background(),
		client: newFetchClient(false), sem: make(chan struct{}, fetchWorkers),
	}
	m.registerActions()
	module.Provide[*Module](d.Registry, ServiceKey, m)
	module.Provide[contracts.TelegramInbox](d.Registry, contracts.TelegramInboxKey, m)
	return m, nil
}

// Name implements module.Module.
func (m *Module) Name() string { return "readlater" }

// Mount implements module.Module.
func (m *Module) Mount(r chi.Router) {
	api.HandlerWithOptions(m, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: httpx.BadParam})
}

// Start schedules the retry of stuck fetches and the weekly digest.
func (m *Module) Start(ctx context.Context) error {
	m.mu.Lock()
	m.ctx = ctx
	m.mu.Unlock()
	m.d.Scheduler.Every("readlater.retry", time.Minute, m.retryStale)
	m.d.Scheduler.Every("readlater.digest", time.Hour, m.digest)
	return nil
}

func (m *Module) now() time.Time { return m.nowFn() }

func (m *Module) background() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ctx
}

// ---------- urls and tags ----------

// trackingParams are dropped from saved addresses so one page is one entry.
var trackingParams = map[string]bool{"fbclid": true, "gclid": true, "igshid": true, "mc_cid": true, "mc_eid": true, "spm": true}

// normalizeURL checks and cleans an address: http or https only, no fragment,
// no login in it, no tracking parameters.
func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", httpx.Invalid("请填写网址")
	}
	if utf8.RuneCountInString(raw) > maxURL {
		return "", httpx.Invalid("网址太长，最多 2000 个字符")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", httpx.Invalid("只能存 http 或 https 开头的网址")
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.User = nil
	if u.RawQuery != "" {
		// keep the order and the encoding of what stays
		var keep []string
		for _, part := range strings.Split(u.RawQuery, "&") {
			key, _, _ := strings.Cut(part, "=")
			if k, err := url.QueryUnescape(key); err == nil {
				key = k
			}
			key = strings.ToLower(key)
			if part == "" || strings.HasPrefix(key, "utm_") || trackingParams[key] {
				continue
			}
			keep = append(keep, part)
		}
		u.RawQuery = strings.Join(keep, "&")
	}
	return u.String(), nil
}

// normalizeTags trims, drops empty and repeated tags and caps the number.
func normalizeTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if t == "" {
			continue
		}
		if r := []rune(t); len(r) > maxTagRunes {
			t = string(r[:maxTagRunes])
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
		if len(out) == maxTags {
			break
		}
	}
	return out
}

func parseTags(raw string) []string {
	var tags []string
	_ = json.Unmarshal([]byte(raw), &tags)
	if tags == nil {
		return []string{}
	}
	return tags
}

func formatTags(tags []string) string {
	raw, _ := json.Marshal(normalizeTags(tags))
	return string(raw)
}

func clipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// ---------- view ----------

func toView(row db.ReadItem, withContent bool) api.ReadItem {
	v := api.ReadItem{
		Id: row.ID, Url: row.Url, Title: row.Title, Site: row.Site, Excerpt: row.Excerpt, Summary: row.Summary,
		Tags: parseTags(row.TagsJson), Note: row.Note, Source: api.ReadItemSource(row.Source), Status: api.ReadStatus(row.Status),
		Error: row.Error, HasContent: row.Content != "", Read: row.ReadAt != nil, ReadAt: row.ReadAt,
		CreatedAt: row.CreatedAt, FetchedAt: row.FetchedAt, UpdatedAt: row.UpdatedAt,
	}
	if withContent {
		c := row.Content
		v.Content = &c
	}
	return v
}

func toListView(row db.ListReadItemsRow) api.ReadItem {
	return api.ReadItem{
		Id: row.ID, Url: row.Url, Title: row.Title, Site: row.Site, Excerpt: row.Excerpt, Summary: row.Summary,
		Tags: parseTags(row.TagsJson), Note: row.Note, Source: api.ReadItemSource(row.Source), Status: api.ReadStatus(row.Status),
		Error: row.Error, HasContent: row.HasContent != 0, Read: row.ReadAt != nil, ReadAt: row.ReadAt,
		CreatedAt: row.CreatedAt, FetchedAt: row.FetchedAt, UpdatedAt: row.UpdatedAt,
	}
}

func (m *Module) publish(topic string, v api.ReadItem) {
	v.Content = nil
	m.d.Bus.Publish(topic, v)
}

func (m *Module) publishRow(topic string, id int64) {
	row, err := m.q.GetReadItem(m.background(), id)
	if err != nil {
		return
	}
	m.publish(topic, toView(row, false))
}

// ---------- list ----------

// likePattern turns a search word into a LIKE pattern.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// matching returns the ids of items whose text holds q.
func (m *Module) matching(ctx context.Context, q string) (map[int64]bool, error) {
	p := likePattern(q)
	rows, err := m.d.DB.QueryContext(ctx,
		`SELECT id FROM read_items WHERE title LIKE ?1 ESCAPE '\' OR url LIKE ?1 ESCAPE '\' OR summary LIKE ?1 ESCAPE '\' OR note LIKE ?1 ESCAPE '\' OR content LIKE ?1 ESCAPE '\'`, p)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func (m *Module) list(ctx context.Context, view, tag, q string) (api.ReadList, error) {
	rows, err := m.q.ListReadItems(ctx)
	if err != nil {
		return api.ReadList{}, err
	}
	var hits map[int64]bool
	if q = strings.TrimSpace(q); q != "" {
		if hits, err = m.matching(ctx, q); err != nil {
			return api.ReadList{}, err
		}
	}
	out := api.ReadList{Items: []api.ReadItem{}, Tags: []api.ReadTag{}}
	counts := map[string]int{}
	for _, row := range rows {
		out.Counts.All++
		if row.ReadAt == nil {
			out.Counts.Unread++
		} else {
			out.Counts.Read++
		}
		if row.Status == string(api.Failed) {
			out.Counts.Failed++
		}
		tags := parseTags(row.TagsJson)
		for _, t := range tags {
			counts[t]++
		}
		if view == "unread" && row.ReadAt != nil || view == "read" && row.ReadAt == nil {
			continue
		}
		if tag != "" && !hasTag(tags, tag) {
			continue
		}
		if hits != nil && !hits[row.ID] {
			continue
		}
		out.Items = append(out.Items, toListView(row))
	}
	out.Tags = rankTags(counts)
	return out, nil
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func rankTags(counts map[string]int) []api.ReadTag {
	list := make([]api.ReadTag, 0, len(counts))
	for t, n := range counts {
		list = append(list, api.ReadTag{Tag: t, Count: n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Tag < list[j].Tag
	})
	if len(list) > topTags {
		list = list[:topTags]
	}
	return list
}

// topTagNames are handed to the AI so it reuses them.
func (m *Module) topTagNames(ctx context.Context) []string {
	rows, err := m.q.ListReadItems(ctx)
	if err != nil {
		return nil
	}
	counts := map[string]int{}
	for _, row := range rows {
		for _, t := range parseTags(row.TagsJson) {
			counts[t]++
		}
	}
	out := []string{}
	for _, t := range rankTags(counts) {
		out = append(out, t.Tag)
	}
	return out
}

// ---------- create ----------

// add saves a link. A link that is already saved is returned as it is.
func (m *Module) add(ctx context.Context, rawURL, note, source string) (db.ReadItem, bool, error) {
	clean, err := normalizeURL(rawURL)
	if err == nil {
		err = m.checkHost(clean)
	}
	if err != nil {
		return db.ReadItem{}, false, err
	}
	if utf8.RuneCountInString(note) > maxNote {
		return db.ReadItem{}, false, httpx.Invalid("备注最多 2000 个字")
	}
	if old, err := m.q.GetReadItemByURL(ctx, clean); err == nil {
		return old, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.ReadItem{}, false, err
	}
	host := hostOf(clean)
	now := m.now().UTC()
	row, err := m.q.InsertReadItem(ctx, db.InsertReadItemParams{Url: clean, Title: host, Site: host, Note: strings.TrimSpace(note), Source: source, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		// two requests saved the same address at once
		if old, err2 := m.q.GetReadItemByURL(ctx, clean); err2 == nil {
			return old, true, nil
		}
		return db.ReadItem{}, false, err
	}
	m.enqueue(row.ID)
	return row, false, nil
}

// checkHost refuses addresses that are plainly on this machine or a private
// network, so the user hears about it at once. The connection itself is
// checked again when it is made (fetch.go), which also covers names that
// resolve to such addresses.
func (m *Module) checkHost(raw string) error {
	if m.allowPrivate {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return httpx.Invalid("网址不对")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return httpx.Invalid("不能存本机或内网的地址")
	}
	if ip, err := netip.ParseAddr(host); err == nil && blockedAddr(ip) {
		return httpx.Invalid("不能存本机或内网的地址")
	}
	return nil
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

// ---------- handlers ----------

// ListReadItems implements api.ServerInterface.
func (m *Module) ListReadItems(w http.ResponseWriter, r *http.Request, params api.ListReadItemsParams) {
	view := "unread"
	if params.View != nil {
		view = string(*params.View)
	}
	var tag, q string
	if params.Tag != nil {
		tag = strings.TrimSpace(*params.Tag)
	}
	if params.Q != nil {
		q = *params.Q
	}
	out, err := m.list(r.Context(), view, tag, q)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// CreateReadItem implements api.ServerInterface.
func (m *Module) CreateReadItem(w http.ResponseWriter, r *http.Request) {
	var body api.CreateReadItemJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	source := "web"
	if body.Source != nil && *body.Source == api.ReadItemInputSourceShare {
		source = "share"
	}
	note := ""
	if body.Note != nil {
		note = *body.Note
	}
	ctx := r.Context()
	row, dup, err := m.add(ctx, body.Url, note, source)
	target := ""
	if err == nil {
		target = strconv.FormatInt(row.ID, 10)
	}
	m.d.Audit.Record(ctx, "readlater.create", target, map[string]any{"url": row.Url, "duplicate": dup}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := toView(row, false)
	if dup {
		httpx.JSON(w, http.StatusOK, api.ReadItemResult{Item: v, Duplicate: true})
		return
	}
	m.publish("readlater.created", v)
	httpx.JSON(w, http.StatusCreated, api.ReadItemResult{Item: v})
}

func (m *Module) get(ctx context.Context, id int64) (db.ReadItem, error) {
	row, err := m.q.GetReadItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, httpx.ErrNotFound
	}
	return row, err
}

// GetReadItem implements api.ServerInterface.
func (m *Module) GetReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	row, err := m.get(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toView(row, true))
}

// UpdateReadItem implements api.ServerInterface.
func (m *Module) UpdateReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	var body api.UpdateReadItemJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	row, err := m.get(ctx, id)
	if err == nil {
		row, err = m.patch(ctx, row, body)
	}
	m.d.Audit.Record(ctx, "readlater.update", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := toView(row, true)
	m.publish("readlater.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

func (m *Module) patch(ctx context.Context, row db.ReadItem, body api.ReadItemPatch) (db.ReadItem, error) {
	p := db.UpdateReadItemParams{Title: row.Title, TitleLocked: row.TitleLocked, TagsJson: row.TagsJson, Note: row.Note, ReadAt: row.ReadAt, ID: row.ID}
	if body.Title != nil {
		t := strings.TrimSpace(*body.Title)
		if t == "" {
			return row, httpx.Invalid("标题不能为空")
		}
		if utf8.RuneCountInString(t) > maxTitle {
			return row, httpx.Invalid("标题最多 300 个字")
		}
		p.Title, p.TitleLocked = t, 1
	}
	if body.Tags != nil {
		p.TagsJson = formatTags(*body.Tags)
	}
	if body.Note != nil {
		if utf8.RuneCountInString(*body.Note) > maxNote {
			return row, httpx.Invalid("备注最多 2000 个字")
		}
		p.Note = strings.TrimSpace(*body.Note)
	}
	if body.Read != nil {
		switch {
		case *body.Read && row.ReadAt == nil:
			now := m.now().UTC()
			p.ReadAt = &now
		case !*body.Read:
			p.ReadAt = nil
		}
	}
	p.UpdatedAt = m.now().UTC()
	return m.q.UpdateReadItem(ctx, p)
}

// DeleteReadItem implements api.ServerInterface.
func (m *Module) DeleteReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	n, err := m.q.DeleteReadItem(ctx, id)
	if err == nil && n == 0 {
		err = httpx.ErrNotFound
	}
	m.d.Audit.Record(ctx, "readlater.delete", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("readlater.deleted", map[string]any{"id": id})
	httpx.NoContent(w)
}

// RefetchReadItem implements api.ServerInterface.
func (m *Module) RefetchReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	row, err := m.get(ctx, id)
	if err == nil {
		err = m.q.ResetReadAttempts(ctx, id)
	}
	if err == nil {
		err = m.q.QueueReadItem(ctx, db.QueueReadItemParams{UpdatedAt: m.now().UTC(), ID: id})
	}
	m.d.Audit.Record(ctx, "readlater.refetch", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.enqueue(row.ID)
	row, err = m.get(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := toView(row, true)
	m.publish("readlater.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}

// SummarizeReadItem implements api.ServerInterface.
func (m *Module) SummarizeReadItem(w http.ResponseWriter, r *http.Request, id api.ItemId) {
	ctx := r.Context()
	row, err := m.get(ctx, id)
	if err == nil {
		row, err = m.summarize(ctx, row)
	}
	m.d.Audit.Record(ctx, "readlater.summarize", strconv.FormatInt(id, 10), nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	v := toView(row, true)
	m.publish("readlater.updated", v)
	httpx.JSON(w, http.StatusOK, v)
}
