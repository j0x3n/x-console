package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/db"
)

// B69：网盘账号。连接信息在 storage_remotes.config，密码、客户端密钥、
// 令牌加密存在设置里。

// Secret names, stored under storage.remote.<id>.<name>.
const (
	secPassword     = "password"
	secClientSecret = "client_secret"
	secToken        = "refresh_token"
)

// remoteConfig is storage_remotes.config.
type remoteConfig struct {
	URL      string `json:"url,omitempty"`      // WebDAV
	Username string `json:"username,omitempty"` // WebDAV
	ClientID string `json:"clientId,omitempty"` // Google Drive
	Account  string `json:"account,omitempty"`  // Google account, filled in after authorization
	Browse   bool   `json:"browse,omitempty"`   // the grant can read every file
	// LegacyCallback: authorized in B63 with /backups/gdrive/callback.
	LegacyCallback bool `json:"legacyCallback,omitempty"`
}

// Aliases of the anonymous structs in the generated API.
type (
	webdavView = struct {
		PasswordSet bool   `json:"passwordSet"`
		Url         string `json:"url"`
		Username    string `json:"username"`
	}
	gdriveView = struct {
		Account     *string `json:"account,omitempty"`
		Authorized  bool    `json:"authorized"`
		ClientId    string  `json:"clientId"`
		Limited     bool    `json:"limited"`
		RedirectUri string  `json:"redirectUri"`
		SecretSet   bool    `json:"secretSet"`
	}
)

var errRemoteMissing = httpx.NewError(http.StatusNotFound, "not_found", "找不到这个网盘账号")

func secretKey(id int64, name string) string { return fmt.Sprintf("storage.remote.%d.%s", id, name) }

func (m *Module) remoteSecret(ctx context.Context, id int64, name string) string {
	var v string
	if err := m.get(ctx, secretKey(id, name), &v); err != nil {
		m.d.Log.Warn("storage: remote secret cannot be read", "id", id, "name", name, "error", err)
	}
	return v
}

func (m *Module) setRemoteSecret(ctx context.Context, id int64, name, v string) error {
	if v == "" {
		return m.d.Settings.Delete(ctx, secretKey(id, name))
	}
	return m.d.Settings.SetSecret(ctx, secretKey(id, name), v)
}

func decodeConfig(raw string) remoteConfig {
	var c remoteConfig
	_ = json.Unmarshal([]byte(raw), &c)
	return c
}

func encodeConfig(c remoteConfig) string {
	b, _ := json.Marshal(c)
	return string(b)
}

func (m *Module) loadRemote(ctx context.Context, id int64) (db.StorageRemote, remoteConfig, error) {
	row, err := m.q.GetRemote(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, remoteConfig{}, errRemoteMissing
	}
	if err != nil {
		return row, remoteConfig{}, err
	}
	return row, decodeConfig(row.Config), nil
}

// webdavName is the default name of a WebDAV account.
func webdavName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "WebDAV"
	}
	if strings.Contains(u.Hostname(), "jianguoyun") {
		return "坚果云"
	}
	return u.Hostname()
}

func defaultName(kind string, c remoteConfig) string {
	if kind == contracts.RemoteGDrive {
		return "Google Drive"
	}
	return webdavName(c.URL)
}

// info is the contract view of an account.
func (m *Module) info(ctx context.Context, row db.StorageRemote) contracts.RemoteDrive {
	c := decodeConfig(row.Config)
	out := contracts.RemoteDrive{ID: row.ID, Kind: row.Kind, Name: row.Name, ShowInDrive: row.ShowInDrive == 1}
	if row.Kind == contracts.RemoteGDrive {
		out.Account = c.Account
		out.Ready = c.ClientID != "" && m.remoteSecret(ctx, row.ID, secClientSecret) != "" && m.remoteSecret(ctx, row.ID, secToken) != ""
		out.Limited = out.Ready && !c.Browse
	} else {
		out.Account = c.Username
		out.Ready = c.URL != ""
	}
	return out
}

func (m *Module) usedByBackup(ctx context.Context, id int64) bool {
	u, ok := module.Lookup[contracts.RemoteUser](m.d.Registry, contracts.RemoteUserKey)
	if !ok {
		return false
	}
	used, err := u.UsesRemote(ctx, id)
	return err == nil && used
}

func (m *Module) remoteView(ctx context.Context, r *http.Request, row db.StorageRemote) api.StorageRemote {
	c := decodeConfig(row.Config)
	in := m.info(ctx, row)
	out := api.StorageRemote{
		Id: row.ID, Kind: api.StorageRemoteKind(row.Kind), Name: row.Name, ShowInDrive: in.ShowInDrive,
		Ready: in.Ready, UsedByBackup: m.usedByBackup(ctx, row.ID), CreatedAt: row.CreatedAt,
	}
	if row.Kind == contracts.RemoteGDrive {
		g := gdriveView{
			ClientId: c.ClientID, SecretSet: m.remoteSecret(ctx, row.ID, secClientSecret) != "",
			Authorized: m.remoteSecret(ctx, row.ID, secToken) != "", Limited: in.Limited,
			RedirectUri: siteBase(m.d.Config.PublicURL, r) + "/api/v1" + callbackPath(c),
		}
		if g.Authorized && c.Account != "" {
			g.Account = ptr(c.Account)
		}
		out.Gdrive = &g
	} else {
		out.Webdav = &webdavView{Url: c.URL, Username: c.Username, PasswordSet: m.remoteSecret(ctx, row.ID, secPassword) != ""}
	}
	return out
}

// remoteHidden: the drive page, or every account of this kind, is hidden
// for the request (B57, B68).
func (m *Module) remoteHidden(ctx context.Context, kind string) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && (h.Hidden(ctx, "drive") || h.Hidden(ctx, "drive-"+kind))
}

// ListStorageRemotes is GET /storage/remotes.
func (m *Module) ListStorageRemotes(w http.ResponseWriter, r *http.Request, params api.ListStorageRemotesParams) {
	ctx := r.Context()
	rows, err := m.q.ListRemotes(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	drive := params.Drive != nil && *params.Drive
	items := make([]api.StorageRemote, 0, len(rows))
	for _, row := range rows {
		if drive {
			in := m.info(ctx, row)
			if !in.ShowInDrive || !in.Ready || m.remoteHidden(ctx, row.Kind) {
				continue
			}
		}
		items = append(items, m.remoteView(ctx, r, row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

// remoteForm is an account being created or changed, with its secrets.
type remoteForm struct {
	kind, name               string
	showInDrive              bool
	cfg                      remoteConfig
	password, secret         string // "" keeps the saved one
	clientChanged, connDirty bool
}

// applyInput merges in onto form. It does not connect.
func applyInput(f *remoteForm, in api.StorageRemoteInput) error {
	if in.Name != nil {
		f.name = strings.TrimSpace(*in.Name)
		if utf8.RuneCountInString(f.name) > 50 {
			return httpx.Invalid("名称最多 50 个字")
		}
	}
	if in.ShowInDrive != nil {
		f.showInDrive = *in.ShowInDrive
	}
	switch f.kind {
	case contracts.RemoteWebDAV:
		if w := in.Webdav; w != nil {
			if w.Url != nil {
				v := strings.TrimSpace(*w.Url)
				f.connDirty = f.connDirty || v != f.cfg.URL
				f.cfg.URL = v
			}
			if w.Username != nil {
				v := strings.TrimSpace(*w.Username)
				f.connDirty = f.connDirty || v != f.cfg.Username
				f.cfg.Username = v
			}
			if w.Password != nil && *w.Password != "" {
				f.password, f.connDirty = *w.Password, true
			}
		}
		if f.cfg.URL == "" {
			return httpx.Invalid("请填 WebDAV 地址")
		}
		if err := files.ValidateWebDAV(files.WebDAVConfig{URL: f.cfg.URL}); err != nil {
			return httpx.Invalid(err.Error())
		}
	case contracts.RemoteGDrive:
		if g := in.Gdrive; g != nil {
			if g.ClientId != nil {
				v := strings.TrimSpace(*g.ClientId)
				f.clientChanged = f.clientChanged || v != f.cfg.ClientID
				f.cfg.ClientID = v
			}
			if g.ClientSecret != nil && strings.TrimSpace(*g.ClientSecret) != "" {
				f.secret = strings.TrimSpace(*g.ClientSecret)
			}
		}
		if f.cfg.ClientID == "" {
			return httpx.Invalid("请填客户端 ID")
		}
	default:
		return httpx.Invalid("网盘类型只能是 webdav 或 gdrive")
	}
	if f.name == "" {
		f.name = defaultName(f.kind, f.cfg)
	}
	return nil
}

// checkWebDAV connects with the form's settings. savedID gives the saved
// password when the form has none.
func (m *Module) checkWebDAV(ctx context.Context, f remoteForm, savedID int64) error {
	password := f.password
	if password == "" && savedID != 0 {
		password = m.remoteSecret(ctx, savedID, secPassword)
	}
	w, err := files.NewWebDAV(files.WebDAVConfig{URL: f.cfg.URL, Username: f.cfg.Username, Password: password})
	if err != nil {
		return err
	}
	if err := w.Check(ctx); err != nil {
		return errors.New("连接测试没通过：" + err.Error())
	}
	return nil
}

// CreateStorageRemote is POST /storage/remotes.
func (m *Module) CreateStorageRemote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.StorageRemoteInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.createRemote(ctx, in)
	m.d.Audit.Record(ctx, "storage.remote.create", row.Name, map[string]any{"kind": row.Kind}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("storage.remotes", map[string]any{"id": row.ID})
	httpx.JSON(w, http.StatusCreated, m.remoteView(ctx, r, row))
}

func (m *Module) createRemote(ctx context.Context, in api.StorageRemoteInput) (db.StorageRemote, error) {
	if in.Kind == nil {
		return db.StorageRemote{}, httpx.Invalid("请选网盘类型")
	}
	f := remoteForm{kind: string(*in.Kind), showInDrive: true}
	if err := applyInput(&f, in); err != nil {
		return db.StorageRemote{}, err
	}
	switch f.kind {
	case contracts.RemoteWebDAV:
		if err := m.checkWebDAV(ctx, f, 0); err != nil {
			return db.StorageRemote{}, httpx.Invalid(err.Error())
		}
	case contracts.RemoteGDrive:
		if f.secret == "" {
			return db.StorageRemote{}, httpx.Invalid("请填客户端密钥")
		}
	}
	return m.insertRemote(ctx, f, "")
}

// insertRemote saves a new account and its secrets.
func (m *Module) insertRemote(ctx context.Context, f remoteForm, token string) (db.StorageRemote, error) {
	now := m.now().UTC()
	row, err := m.q.CreateRemote(ctx, db.CreateRemoteParams{
		Kind: f.kind, Name: f.name, Config: encodeConfig(f.cfg), ShowInDrive: boolInt(f.showInDrive), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return row, err
	}
	for name, v := range map[string]string{secPassword: f.password, secClientSecret: f.secret, secToken: token} {
		if v == "" {
			continue
		}
		if err := m.setRemoteSecret(ctx, row.ID, name, v); err != nil {
			_ = m.q.DeleteRemote(ctx, row.ID)
			return row, err
		}
	}
	return row, nil
}

// UpdateStorageRemote is PATCH /storage/remotes/{id}.
func (m *Module) UpdateStorageRemote(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.StorageRemoteInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, err := m.updateRemote(ctx, id, in)
	m.d.Audit.Record(ctx, "storage.remote.update", row.Name, map[string]any{"id": id}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("storage.remotes", map[string]any{"id": row.ID})
	httpx.JSON(w, http.StatusOK, m.remoteView(ctx, r, row))
}

func (m *Module) updateRemote(ctx context.Context, id int64, in api.StorageRemoteInput) (db.StorageRemote, error) {
	row, cfg, err := m.loadRemote(ctx, id)
	if err != nil {
		return row, err
	}
	if in.Kind != nil && string(*in.Kind) != row.Kind {
		return row, httpx.Invalid("不能改网盘类型，请删掉再加一个")
	}
	f := remoteForm{kind: row.Kind, name: row.Name, showInDrive: row.ShowInDrive == 1, cfg: cfg}
	if err := applyInput(&f, in); err != nil {
		return row, err
	}
	if f.kind == contracts.RemoteWebDAV && f.connDirty {
		if err := m.checkWebDAV(ctx, f, row.ID); err != nil {
			return row, httpx.Invalid(err.Error())
		}
	}
	if f.clientChanged {
		// 令牌属于原来的客户端
		if err := m.setRemoteSecret(ctx, row.ID, secToken, ""); err != nil {
			return row, err
		}
		f.cfg.Account, f.cfg.Browse = "", false
	}
	for name, v := range map[string]string{secPassword: f.password, secClientSecret: f.secret} {
		if v == "" {
			continue
		}
		if err := m.setRemoteSecret(ctx, row.ID, name, v); err != nil {
			return row, err
		}
	}
	return m.q.UpdateRemote(ctx, db.UpdateRemoteParams{
		Name: f.name, Config: encodeConfig(f.cfg), ShowInDrive: boolInt(f.showInDrive), UpdatedAt: m.now().UTC(), ID: row.ID,
	})
}

// DeleteStorageRemote is DELETE /storage/remotes/{id}.
func (m *Module) DeleteStorageRemote(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	row, _, err := m.loadRemote(ctx, id)
	if err == nil && m.usedByBackup(ctx, id) {
		err = httpx.NewError(http.StatusConflict, "conflict", "自动备份正在用这个账号，先到 设置 → 备份 换一个备份位置")
	}
	if err == nil {
		if token := m.remoteSecret(ctx, id, secToken); token != "" {
			if rerr := files.GoogleRevoke(ctx, m.google, token); rerr != nil {
				m.d.Log.Warn("storage: revoke Google token", "error", rerr)
			}
		}
		for _, name := range []string{secPassword, secClientSecret, secToken} {
			if err = m.setRemoteSecret(ctx, id, name, ""); err != nil {
				break
			}
		}
		if err == nil {
			err = m.q.DeleteRemote(ctx, id)
		}
	}
	m.d.Audit.Record(ctx, "storage.remote.delete", row.Name, map[string]any{"id": id}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.d.Bus.Publish("storage.remotes", map[string]any{"id": id})
	httpx.NoContent(w)
}

// TestStorageRemote is POST /storage/remotes/test.
func (m *Module) TestStorageRemote(w http.ResponseWriter, r *http.Request, params api.TestStorageRemoteParams) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.StorageRemoteInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var f remoteForm
	var savedID int64
	if params.Id != nil {
		row, cfg, err := m.loadRemote(ctx, *params.Id)
		if err != nil {
			httpx.Fail(w, r, err)
			return
		}
		f = remoteForm{kind: row.Kind, name: row.Name, cfg: cfg}
		savedID = row.ID
	} else if in.Kind != nil {
		f.kind = string(*in.Kind)
	}
	out := api.TestResult{Ok: true, Message: "连接正常"}
	fail := func(err error) {
		var herr *httpx.Error
		if errors.As(err, &herr) {
			out = api.TestResult{Ok: false, Message: herr.Message}
			return
		}
		out = api.TestResult{Ok: false, Message: err.Error()}
	}
	if err := applyInput(&f, in); err != nil {
		fail(err)
	} else if f.kind == contracts.RemoteWebDAV {
		if err := m.checkWebDAV(ctx, f, savedID); err != nil {
			fail(err)
		} else {
			out.Message = "连接正常，可以读写"
		}
	} else if savedID == 0 || f.clientChanged || m.remoteSecret(ctx, savedID, secToken) == "" {
		fail(errors.New("Google Drive 要先保存再授权，授权后才能测试"))
	} else if g, err := m.openGDrive(ctx, savedID, "", ""); err != nil {
		fail(err)
	} else if account, err := g.Account(ctx); err != nil {
		fail(errors.New("连接测试没通过：" + err.Error()))
	} else {
		out.Message = "连接正常：" + account
	}
	httpx.JSON(w, http.StatusOK, out)
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
