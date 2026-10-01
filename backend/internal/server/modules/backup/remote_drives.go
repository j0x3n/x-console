package backup

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
)

// B68 的旧地址。B69 起网盘账号在存储模块里，可以有多个。这里每种只给第一个，
// 浏览和下载跳到新地址。下个版本删掉。

var errNoRemote = httpx.NewError(http.StatusNotFound, "not_found", "找不到这个网盘")

func (m *Module) remoteHidden(ctx context.Context, kind string) bool {
	h, ok := module.Lookup[contracts.HiddenModules](m.d.Registry, contracts.HiddenModulesKey)
	return ok && (h.Hidden(ctx, "drive") || h.Hidden(ctx, "drive-"+kind))
}

// firstRemotes is the first visible account of each kind.
func (m *Module) firstRemotes(ctx context.Context) (map[string]contracts.RemoteDrive, error) {
	rd, err := m.remotes()
	if err != nil {
		return nil, err
	}
	list, err := rd.List(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]contracts.RemoteDrive{}
	for _, a := range list {
		if _, seen := out[a.Kind]; seen || !a.Ready || !a.ShowInDrive || m.remoteHidden(ctx, a.Kind) {
			continue
		}
		out[a.Kind] = a
	}
	return out, nil
}

// ListRemoteDrives is GET /remote-drives (old, B68).
func (m *Module) ListRemoteDrives(w http.ResponseWriter, r *http.Request) {
	first, err := m.firstRemotes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	out := []api.RemoteDrive{}
	for _, kind := range []string{contracts.RemoteWebDAV, contracts.RemoteGDrive} {
		a, ok := first[kind]
		if !ok {
			continue
		}
		d := api.RemoteDrive{Id: api.RemoteDriveId(kind), Name: a.Name, Limited: a.Limited}
		if a.Account != "" {
			d.Account = ptr(a.Account)
		}
		out = append(out, d)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (m *Module) redirectRemote(w http.ResponseWriter, r *http.Request, remote api.Remote, what string, ref *string) {
	first, err := m.firstRemotes(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	a, ok := first[string(remote)]
	if !ok {
		httpx.Fail(w, r, errNoRemote)
		return
	}
	target := fmt.Sprintf("/api/v1/storage/remotes/%d/%s", a.ID, what)
	if ref != nil {
		target += "?" + url.Values{"ref": {*ref}}.Encode()
	}
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}

// ListRemoteDriveItems is GET /remote-drives/{remote}/items (old, B68).
func (m *Module) ListRemoteDriveItems(w http.ResponseWriter, r *http.Request, remote api.Remote, params api.ListRemoteDriveItemsParams) {
	m.redirectRemote(w, r, remote, "items", params.Ref)
}

// DownloadRemoteDriveFile is GET /remote-drives/{remote}/download (old, B68).
func (m *Module) DownloadRemoteDriveFile(w http.ResponseWriter, r *http.Request, remote api.Remote, params api.DownloadRemoteDriveFileParams) {
	m.redirectRemote(w, r, remote, "download", &params.Ref)
}
