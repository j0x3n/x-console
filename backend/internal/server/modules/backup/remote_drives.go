package backup

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

// The drive page browses the WebDAV and Google Drive accounts of the backup
// settings (B68). Only reading: list folders and download files.

var errNoRemote = httpx.NewError(http.StatusNotFound, "not_found", "找不到这个网盘")

// remoteHidden tells whether the drive page, or this one drive tab, is
// hidden for the request. Hidden drives look like they do not exist; the
// backups keep going.
func (m *Module) remoteHidden(ctx context.Context, id string) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && (h.Hidden(ctx, "drive") || h.Hidden(ctx, "drive-"+id))
}

// webdavName is the tab name of a WebDAV address.
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

// remoteDrives lists the bound drives the request may see.
func (m *Module) remoteDrives(ctx context.Context, s settingsData) []api.RemoteDrive {
	out := []api.RemoteDrive{}
	if s.WebDAV.URL != "" && !m.remoteHidden(ctx, targetWebDAV) {
		d := api.RemoteDrive{Id: api.RemoteDriveIdWebdav, Name: webdavName(s.WebDAV.URL)}
		if s.WebDAV.Username != "" {
			d.Account = ptr(s.WebDAV.Username)
		}
		out = append(out, d)
	}
	if s.GDrive.ClientID != "" && m.secret(ctx, keyGDriveToken) != "" && !m.remoteHidden(ctx, targetGDrive) {
		d := api.RemoteDrive{Id: api.RemoteDriveIdGdrive, Name: "Google Drive", Limited: !s.GDrive.Browse}
		if s.GDrive.Account != "" {
			d.Account = ptr(s.GDrive.Account)
		}
		out = append(out, d)
	}
	return out
}

// ListRemoteDrives is GET /remote-drives.
func (m *Module) ListRemoteDrives(w http.ResponseWriter, r *http.Request) {
	s, err := m.loadSettings(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": m.remoteDrives(r.Context(), s)})
}

// browser is one opened drive.
type browser struct {
	webdav *files.WebDAV
	gdrive *files.GDrive
}

func (m *Module) openBrowser(ctx context.Context, remote api.Remote) (browser, error) {
	s, err := m.loadSettings(ctx)
	if err != nil {
		return browser{}, err
	}
	id := string(remote)
	found := false
	for _, d := range m.remoteDrives(ctx, s) {
		found = found || string(d.Id) == id
	}
	if !found {
		return browser{}, errNoRemote
	}
	if id == targetWebDAV {
		w, err := files.NewWebDAV(files.WebDAVConfig{URL: s.WebDAV.URL, Username: s.WebDAV.Username, Password: m.secret(ctx, keyWebDAVPassword)})
		if err != nil {
			return browser{}, httpx.Invalid(err.Error())
		}
		return browser{webdav: w}, nil
	}
	g, err := m.openGDrive(ctx, s, "")
	if err != nil {
		return browser{}, err
	}
	return browser{gdrive: g}, nil
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

func entryAPI(e files.DirEntry) api.RemoteDriveEntry {
	out := api.RemoteDriveEntry{Ref: e.Ref, Name: e.Name, IsDir: e.Dir, Downloadable: e.Downloadable()}
	if !e.Dir {
		out.Size = ptr(e.Size)
	}
	if !e.ModTime.IsZero() {
		out.ModifiedAt = ptr(e.ModTime.UTC())
	}
	return out
}

// ListRemoteDriveItems is GET /remote-drives/{remote}/items.
func (m *Module) ListRemoteDriveItems(w http.ResponseWriter, r *http.Request, remote api.Remote, params api.ListRemoteDriveItemsParams) {
	ctx := r.Context()
	b, err := m.openBrowser(ctx, remote)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref := strings.Trim(value(params.Ref), "/")
	out := api.RemoteDriveListing{Ref: ref, Items: []api.RemoteDriveEntry{}}
	type crumb = struct {
		Name string `json:"name"`
		Ref  string `json:"ref"`
	}
	out.Trail = []crumb{}
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

// DownloadRemoteDriveFile is GET /remote-drives/{remote}/download.
func (m *Module) DownloadRemoteDriveFile(w http.ResponseWriter, r *http.Request, remote api.Remote, params api.DownloadRemoteDriveFileParams) {
	ctx := r.Context()
	b, err := m.openBrowser(ctx, remote)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref := strings.Trim(params.Ref, "/")
	disposition := func(name string) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
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
