package backup

import (
	"context"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

// B69：网盘账号挪到了存储模块。这里留着 B63 的几个地址，转给存储模块，
// 下个版本删掉。回调地址一直保留：从 B63 迁过来的账号还用它。

// gdriveCallbackPath is the address B63 asked people to enter at Google.
const gdriveCallbackPath = "/backups/gdrive/callback"

// keyMigrated records that the B63 accounts were moved (B69).
const keyMigrated = "storage.migrated_b69"

// PublicPaths implements module.PublicPather.
func (m *Module) PublicPaths() []string { return []string{gdriveCallbackPath} }

func siteBase(public string, r *http.Request) string {
	if base := strings.TrimRight(public, "/"); base != "" {
		return base
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

// legacyGDrive is the Google account the old addresses mean: the one the
// backups use, or else the first one.
func (m *Module) legacyGDrive(ctx context.Context) (contracts.RemoteDrives, int64, error) {
	rd, err := m.remotes()
	if err != nil {
		return nil, 0, err
	}
	s, err := m.loadSettings(ctx)
	if err != nil {
		return nil, 0, err
	}
	list, err := rd.List(ctx)
	if err != nil {
		return nil, 0, err
	}
	var first int64
	for _, a := range list {
		if a.Kind != contracts.RemoteGDrive {
			continue
		}
		if s.Target == targetRemote && s.RemoteID == a.ID {
			return rd, a.ID, nil
		}
		if first == 0 {
			first = a.ID
		}
	}
	if first == 0 {
		return nil, 0, httpx.Invalid("先到 设置 → 存储 添加 Google Drive 账号")
	}
	return rd, first, nil
}

// StartGdriveAuth is GET /backups/gdrive/auth (old, B63).
func (m *Module) StartGdriveAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rd, id, err := m.legacyGDrive(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	u, redirect, err := rd.StartGDriveAuth(ctx, id, siteBase(m.d.Config.PublicURL, r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"url": u, "redirectUri": redirect})
}

// RevokeGdriveAuth is DELETE /backups/gdrive/auth (old, B63).
func (m *Module) RevokeGdriveAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rd, id, err := m.legacyGDrive(ctx)
	if err == nil {
		err = rd.RevokeGDrive(ctx, id)
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// GdriveCallback is GET /backups/gdrive/callback. The storage module checks
// the state and saves the token.
func (m *Module) GdriveCallback(w http.ResponseWriter, r *http.Request, params api.GdriveCallbackParams) {
	target := "/settings/storage?gdrive=error&message=" + "存储模块没有启用"
	if rd, err := m.remotes(); err == nil {
		target = rd.FinishGDriveAuth(r.Context(), value(params.State), value(params.Code), value(params.Error))
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// UsesRemote implements contracts.RemoteUser: the automatic backups go to
// this account.
func (m *Module) UsesRemote(ctx context.Context, id int64) (bool, error) {
	s, err := m.loadSettings(ctx)
	if err != nil {
		return false, err
	}
	return s.Target == targetRemote && s.RemoteID == id, nil
}

// migrateRemotes moves the B63 WebDAV and Google Drive setup to the storage
// accounts once (B69). The old keys stay for one version.
func (m *Module) migrateRemotes(ctx context.Context) error {
	var done bool
	if err := m.d.Settings.Get(ctx, keyMigrated, &done); err == nil && done {
		return nil
	}
	rd, err := m.remotes()
	if err != nil {
		return nil // 没有存储模块时不迁
	}
	s, err := m.loadSettings(ctx)
	if err != nil {
		return err
	}
	var webdavID, gdriveID int64
	if s.WebDAV.URL != "" {
		webdavID, err = rd.Import(ctx, contracts.RemoteImport{
			Kind: contracts.RemoteWebDAV, URL: s.WebDAV.URL, Username: s.WebDAV.Username, Password: m.secret(ctx, keyWebDAVPassword),
		})
		if err != nil {
			return err
		}
	}
	if s.GDrive.ClientID != "" {
		token := m.secret(ctx, keyGDriveToken)
		in := contracts.RemoteImport{
			Kind: contracts.RemoteGDrive, ClientID: s.GDrive.ClientID, ClientSecret: m.secret(ctx, keyGDriveSecret),
			RefreshToken: token, Browse: s.GDrive.Browse, LegacyCallback: true,
		}
		if token != "" {
			in.Account = s.GDrive.Account
		}
		if gdriveID, err = rd.Import(ctx, in); err != nil {
			return err
		}
	}
	err = m.update(ctx, func(cur *settingsData) {
		switch {
		case cur.Target == targetWebDAV && webdavID != 0:
			cur.Target, cur.RemoteID = targetRemote, webdavID
		case cur.Target == targetGDrive && gdriveID != 0:
			cur.Target, cur.RemoteID = targetRemote, gdriveID
		case cur.Target == targetWebDAV || cur.Target == targetGDrive:
			cur.Target, cur.Enabled = targetStorage, false
		}
	})
	if err != nil {
		return err
	}
	if webdavID != 0 || gdriveID != 0 {
		m.log().Info("backup: moved the B63 drive accounts to storage", "webdav", webdavID, "gdrive", gdriveID)
	}
	return m.d.Settings.Set(ctx, keyMigrated, true)
}
