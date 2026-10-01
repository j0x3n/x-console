package backup

import (
	"context"
	"errors"
	"io"
	"iter"
	"net/http"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
)

// Where the automatic backups go (B25, B63).
const (
	targetStorage = "storage" // the S3 of the storage settings
	targetCustom  = "custom"  // an S3 of its own
	targetWebDAV  = "webdav"
	targetGDrive  = "gdrive"
)

// More settings keys, all encrypted (B63).
const (
	keyWebDAVPassword = "backup.webdav_password"
	keyGDriveSecret   = "backup.gdrive_client_secret"
	keyGDriveToken    = "backup.gdrive_refresh_token"
)

const (
	defaultWebDAVFolder = "x-console-backups"
	defaultGDriveFolder = "X Console 备份"
)

// webdavFields is the WebDAV setup, kept in the settings without the password.
type webdavFields struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Folder   string `json:"folder"`
}

// gdriveFields is the Google Drive setup, kept without the client secret and
// the refresh token. FolderID and Account are filled in by the module.
type gdriveFields struct {
	ClientID   string `json:"clientId"`
	FolderName string `json:"folderName"`
	FolderID   string `json:"folderId,omitempty"`
	Account    string `json:"account,omitempty"`
}

// newSecrets holds secrets typed in but not saved yet. "" means the saved one.
type newSecrets struct {
	s3, webdav, gdrive string
}

// target is an opened backup location.
type target struct {
	store  files.Store // keys start with remoteFolder
	loc    api.BackupLocation
	check  func(context.Context) error
	gdrive *files.GDrive // set for Google Drive, to remember the folder id
}

func (m *Module) secret(ctx context.Context, key string) string {
	var v string
	if err := m.d.Settings.Get(ctx, key, &v); err != nil && !errors.Is(err, settings.ErrNotSet) {
		m.log().Warn("backup: secret cannot be read, treating it as not set", "key", key, "error", err)
	}
	return v
}

func notReady(message string) error {
	return httpx.NewError(http.StatusPreconditionFailed, "integration_not_configured", message)
}

// open builds the store the settings point at. It does not connect.
func (m *Module) open(ctx context.Context, s settingsData, p newSecrets) (target, error) {
	switch s.Target {
	case targetWebDAV:
		password := p.webdav
		if password == "" {
			password = m.secret(ctx, keyWebDAVPassword)
		}
		if s.WebDAV.URL == "" {
			return target{}, notReady("WebDAV 设置没有填完整")
		}
		w, err := files.NewWebDAV(files.WebDAVConfig{URL: s.WebDAV.URL, Username: s.WebDAV.Username, Password: password, Folder: s.WebDAV.Folder})
		if err != nil {
			return target{}, httpx.Invalid(err.Error())
		}
		return target{store: flat{w}, loc: api.BackupLocationWebdav, check: w.Check}, nil
	case targetGDrive:
		g, err := m.openGDrive(ctx, s, p.gdrive)
		if err != nil {
			return target{}, err
		}
		return target{store: flat{g}, loc: api.BackupLocationGdrive, check: g.Check, gdrive: g}, nil
	}
	cfg, ok, err := m.s3Config(ctx, s, p.s3)
	if err != nil {
		return target{}, err
	}
	if !ok {
		return target{}, errNoS3
	}
	s3, err := files.NewS3(cfg)
	if err != nil {
		return target{}, httpx.Invalid(err.Error())
	}
	return target{store: s3, loc: api.BackupLocationS3, check: s3.Check}, nil
}

func (m *Module) openGDrive(ctx context.Context, s settingsData, newSecret string) (*files.GDrive, error) {
	secret := newSecret
	if secret == "" {
		secret = m.secret(ctx, keyGDriveSecret)
	}
	token := m.secret(ctx, keyGDriveToken)
	if s.GDrive.ClientID == "" || secret == "" {
		return nil, notReady("Google Drive 的客户端 ID 和密钥没有填")
	}
	if token == "" {
		return nil, notReady("Google Drive 还没授权")
	}
	g, err := files.NewGDrive(files.GDriveConfig{ClientID: s.GDrive.ClientID, ClientSecret: secret, RefreshToken: token,
		FolderID: s.GDrive.FolderID, FolderName: s.GDrive.FolderName, Endpoints: m.google})
	if err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	return g, nil
}

// rememberFolder saves the id of the Drive folder once it is known, so later
// runs do not search for it by name.
func (m *Module) rememberFolder(ctx context.Context, s settingsData, g *files.GDrive) {
	if g == nil || s.GDrive.FolderID != "" {
		return
	}
	id, err := g.Folder(ctx)
	if err != nil {
		return
	}
	err = m.update(ctx, func(cur *settingsData) {
		if cur.GDrive.FolderName == s.GDrive.FolderName && cur.GDrive.ClientID == s.GDrive.ClientID {
			cur.GDrive.FolderID = id
		}
	})
	if err != nil {
		m.log().Warn("backup: remember Drive folder", "error", err)
	}
}

// flat keeps the packages at the top of a WebDAV or Drive folder: the key
// "backups/x.tar.gz" is stored as "x.tar.gz". The folder is the user's own,
// so a second "backups" level would only be in the way.
type flat struct{ s files.Store }

func (f flat) key(key string) (string, error) {
	rest, ok := strings.CutPrefix(key, remoteFolder)
	if !ok || rest == "" {
		return "", files.ErrBadKey
	}
	return rest, nil
}

func (f flat) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	k, err := f.key(key)
	if err != nil {
		return err
	}
	return f.s.Put(ctx, k, r, size)
}

func (f flat) Get(ctx context.Context, key string) (io.ReadCloser, files.Info, error) {
	return f.GetRange(ctx, key, 0, -1)
}

func (f flat) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, files.Info, error) {
	k, err := f.key(key)
	if err != nil {
		return nil, files.Info{}, err
	}
	rc, info, err := f.s.GetRange(ctx, k, offset, length)
	info.Key = remoteFolder + info.Key
	return rc, info, err
}

func (f flat) Stat(ctx context.Context, key string) (files.Info, error) {
	k, err := f.key(key)
	if err != nil {
		return files.Info{}, err
	}
	info, err := f.s.Stat(ctx, k)
	info.Key = remoteFolder + info.Key
	return info, err
}

func (f flat) Delete(ctx context.Context, key string) error {
	k, err := f.key(key)
	if err != nil {
		return err
	}
	return f.s.Delete(ctx, k)
}

func (f flat) List(ctx context.Context, prefix string) iter.Seq2[files.Info, error] {
	return func(yield func(files.Info, error) bool) {
		if strings.TrimSuffix(prefix, "/") != strings.TrimSuffix(remoteFolder, "/") {
			yield(files.Info{}, files.ErrBadKey)
			return
		}
		for info, err := range f.s.List(ctx, "") {
			if err == nil {
				info.Key = remoteFolder + info.Key
			}
			if !yield(info, err) {
				return
			}
		}
	}
}

func (f flat) Copy(ctx context.Context, from, to string) error {
	a, err := f.key(from)
	if err != nil {
		return err
	}
	b, err := f.key(to)
	if err != nil {
		return err
	}
	return f.s.Copy(ctx, a, b)
}

// s3Config is the bucket of the S3 targets.
// secret is a new secret key that is not saved yet, or "" to use the saved one.
func (m *Module) s3Config(ctx context.Context, s settingsData, secret string) (files.S3Config, bool, error) {
	if s.Target == targetCustom {
		if secret == "" {
			secret = m.secret(ctx, keyS3Secret)
		}
		c := files.S3Config{Endpoint: s.S3.Endpoint, Region: s.S3.Region, Bucket: s.S3.Bucket, Prefix: s.S3.Prefix,
			AccessKeyID: s.S3.AccessKeyID, SecretAccessKey: secret, PathStyle: s.S3.PathStyle}
		return c, c.Endpoint != "" && c.Bucket != "" && c.AccessKeyID != "" && secret != "", nil
	}
	return storage.S3Config(ctx, m.d.Settings, m.log())
}
