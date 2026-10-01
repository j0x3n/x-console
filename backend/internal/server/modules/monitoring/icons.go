package monitoring

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/db"
)

const (
	iconPageLimit    = 512 << 10
	iconMaxBytes     = 64 << 10
	iconMaxRedirects = 3
	iconTimeout      = 5 * time.Second
	iconFreshFor     = 7 * 24 * time.Hour
)

func (m *Module) markIconBusy(id int64) bool {
	m.iconMu.Lock()
	defer m.iconMu.Unlock()
	if _, ok := m.iconBusy[id]; ok {
		return false
	}
	m.iconBusy[id] = struct{}{}
	return true
}

func (m *Module) clearIconBusy(id int64) {
	m.iconMu.Lock()
	delete(m.iconBusy, id)
	m.iconMu.Unlock()
}

func (m *Module) scheduleIcon(id int64, target string) {
	if !m.markIconBusy(id) {
		return
	}
	go func() {
		defer m.clearIconBusy(id)
		ctx, cancel := context.WithTimeout(m.background(), 20*time.Second)
		defer cancel()
		if err := m.fetchIcon(ctx, id, target); err != nil {
			m.d.Log.Warn("monitor icon", "monitor", id, "err", err)
		}
	}()
}

func (m *Module) refreshIconIfStale(ctx context.Context, id int64, target string, now time.Time) {
	at, err := m.q.GetMonitorIconTime(ctx, id)
	if err == nil && now.Sub(at) < iconFreshFor {
		return
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	m.scheduleIcon(id, target)
}

func (m *Module) fetchIcon(ctx context.Context, id int64, target string) error {
	home, err := iconHome(target)
	if err != nil {
		return err
	}
	client := m.iconClient()
	var iconURL *url.URL
	if body, final, err := m.iconGET(ctx, client, home.String(), iconPageLimit); err == nil {
		iconURL = bestIconURL(final, body)
	}
	if iconURL == nil {
		fav := *home
		fav.Path = "/favicon.ico"
		iconURL = &fav
	}
	data, mime, err := m.downloadIcon(ctx, client, iconURL.String())
	if err != nil {
		return err
	}
	return m.q.UpsertMonitorIcon(ctx, db.UpsertMonitorIconParams{
		MonitorID: id, Mime: mime, Data: data, FetchedAt: m.now(),
	})
}

func (m *Module) iconClient() *http.Client {
	return &http.Client{
		Transport: m.transport(),
		Timeout:   iconTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= iconMaxRedirects+1 {
				return errors.New("跳转次数太多")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("只跟随 http 和 https")
			}
			return nil
		},
	}
}

func (m *Module) downloadIcon(ctx context.Context, client *http.Client, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "x-console-monitor/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("状态码是 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, iconMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > iconMaxBytes {
		return nil, "", errors.New("站标超过 64 KB")
	}
	mime := iconMIME(resp.Header.Get("Content-Type"), data)
	if mime == "" || len(data) == 0 {
		return nil, "", errors.New("站标类型不对")
	}
	return data, mime, nil
}

func (m *Module) iconGET(ctx context.Context, client *http.Client, rawURL string, limit int64) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "x-console-monitor/1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("状态码是 %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, nil, err
	}
	final := resp.Request.URL
	return data, final, nil
}

func iconHome(target string) (*url.URL, error) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("网址不对")
	}
	u.User = nil
	u.Path = "/"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func iconMIME(contentType string, data []byte) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "image/png", "image/x-icon", "image/vnd.microsoft.icon", "image/jpeg", "image/gif", "image/webp", "image/svg+xml":
		return ct
	}
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0 {
		return "image/x-icon"
	}
	return ""
}

type iconLink struct {
	href  string
	apple bool
	size  int
}

var iconSizePattern = regexp.MustCompile(`(?i)(\d+)\s*x\s*(\d+)`)

func iconSize(sizes string) int {
	best := 0
	for _, m := range iconSizePattern.FindAllStringSubmatch(sizes, -1) {
		w, _ := strconv.Atoi(m[1])
		h, _ := strconv.Atoi(m[2])
		if w > best {
			best = w
		}
		if h > best {
			best = h
		}
	}
	return best
}

func iconLinks(body []byte) []iconLink {
	z := html.NewTokenizer(bytes.NewReader(body))
	var out []iconLink
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return out
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			continue
		}
		name, hasAttr := z.TagName()
		if !hasAttr || !bytes.EqualFold(name, []byte("link")) {
			continue
		}
		var rel, href, sizes string
		for {
			k, v, more := z.TagAttr()
			switch strings.ToLower(string(k)) {
			case "rel":
				rel = string(v)
			case "href":
				href = string(v)
			case "sizes":
				sizes = string(v)
			}
			if !more {
				break
			}
		}
		relLower := strings.ToLower(rel)
		if href == "" || !strings.Contains(relLower, "icon") {
			continue
		}
		out = append(out, iconLink{href: href, apple: strings.Contains(relLower, "apple-touch-icon"), size: iconSize(sizes)})
	}
}

func iconBetter(a, b iconLink) bool {
	if a.apple != b.apple {
		return a.apple
	}
	if a.size != b.size {
		return a.size > b.size
	}
	return false
}

func bestIconURL(base *url.URL, body []byte) *url.URL {
	if base == nil {
		return nil
	}
	var best *iconLink
	var bestURL *url.URL
	for _, link := range iconLinks(body) {
		u := resolveIcon(base, link.href)
		if u == nil {
			continue
		}
		if best == nil || iconBetter(link, *best) {
			cp := link
			best = &cp
			bestURL = u
		}
	}
	return bestURL
}

func resolveIcon(base *url.URL, href string) *url.URL {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return nil
	}
	abs := base.ResolveReference(ref)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return nil
	}
	return abs
}

func (m *Module) GetMonitorIcon(w http.ResponseWriter, r *http.Request, monitorID int64) {
	row, err := m.q.GetMonitorIcon(r.Context(), monitorID)
	if errors.Is(err, sql.ErrNoRows) {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", row.Mime)
	h.Set("Cache-Control", "private, max-age=604800")
	h.Set("X-Content-Type-Options", "nosniff")
	if row.Mime == "image/svg+xml" {
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(row.Data)
}
