package music

import (
	"net/http"
	"slices"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
)

const maxFolders = 20

func (m *Module) settingsDTO(r *http.Request) (api.MusicSettings, error) {
	ids, err := m.folders(r.Context())
	if err != nil {
		return api.MusicSettings{}, err
	}
	drive, err := m.drive()
	if err != nil {
		return api.MusicSettings{}, err
	}
	out := api.MusicSettings{Folders: []api.MusicFolder{}}
	for _, id := range ids {
		f, err := drive.Folder(r.Context(), id)
		if err != nil {
			continue // deleted, trashed or hidden since
		}
		out.Folders = append(out.Folders, api.MusicFolder{Id: f.ID, Name: f.Name, Path: f.Path})
	}
	return out, nil
}

func (m *Module) GetMusicSettings(w http.ResponseWriter, r *http.Request) {
	out, err := m.settingsDTO(r)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) PutMusicSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body api.MusicSettingsInput
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if len(body.Folders) > maxFolders {
		httpx.Fail(w, r, httpx.Invalid("音乐目录太多"))
		return
	}
	drive, err := m.drive()
	if fail(w, r, err) {
		return
	}
	var folders []contracts.DriveFolder
	for _, id := range body.Folders {
		if slices.ContainsFunc(folders, func(f contracts.DriveFolder) bool { return f.ID == id }) {
			continue
		}
		f, err := drive.Folder(ctx, id)
		if err != nil {
			httpx.Fail(w, r, httpx.Invalid("目录不可用，要选云盘里没有隐藏的文件夹"))
			return
		}
		folders = append(folders, f)
	}
	// A folder inside another chosen folder is already covered.
	var ids []int64
	for _, f := range folders {
		inside := slices.ContainsFunc(folders, func(o contracts.DriveFolder) bool {
			return o.ID != f.ID && strings.HasPrefix(f.Path+"/", o.Path+"/")
		})
		if !inside {
			ids = append(ids, f.ID)
		}
	}
	if ids == nil {
		ids = []int64{}
	}
	if fail(w, r, m.d.Settings.Set(ctx, FoldersKey, ids)) {
		return
	}
	m.requestScan()
	out, err := m.settingsDTO(r)
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) ScanMusic(w http.ResponseWriter, r *http.Request) {
	m.requestScan()
	httpx.JSON(w, http.StatusAccepted, api.MusicScanStatus{Running: true})
}
