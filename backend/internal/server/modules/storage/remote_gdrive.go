package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
)

// Google 授权跳回的地址。公开，靠一次性 state 校验。B63 时授权的账号用
// 旧地址，这样 Google 控制台里填的重定向 URI 不用改。
const (
	gdriveCallbackPath = "/storage/remotes/gdrive/callback"
	legacyCallbackPath = "/backups/gdrive/callback"
	stateTTL           = 10 * time.Minute
)

// oauthState is one authorization started and not finished yet.
type oauthState struct {
	id       int64
	expires  time.Time
	redirect string
	actor    string
}

// PublicPaths implements module.PublicPather.
func (m *Module) PublicPaths() []string { return []string{gdriveCallbackPath} }

func callbackPath(c remoteConfig) string {
	if c.LegacyCallback {
		return legacyCallbackPath
	}
	return gdriveCallbackPath
}

// siteBase is the address the browser uses for the site.
func siteBase(public string, r *http.Request) string {
	if base := strings.TrimRight(public, "/"); base != "" {
		return base
	}
	if r == nil {
		return ""
	}
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		proto = forwarded
	}
	return proto + "://" + r.Host
}

// openGDrive opens a Google Drive account.
func (m *Module) openGDrive(ctx context.Context, id int64, folderID, folderName string) (*files.GDrive, error) {
	row, c, err := m.loadRemote(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != contracts.RemoteGDrive {
		return nil, httpx.Invalid("这个账号不是 Google Drive")
	}
	secret, token := m.remoteSecret(ctx, id, secClientSecret), m.remoteSecret(ctx, id, secToken)
	if c.ClientID == "" || secret == "" {
		return nil, notReady("Google Drive 的客户端 ID 和密钥没有填")
	}
	if token == "" {
		return nil, notReady("Google Drive 还没授权，到 设置 → 存储 授权")
	}
	if folderID == "" && folderName == "" {
		// 只浏览或读账号时用不到这个文件夹，不会去建
		folderName = "X Console 备份"
	}
	g, err := files.NewGDrive(files.GDriveConfig{ClientID: c.ClientID, ClientSecret: secret, RefreshToken: token,
		FolderID: folderID, FolderName: folderName, Endpoints: m.google})
	if err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	return g, nil
}

func notReady(message string) error {
	return httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", message)
}

func (m *Module) startGDriveAuth(ctx context.Context, id int64, base string) (string, string, error) {
	row, c, err := m.loadRemote(ctx, id)
	if err != nil {
		return "", "", err
	}
	if row.Kind != contracts.RemoteGDrive {
		return "", "", httpx.Invalid("这个账号不是 Google Drive")
	}
	if c.ClientID == "" || m.remoteSecret(ctx, id, secClientSecret) == "" {
		return "", "", httpx.Invalid("先填好客户端 ID 和密钥并保存，再授权")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	state := hex.EncodeToString(raw)
	redirect := base + "/api/v1" + callbackPath(c)
	now := m.now()
	m.statesMu.Lock()
	for k, v := range m.states {
		if now.After(v.expires) {
			delete(m.states, k)
		}
	}
	m.states[state] = oauthState{id: id, expires: now.Add(stateTTL), redirect: redirect, actor: audit.Actor(ctx)}
	m.statesMu.Unlock()
	return files.GoogleAuthURL(m.google, c.ClientID, redirect, state), redirect, nil
}

// StartStorageGdriveAuth is GET /storage/remotes/{id}/gdrive/auth.
func (m *Module) StartStorageGdriveAuth(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, redirect, err := m.startGDriveAuth(ctx, id, siteBase(m.d.Config.PublicURL, r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"url": u, "redirectUri": redirect})
}

func (m *Module) takeState(state string) (oauthState, bool) {
	m.statesMu.Lock()
	defer m.statesMu.Unlock()
	st, ok := m.states[state]
	delete(m.states, state)
	if !ok || m.now().After(st.expires) {
		return oauthState{}, false
	}
	return st, true
}

// finishGDriveAuth handles the browser coming back from Google, on either
// callback address, and returns where to send it.
func (m *Module) finishGDriveAuth(ctx context.Context, state, code, googleError string) string {
	back := func(err error) string {
		if err == nil {
			return "/settings/storage?gdrive=ok"
		}
		return "/settings/storage?" + url.Values{"gdrive": {"error"}, "message": {err.Error()}}.Encode()
	}
	st, ok := m.takeState(state)
	if !ok {
		return back(errors.New("授权链接已失效，请回到设置页重新点授权"))
	}
	ctx = audit.WithActor(ctx, st.actor)
	if googleError != "" {
		if googleError == "access_denied" {
			return back(errors.New("你在 Google 页面取消了授权"))
		}
		return back(errors.New("Google 授权失败：" + googleError))
	}
	account, err := m.authorize(ctx, st.id, code, st.redirect)
	m.d.Audit.Record(ctx, "storage.remote.gdrive.authorize", account, map[string]any{"id": st.id}, err)
	if err == nil {
		m.d.Bus.Publish("storage.remotes", map[string]any{"id": st.id})
	}
	return back(err)
}

// StorageGdriveCallback is GET /storage/remotes/gdrive/callback.
func (m *Module) StorageGdriveCallback(w http.ResponseWriter, r *http.Request, params api.StorageGdriveCallbackParams) {
	target := m.finishGDriveAuth(r.Context(), value(params.State), value(params.Code), value(params.Error))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// authorize trades the code for a refresh token and saves it with the
// account name.
func (m *Module) authorize(ctx context.Context, id int64, code, redirect string) (string, error) {
	if code == "" {
		return "", errors.New("Google 没有返回授权码")
	}
	row, c, err := m.loadRemote(ctx, id)
	if err != nil {
		return "", errors.New("这个网盘账号已经删了")
	}
	secret := m.remoteSecret(ctx, id, secClientSecret)
	if c.ClientID == "" || secret == "" {
		return "", errors.New("客户端 ID 和密钥没有保存")
	}
	grant, err := files.GoogleExchange(ctx, m.google, c.ClientID, secret, code, redirect)
	if err != nil {
		return "", err
	}
	if err := m.setRemoteSecret(ctx, id, secToken, grant.RefreshToken); err != nil {
		return "", err
	}
	g, err := m.openGDrive(ctx, id, "", "")
	if err != nil {
		return "", err
	}
	account, err := g.Account(ctx)
	if err != nil {
		return "", errors.New("已授权，但读不到账号：" + err.Error())
	}
	c.Account, c.Browse = account, grant.CanBrowse()
	_, err = m.q.UpdateRemote(ctx, updateParams(row, c, m.now()))
	return account, err
}

// RevokeStorageGdriveAuth is DELETE /storage/remotes/{id}/gdrive/auth.
func (m *Module) RevokeStorageGdriveAuth(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.revokeGDrive(ctx, id)
	m.d.Audit.Record(ctx, "storage.remote.gdrive.revoke", "", map[string]any{"id": id}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("storage.remotes", map[string]any{"id": id})
	httpx.NoContent(w)
}

func (m *Module) revokeGDrive(ctx context.Context, id int64) error {
	row, c, err := m.loadRemote(ctx, id)
	if err != nil {
		return err
	}
	if token := m.remoteSecret(ctx, id, secToken); token != "" {
		if err := files.GoogleRevoke(ctx, m.google, token); err != nil {
			// 本地的令牌照样删，Google 会自己作废不用的令牌
			m.d.Log.Warn("storage: revoke Google token", "error", err)
		}
	}
	if err := m.setRemoteSecret(ctx, id, secToken, ""); err != nil {
		return err
	}
	c.Account, c.Browse = "", false
	_, err = m.q.UpdateRemote(ctx, updateParams(row, c, m.now()))
	return err
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
