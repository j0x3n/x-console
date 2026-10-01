package storage

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/db"
)

// 云盘页浏览网盘账号（B68、B69）：只能列文件夹和下载。

// browser is one opened account.
type browser struct {
	webdav *files.WebDAV
	gdrive *files.GDrive
}

func (m *Module) openWebDAV(ctx context.Context, id int64, folder string) (*files.WebDAV, error) {
	row, c, err := m.loadRemote(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != contracts.RemoteWebDAV {
		return nil, httpx.Invalid("这个账号不是 WebDAV")
	}
	if c.URL == "" {
		return nil, notReady("WebDAV 设置没有填完整")
	}
	w, err := files.NewWebDAV(files.WebDAVConfig{URL: c.URL, Username: c.Username, Password: m.remoteSecret(ctx, id, secPassword), Folder: folder})
	if err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	return w, nil
}

// openBrowser opens an account for the drive page. Hidden accounts look
// like they do not exist.
func (m *Module) openBrowser(ctx context.Context, id int64) (browser, error) {
	row, _, err := m.loadRemote(ctx, id)
	if err != nil {
		return browser{}, err
	}
	if m.remoteHidden(ctx, row.Kind) {
		return browser{}, errRemoteMissing
	}
	if row.Kind == contracts.RemoteWebDAV {
		w, err := m.openWebDAV(ctx, id, "")
		return browser{webdav: w}, err
	}
	g, err := m.openGDrive(ctx, id, "", "")
	return browser{gdrive: g}, err
}

// browseError turns a store error into an answer for the page.
func browseError(err error) error {
	switch {
	case errors.Is(err, files.ErrNotFound):
		return httpx.NewError(http.StatusNotFound, "not_found", "找不到这个文件或文件夹")
	case errors.Is(err, files.ErrBadKey):
		return httpx.Invalid("位置不正确")
	}
	var herr *httpx.Error
	if errors.As(err, &herr) {
		return err
	}
	return httpx.NewError(http.StatusBadGateway, "remote_failed", err.Error())
}

func entryAPI(e files.DirEntry) api.StorageRemoteEntry {
	out := api.StorageRemoteEntry{Ref: e.Ref, Name: e.Name, IsDir: e.Dir, Downloadable: e.Downloadable()}
	if !e.Dir {
		out.Size = ptr(e.Size)
	}
	if !e.ModTime.IsZero() {
		out.ModifiedAt = ptr(e.ModTime.UTC())
	}
	return out
}

type crumb = struct {
	Name string `json:"name"`
	Ref  string `json:"ref"`
}

// ListStorageRemoteItems is GET /storage/remotes/{id}/items.
func (m *Module) ListStorageRemoteItems(w http.ResponseWriter, r *http.Request, id api.RemoteId, params api.ListStorageRemoteItemsParams) {
	ctx := r.Context()
	b, err := m.openBrowser(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref := strings.Trim(value(params.Ref), "/")
	out := api.StorageRemoteListing{Ref: ref, Items: []api.StorageRemoteEntry{}, Trail: []crumb{}}
	var list []files.DirEntry
	if b.webdav != nil {
		list, err = b.webdav.ReadDir(ctx, ref)
		if ref != "" {
			segs := strings.Split(ref, "/")
			for i := range segs {
				out.Trail = append(out.Trail, crumb{Name: segs[i], Ref: strings.Join(segs[:i+1], "/")})
			}
		}
	} else {
		list, err = b.gdrive.ReadDir(ctx, ref)
		if err == nil {
			var trail []files.DirEntry
			trail, err = b.gdrive.Trail(ctx, ref)
			for _, t := range trail {
				out.Trail = append(out.Trail, crumb{Name: t.Name, Ref: t.Ref})
			}
		}
	}
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	for _, e := range list {
		out.Items = append(out.Items, entryAPI(e))
	}
	httpx.JSON(w, http.StatusOK, out)
}

// DownloadStorageRemoteFile is GET /storage/remotes/{id}/download.
func (m *Module) DownloadStorageRemoteFile(w http.ResponseWriter, r *http.Request, id api.RemoteId, params api.DownloadStorageRemoteFileParams) {
	ctx := r.Context()
	b, err := m.openBrowser(ctx, id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref := strings.Trim(params.Ref, "/")
	disposition := func(name string) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
	}
	if b.webdav != nil {
		f, info, err := files.OpenSeeker(ctx, b.webdav, ref)
		if err != nil {
			httpx.Fail(w, r, browseError(err))
			return
		}
		defer f.Close()
		name := ref[strings.LastIndex(ref, "/")+1:]
		disposition(name)
		http.ServeContent(w, r, name, info.ModTime, f)
		return
	}
	rc, e, err := b.gdrive.OpenFile(ctx, ref)
	if err != nil {
		if !errors.Is(err, files.ErrNotFound) && e.Name != "" && !e.Downloadable() {
			httpx.Fail(w, r, httpx.Invalid(err.Error()))
			return
		}
		httpx.Fail(w, r, browseError(err))
		return
	}
	defer rc.Close()
	disposition(e.Name)
	w.Header().Set("Content-Length", strconv.FormatInt(e.Size, 10))
	io.Copy(w, rc) //nolint:errcheck // the client went away
}

func updateParams(row db.StorageRemote, c remoteConfig, now time.Time) db.UpdateRemoteParams {
	return db.UpdateRemoteParams{Name: row.Name, Config: encodeConfig(c), ShowInDrive: row.ShowInDrive, UpdatedAt: now.UTC(), ID: row.ID}
}

// ---- contracts.RemoteDrives ----

// remotes is what other modules get.
type remotes struct{ m *Module }

var _ contracts.RemoteDrives = remotes{}

func (s remotes) List(ctx context.Context) ([]contracts.RemoteDrive, error) {
	rows, err := s.m.q.ListRemotes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contracts.RemoteDrive, 0, len(rows))
	for _, row := range rows {
		out = append(out, s.m.info(ctx, row))
	}
	return out, nil
}

func (s remotes) Get(ctx context.Context, id int64) (contracts.RemoteDrive, error) {
	row, _, err := s.m.loadRemote(ctx, id)
	if err != nil {
		return contracts.RemoteDrive{}, err
	}
	return s.m.info(ctx, row), nil
}

func (s remotes) WebDAV(ctx context.Context, id int64, folder string) (*files.WebDAV, error) {
	return s.m.openWebDAV(ctx, id, folder)
}

func (s remotes) GDrive(ctx context.Context, id int64, folderID, folderName string) (*files.GDrive, error) {
	return s.m.openGDrive(ctx, id, folderID, folderName)
}

func (s remotes) Import(ctx context.Context, in contracts.RemoteImport) (int64, error) {
	f := remoteForm{kind: in.Kind, name: strings.TrimSpace(in.Name), showInDrive: true, password: in.Password, secret: in.ClientSecret,
		cfg: remoteConfig{URL: in.URL, Username: in.Username, ClientID: in.ClientID, Account: in.Account, Browse: in.Browse, LegacyCallback: in.LegacyCallback}}
	if f.name == "" {
		f.name = defaultName(f.kind, f.cfg)
	}
	row, err := s.m.insertRemote(ctx, f, in.RefreshToken)
	return row.ID, err
}

func (s remotes) StartGDriveAuth(ctx context.Context, id int64, base string) (string, string, error) {
	return s.m.startGDriveAuth(ctx, id, base)
}

func (s remotes) FinishGDriveAuth(ctx context.Context, state, code, googleError string) string {
	return s.m.finishGDriveAuth(ctx, state, code, googleError)
}

func (s remotes) RevokeGDrive(ctx context.Context, id int64) error { return s.m.revokeGDrive(ctx, id) }
