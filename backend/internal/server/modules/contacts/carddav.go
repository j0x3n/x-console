package contacts

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/contacts/api"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

/*
 * iCloud 通讯录同步。只读 CardDAV：登录 → 找到当前用户 → 找到通讯录目录 →
 * 列出通讯录 → 一次取回全部 vCard。不改 iCloud 上的任何东西。
 * 自己写这几个请求，是因为 iCloud 返回的通讯录目录在另一台主机（pNN-contacts.icloud.com），
 * 现成的客户端库只保留路径、会丢掉主机名。
 */

const (
	icloudServer = "https://contacts.icloud.com"
	keySync      = "contacts.sync"       // 账号，加密存储
	keySyncState = "contacts.sync_state" // 上次同步的结果
	syncEvery    = 6 * time.Hour
	maxResponse  = 64 << 20
	maxRedirects = 5
)

var errNotConfigured = httpx.NewError(http.StatusConflict, "sync_not_configured", "还没设置通讯录同步")

type syncConfig struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Server        string `json:"server"`
	OnlyWithDates bool   `json:"onlyWithDates"`
}

type syncState struct {
	LastSyncAt *time.Time `json:"lastSyncAt,omitempty"`
	LastError  string     `json:"lastError"`
	Total      int        `json:"total"`
	Created    int        `json:"created"`
	Updated    int        `json:"updated"`
}

// ---------- CardDAV ----------

type multistatus struct {
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string        `xml:"href"`
	Propstat []davPropstat `xml:"propstat"`
}

type davPropstat struct {
	Status string  `xml:"status"`
	Prop   davProp `xml:"prop"`
}

type davProp struct {
	Principal struct {
		Href string `xml:"href"`
	} `xml:"current-user-principal"`
	HomeSet struct {
		Href string `xml:"href"`
	} `xml:"addressbook-home-set"`
	ResourceType struct {
		AddressBook *struct{} `xml:"addressbook"`
	} `xml:"resourcetype"`
	AddressData string `xml:"address-data"`
}

// props returns the properties the server found (status 200).
func (r davResponse) props() []davProp {
	var out []davProp
	for _, ps := range r.Propstat {
		if strings.Contains(ps.Status, " 200") {
			out = append(out, ps.Prop)
		}
	}
	return out
}

type davClient struct {
	http     *http.Client
	username string
	password string
	// origin is the address the user gave. iCloud answers with the address of
	// the server that holds the account (pNN-contacts.icloud.com), a name that
	// does not resolve on every network. Only the path of an answer is used,
	// and every request goes to origin, which passes it on.
	origin string
}

// do sends one request. Redirects are followed by hand, so the method, the body
// and the login are kept.
func (c *davClient) do(ctx context.Context, method, target, depth, body string) (*multistatus, error) {
	for i := 0; i <= maxRedirects; i++ {
		u, err := url.Parse(target)
		if err != nil || u.Scheme != "https" && !allowInsecure || u.Host == "" {
			return nil, errors.New("服务器地址要以 https:// 开头")
		}
		req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(c.username, c.password)
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req.Header.Set("Depth", depth)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("连不上通讯录服务器：%w", err)
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
		resp.Body.Close()
		if rerr != nil {
			return nil, rerr
		}
		switch {
		case resp.StatusCode == http.StatusMultiStatus:
			var ms multistatus
			if err := xml.Unmarshal(raw, &ms); err != nil {
				return nil, fmt.Errorf("通讯录服务器返回的内容看不懂：%w", err)
			}
			return &ms, nil
		case resp.StatusCode >= 300 && resp.StatusCode < 400 && resp.Header.Get("Location") != "":
			if target, err = c.resolve(target, resp.Header.Get("Location")); err != nil {
				return nil, err
			}
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, errors.New("登录失败：Apple ID 或应用专用密码不对。要在 appleid.apple.com 里生成“应用专用密码”，不能用 Apple ID 的登录密码")
		case resp.StatusCode == http.StatusForbidden:
			return nil, errors.New("通讯录服务器拒绝了请求（403）。检查 Apple ID 是否开了双重认证，并且用的是应用专用密码")
		default:
			return nil, fmt.Errorf("通讯录服务器返回 %d", resp.StatusCode)
		}
	}
	return nil, errors.New("通讯录服务器重定向太多次")
}

// resolve reads href as an address relative to base, and puts it on origin:
// only its path is kept.
func (c *davClient) resolve(base, href string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	h, err := b.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", err
	}
	o, err := url.Parse(c.origin)
	if err != nil {
		return "", err
	}
	h.Scheme, h.Host = o.Scheme, o.Host
	return h.String(), nil
}

const (
	principalBody = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`
	homeBody      = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><d:prop><c:addressbook-home-set/></d:prop></d:propfind>`
	booksBody     = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`
	queryBody     = `<?xml version="1.0"?><c:addressbook-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><d:prop><c:address-data/></d:prop></c:addressbook-query>`
)

// fetch returns the text of every vCard in every address book of the account.
func (c *davClient) fetch(ctx context.Context, server string) ([]string, error) {
	ms, err := c.do(ctx, "PROPFIND", server, "0", principalBody)
	if err != nil {
		return nil, err
	}
	principal := ""
	for _, r := range ms.Responses {
		for _, p := range r.props() {
			if p.Principal.Href != "" {
				principal = p.Principal.Href
			}
		}
	}
	if principal == "" {
		return nil, errors.New("通讯录服务器没有返回账号信息")
	}
	if principal, err = c.resolve(server, principal); err != nil {
		return nil, err
	}
	if ms, err = c.do(ctx, "PROPFIND", principal, "0", homeBody); err != nil {
		return nil, err
	}
	home := ""
	for _, r := range ms.Responses {
		for _, p := range r.props() {
			if p.HomeSet.Href != "" {
				home = p.HomeSet.Href
			}
		}
	}
	if home == "" {
		return nil, errors.New("通讯录服务器没有返回通讯录目录")
	}
	if home, err = c.resolve(principal, home); err != nil {
		return nil, err
	}
	if ms, err = c.do(ctx, "PROPFIND", home, "1", booksBody); err != nil {
		return nil, err
	}
	var books []string
	for _, r := range ms.Responses {
		for _, p := range r.props() {
			if p.ResourceType.AddressBook != nil {
				if u, err := c.resolve(home, r.Href); err == nil {
					books = append(books, u)
				}
			}
		}
	}
	if len(books) == 0 {
		return nil, errors.New("这个账号下没有找到通讯录")
	}
	var cards []string
	for _, book := range books {
		ms, err := c.do(ctx, "REPORT", book, "1", queryBody)
		if err != nil {
			return nil, err
		}
		for _, r := range ms.Responses {
			for _, p := range r.props() {
				if strings.TrimSpace(p.AddressData) != "" {
					cards = append(cards, p.AddressData)
				}
			}
		}
	}
	return cards, nil
}

// ---------- settings and state ----------

func (m *Module) loadSync(ctx context.Context) (cfg syncConfig, ok bool, err error) {
	err = m.d.Settings.Get(ctx, keySync, &cfg)
	if errors.Is(err, settings.ErrNotSet) {
		return cfg, false, nil
	}
	return cfg, err == nil && cfg.Username != "", err
}

func (m *Module) loadState(ctx context.Context) (syncState, error) {
	var st syncState
	if err := m.d.Settings.Get(ctx, keySyncState, &st); err != nil && !errors.Is(err, settings.ErrNotSet) {
		return st, err
	}
	return st, nil
}

func (m *Module) status(ctx context.Context) (api.ContactSyncStatus, error) {
	cfg, ok, err := m.loadSync(ctx)
	if err != nil {
		return api.ContactSyncStatus{}, err
	}
	st, err := m.loadState(ctx)
	if err != nil {
		return api.ContactSyncStatus{}, err
	}
	out := api.ContactSyncStatus{Configured: ok, Syncing: m.syncing.Load(), LastSyncAt: st.LastSyncAt,
		Total: st.Total, Created: st.Created, Updated: st.Updated}
	if st.LastError != "" {
		out.LastError = &st.LastError
	}
	if ok {
		out.Username, out.Server, out.OnlyWithDates = &cfg.Username, &cfg.Server, &cfg.OnlyWithDates
	}
	return out, nil
}

// ---------- sync ----------

var allowInsecure bool // only the tests talk to a server without https

func (m *Module) davClient(cfg syncConfig) *davClient {
	hc := m.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}
	// redirects are followed in davClient.do, which keeps the method and the login
	hc2 := *hc
	hc2.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	origin := cfg.Server
	if origin == "" {
		origin = icloudServer
	}
	return &davClient{http: &hc2, username: cfg.Username, password: cfg.Password, origin: origin}
}

// syncNow reads the address books and applies them. Only one runs at a time.
func (m *Module) syncNow(ctx context.Context, cfg syncConfig) (applyResult, error) {
	if !m.syncing.CompareAndSwap(false, true) {
		return applyResult{}, httpx.NewError(http.StatusConflict, "sync_running", "正在同步，等它跑完")
	}
	defer m.syncing.Store(false)
	server := cfg.Server
	if server == "" {
		server = icloudServer
	}
	cards, err := m.davClient(cfg).fetch(ctx, server)
	if err != nil {
		return applyResult{}, err
	}
	var people []person
	bad := 0
	for _, c := range cards {
		p, b := parseVCards(strings.NewReader(c))
		people = append(people, p...)
		bad += b
	}
	res, err := m.apply(ctx, people, sourceICloud, cfg.OnlyWithDates)
	res.total += bad
	res.skipped += bad
	return res, err
}

// syncAndRecord runs a sync and keeps the result for the status line.
func (m *Module) syncAndRecord(ctx context.Context, cfg syncConfig) error {
	res, err := m.syncNow(ctx, cfg)
	st, _ := m.loadState(ctx)
	if err != nil {
		st.LastError = err.Error()
	} else {
		now := m.now().UTC()
		st = syncState{LastSyncAt: &now, Total: res.total, Created: res.created, Updated: res.updated}
		m.d.Bus.Publish("contact.updated", map[string]any{"sync": true})
	}
	if serr := m.d.Settings.Set(ctx, keySyncState, st); serr != nil && err == nil {
		err = serr
	}
	return err
}

// syncJob is the scheduled sync. Without an account it does nothing.
func (m *Module) syncJob(ctx context.Context) error {
	cfg, ok, err := m.loadSync(ctx)
	if err != nil || !ok {
		return err
	}
	st, _ := m.loadState(ctx)
	if st.LastSyncAt != nil && m.now().Sub(*st.LastSyncAt) < syncEvery-time.Minute {
		return nil
	}
	return m.syncAndRecord(ctx, cfg)
}

// ---------- handlers ----------

// GetContactSync implements api.ServerInterface.
func (m *Module) GetContactSync(w http.ResponseWriter, r *http.Request) {
	st, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

// SetContactSync implements api.ServerInterface.
func (m *Module) SetContactSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.SetContactSyncJSONRequestBody
	if err := httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cfg := syncConfig{Username: strings.TrimSpace(body.Username), Password: strings.TrimSpace(body.Password), OnlyWithDates: body.OnlyWithDates != nil && *body.OnlyWithDates}
	if body.Server != nil {
		cfg.Server = strings.TrimSpace(*body.Server)
	}
	err := m.setSync(ctx, cfg)
	m.d.Audit.Record(ctx, "contact.sync.set", cfg.Username, nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	st, err := m.status(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}

func (m *Module) setSync(ctx context.Context, cfg syncConfig) error {
	if cfg.Username == "" || cfg.Password == "" {
		return httpx.Invalid("请填 Apple ID 和应用专用密码")
	}
	if len(cfg.Username) > 200 || len(cfg.Password) > 200 {
		return httpx.Invalid("账号或密码太长了")
	}
	if cfg.Server != "" {
		if u, err := url.Parse(cfg.Server); err != nil || u.Scheme != "https" && !allowInsecure || u.Host == "" {
			return httpx.Invalid("服务器地址要以 https:// 开头")
		}
	}
	// 先登录试一次并同步，不通过就不保存
	res, err := m.syncNow(ctx, cfg)
	if err != nil {
		var apiErr *httpx.Error
		if errors.As(err, &apiErr) {
			return err
		}
		return httpx.Invalid(err.Error())
	}
	if err := m.d.Settings.SetSecret(ctx, keySync, cfg); err != nil {
		return err
	}
	now := m.now().UTC()
	m.d.Bus.Publish("contact.updated", map[string]any{"sync": true})
	return m.d.Settings.Set(ctx, keySyncState, syncState{LastSyncAt: &now, Total: res.total, Created: res.created, Updated: res.updated})
}

// DeleteContactSync implements api.ServerInterface.
func (m *Module) DeleteContactSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.d.Settings.Delete(ctx, keySync)
	if err == nil {
		err = m.d.Settings.Delete(ctx, keySyncState)
	}
	m.d.Audit.Record(ctx, "contact.sync.delete", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// RunContactSync implements api.ServerInterface.
func (m *Module) RunContactSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	cfg, ok, err := m.loadSync(ctx)
	if err == nil && !ok {
		err = errNotConfigured
	}
	if err == nil {
		err = m.syncAndRecord(ctx, cfg)
	}
	m.d.Audit.Record(ctx, "contact.sync.run", "", nil, err)
	if err != nil {
		var apiErr *httpx.Error
		if !errors.As(err, &apiErr) {
			// 同步失败的原因对用户有用，上面已经记在状态里
			err = httpx.NewError(http.StatusBadGateway, "sync_failed", err.Error())
		}
		httpx.Fail(w, r, err)
		return
	}
	st, err := m.status(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}
