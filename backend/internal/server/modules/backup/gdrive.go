package backup

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
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

// gdriveCallbackPath is where Google sends the browser back. It is public:
// the one-time state proves the request started from a signed-in session.
const gdriveCallbackPath = "/backups/gdrive/callback"

const stateTTL = 10 * time.Minute

// oauthState is one authorization that was started and not finished yet.
type oauthState struct {
	expires  time.Time
	redirect string
	actor    string
}

// PublicPaths implements module.PublicPather.
func (m *Module) PublicPaths() []string { return []string{gdriveCallbackPath} }

// redirectURI is the callback address Google must send the browser to. It
// must match the one in the OAuth client.
func (m *Module) redirectURI(r *http.Request) string {
	base := strings.TrimRight(m.d.Config.PublicURL, "/")
	if base == "" {
		proto := "http"
		if r.TLS != nil {
			proto = "https"
		}
		if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
			proto = forwarded
		}
		base = proto + "://" + r.Host
	}
	return base + "/api/v1" + gdriveCallbackPath
}

// StartGdriveAuth is GET /backups/gdrive/auth.
func (m *Module) StartGdriveAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := m.loadSettings(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if s.GDrive.ClientID == "" || m.secret(ctx, keyGDriveSecret) == "" {
		httpx.Fail(w, r, httpx.Invalid("先填好客户端 ID 和密钥并保存，再授权"))
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	state := hex.EncodeToString(raw)
	redirect := m.redirectURI(r)
	now := m.now()
	m.statesMu.Lock()
	for k, v := range m.states {
		if now.After(v.expires) {
			delete(m.states, k)
		}
	}
	m.states[state] = oauthState{expires: now.Add(stateTTL), redirect: redirect, actor: audit.Actor(ctx)}
	m.statesMu.Unlock()
	httpx.JSON(w, http.StatusOK, map[string]string{
		"url": files.GoogleAuthURL(m.google, s.GDrive.ClientID, redirect, state), "redirectUri": redirect})
}

// takeState returns and forgets a state that is still valid.
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

// GdriveCallback is GET /backups/gdrive/callback.
func (m *Module) GdriveCallback(w http.ResponseWriter, r *http.Request, params api.GdriveCallbackParams) {
	back := func(err error) {
		target := "/settings/backup?gdrive=ok"
		if err != nil {
			target = "/settings/backup?" + url.Values{"gdrive": {"error"}, "message": {err.Error()}}.Encode()
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, target, http.StatusFound)
	}
	st, ok := m.takeState(value(params.State))
	if !ok {
		back(errors.New("授权链接已失效，请回到设置页重新点授权"))
		return
	}
	ctx := audit.WithActor(r.Context(), st.actor)
	if e := value(params.Error); e != "" {
		if e == "access_denied" {
			back(errors.New("你在 Google 页面取消了授权"))
		} else {
			back(errors.New("Google 授权失败：" + e))
		}
		return
	}
	account, err := m.authorize(ctx, value(params.Code), st.redirect)
	m.d.Audit.Record(ctx, "backup.gdrive.authorize", account, nil, err)
	back(err)
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// authorize trades the code for a refresh token, saves it, and finds the
// account and the folder.
func (m *Module) authorize(ctx context.Context, code, redirect string) (string, error) {
	if code == "" {
		return "", errors.New("Google 没有返回授权码")
	}
	s, err := m.loadSettings(ctx)
	if err != nil {
		return "", err
	}
	secret := m.secret(ctx, keyGDriveSecret)
	if s.GDrive.ClientID == "" || secret == "" {
		return "", errors.New("客户端 ID 和密钥没有保存")
	}
	grant, err := files.GoogleExchange(ctx, m.google, s.GDrive.ClientID, secret, code, redirect)
	if err != nil {
		return "", err
	}
	if err := m.d.Settings.SetSecret(ctx, keyGDriveToken, grant.RefreshToken); err != nil {
		return "", err
	}
	s.GDrive.FolderID = "" // the folder may belong to another account
	g, err := m.openGDrive(ctx, s, "")
	if err != nil {
		return "", err
	}
	account, err := g.Account(ctx)
	if err != nil {
		return "", errors.New("已授权，但读不到账号：" + err.Error())
	}
	folder, err := g.Folder(ctx)
	if err != nil {
		return account, errors.New("已授权，但建不了备份文件夹：" + err.Error())
	}
	err = m.update(ctx, func(cur *settingsData) {
		cur.GDrive.Account = account
		cur.GDrive.Browse = grant.CanBrowse()
		if cur.GDrive.FolderName == s.GDrive.FolderName {
			cur.GDrive.FolderID = folder
		} else {
			cur.GDrive.FolderID = ""
		}
	})
	return account, err
}

// RevokeGdriveAuth is DELETE /backups/gdrive/auth.
func (m *Module) RevokeGdriveAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if token := m.secret(ctx, keyGDriveToken); token != "" {
		if err := files.GoogleRevoke(ctx, m.google, token); err != nil {
			// The local token goes anyway; Google drops unused ones in time.
			m.log().Warn("backup: revoke Google token", "error", err)
		}
	}
	err := m.d.Settings.Delete(ctx, keyGDriveToken)
	if err == nil {
		err = m.update(ctx, func(s *settingsData) { s.GDrive.Account, s.GDrive.FolderID, s.GDrive.Browse = "", "", false })
	}
	m.d.Audit.Record(ctx, "backup.gdrive.revoke", "", nil, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}
