package readlater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"syscall"
	"time"

	readability "github.com/go-shiori/go-readability"
	"golang.org/x/net/html/charset"

	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater/db"
)

const (
	fetchTimeout = 15 * time.Second
	maxBody      = 5 << 20
	maxContent   = 100000
	maxRedirects = 5
	userAgent    = "X-Console-ReadLater/1.0 (personal read-later archive)"
)

// errBlocked is what the dialer says for an address the server must not call.
var errBlocked = errors.New("不能访问这个地址")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// blockedAddr is true for addresses that are not on the public internet:
// loopback, private, link local (cloud metadata is 169.254.169.254),
// carrier-grade NAT, multicast and unspecified.
func blockedAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	return !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip)
}

// newFetchClient builds the client that reads saved pages. The check runs
// when a connection is made, on the address the name resolved to, so a
// redirect or a DNS answer cannot lead to an internal address. allowPrivate is
// for tests that talk to a local server.
func newFetchClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errBlocked
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || blockedAddr(ip) {
				return errBlocked
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: fetchTimeout,
		Transport: &http.Transport{
			// No proxy: the address check above only sees the next hop.
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("跳转太多次")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errBlocked
			}
			return nil
		},
	}
}

// page is what was read from one address.
type page struct {
	Title   string
	Site    string
	Excerpt string
	Content string
	// HTML is the page for reading, not cleaned yet (archive.go does that).
	HTML string
	// Base resolves the relative addresses in HTML.
	Base *url.URL
	Kind string
	Meta map[string]any
}

// fetchPage downloads an address and pulls out the title and the text.
func (m *Module) fetchPage(ctx context.Context, rawURL string) (page, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return page{}, errors.New("网址不对")
	}
	if handle, id, ok := parseTweetURL(u); ok {
		pg, err := m.fetchTweet(ctx, handle, id)
		if err == nil {
			pg.Base = u
		}
		return pg, err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return page{}, errors.New("网址不对")
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,text/plain;q=0.5,*/*;q=0.3")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return page{}, errors.New(fetchError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return page{}, fmt.Errorf("网页打不开，状态码 %d", resp.StatusCode)
	}
	host := hostOf(rawURL)
	kind := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(kind, "html") || kind == "":
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			return page{}, errors.New(fetchError(err))
		}
		return extract(body, resp.Header.Get("Content-Type"), resp.Request.URL, host), nil
	case strings.HasPrefix(kind, "text/"):
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			return page{}, errors.New(fetchError(err))
		}
		text := tidyText(string(body))
		return page{Title: fileTitle(u, host), Site: host, Content: clipRunes(text, maxContent)}, nil
	default:
		// PDF, pictures and the like: only the address and the file name are kept
		return page{Title: fileTitle(u, host), Site: host}, nil
	}
}

func fileTitle(u *url.URL, host string) string {
	if name := path.Base(u.Path); name != "" && name != "." && name != "/" {
		return name
	}
	return host
}

// fetchError says in a few words why a download failed.
func fetchError(err error) string {
	switch {
	case errors.Is(err, errBlocked):
		return "不能访问这个地址"
	case errors.Is(err, context.DeadlineExceeded):
		return "网页太慢，超时了"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "网页太慢，超时了"
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "找不到这个网站"
	}
	if strings.Contains(err.Error(), "不能访问这个地址") {
		return "不能访问这个地址"
	}
	return "网页打不开"
}

// extract reads the page with go-readability. A page it cannot read still
// gets a title from its address.
func extract(body []byte, contentType string, pageURL *url.URL, host string) page {
	reader, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		reader = bytes.NewReader(body)
	}
	article, err := readability.FromReader(reader, pageURL)
	if err != nil {
		return page{Title: host, Site: host}
	}
	p := page{
		Title:   clipRunes(strings.Join(strings.Fields(article.Title), " "), maxTitle),
		Site:    strings.TrimSpace(article.SiteName),
		Excerpt: clipRunes(strings.Join(strings.Fields(article.Excerpt), " "), 500),
		Content: clipRunes(tidyText(article.TextContent), maxContent),
		HTML:    article.Content,
		Base:    pageURL,
	}
	if p.Site == "" {
		p.Site = host
	}
	return p
}

// tidyText trims every line and keeps at most one empty line in a row.
func tidyText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	blank := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			blank++
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
			if blank > 0 {
				b.WriteString("\n")
			}
		}
		blank = 0
		b.WriteString(line)
	}
	return b.String()
}

// enqueue starts fetching one item in the background.
func (m *Module) enqueue(id int64) {
	ctx := m.background()
	go func() {
		select {
		case m.sem <- struct{}{}:
			defer func() { <-m.sem }()
		case <-ctx.Done():
			return
		}
		m.process(ctx, id)
	}()
}

// process fetches the page of one item and asks the AI about it.
func (m *Module) process(ctx context.Context, id int64) {
	row, err := m.q.GetReadItem(ctx, id)
	if err != nil {
		return
	}
	pg, err := m.fetchPage(ctx, row.Url)
	now := m.now().UTC()
	if err != nil {
		if e := m.q.SaveReadFailure(ctx, db.SaveReadFailureParams{Error: err.Error(), UpdatedAt: now, ID: id}); e != nil {
			m.d.Log.Warn("readlater save failure", "id", id, "err", e)
		}
		m.publishRow("readlater.updated", id)
		return
	}
	var archived string
	var kept map[string]bool
	if pg.HTML != "" {
		base := pg.Base
		if base == nil {
			base, _ = url.Parse(row.Url)
		}
		actx, cancel := context.WithTimeout(ctx, archiveTimeout)
		archived, kept = m.archive(actx, id, base, pg.HTML)
		cancel()
		if archived == "" {
			kept = nil
		}
	}
	kind := pg.Kind
	if kind == "" {
		kind = string(api.Page)
	}
	meta := "{}"
	if len(pg.Meta) > 0 {
		if raw, err := json.Marshal(pg.Meta); err == nil {
			meta = string(raw)
		}
	}
	err = m.q.SaveReadFetched(ctx, db.SaveReadFetchedParams{Title: pg.Title, Site: pg.Site, Excerpt: pg.Excerpt, Content: pg.Content, ContentHtml: archived, Kind: kind, MetaJson: meta, At: &now, ID: id})
	if err != nil {
		m.d.Log.Warn("readlater save page", "id", id, "err", err)
		return
	}
	m.dropAssets(ctx, id, kept)
	m.publishRow("readlater.updated", id)
	if fresh, err := m.q.GetReadItem(ctx, id); err == nil && fresh.Summary == "" && fresh.Content != "" {
		if _, err := m.summarize(ctx, fresh); err == nil {
			m.publishRow("readlater.updated", id)
		}
	}
}

// retryStale queues the items that were never finished, for example because
// the server restarted while they were waiting.
func (m *Module) retryStale(ctx context.Context) error {
	ids, err := m.q.ListStaleQueuedReadItems(ctx, m.now().UTC().Add(-time.Minute))
	if err != nil {
		return err
	}
	for _, id := range ids {
		m.enqueue(id)
	}
	return nil
}
