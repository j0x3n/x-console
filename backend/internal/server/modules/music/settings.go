package music

import (
	"context"
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
	o := m.options(r.Context())
	if o.Focus.PlaylistID != 0 {
		// A playlist that was deleted since is no longer the focus playlist.
		if _, err := m.q.GetPlaylist(r.Context(), o.Focus.PlaylistID); err != nil {
			o.Focus.PlaylistID = 0
		}
	}
	out := api.MusicSettings{Folders: []api.MusicFolder{}, AutoMatch: o.AutoMatch, WriteBack: o.WriteBack, Providers: o.Providers,
		Focus: api.MusicFocusSettings{AutoPlay: o.Focus.AutoPlay, PlaylistId: o.Focus.PlaylistID, AutoPause: o.Focus.AutoPause, OnlyFocusList: o.Focus.OnlyFocusList}}
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
	if body.Folders != nil {
		ids, err := m.checkFolders(ctx, *body.Folders)
		if fail(w, r, err) {
			return
		}
		if fail(w, r, m.d.Settings.Set(ctx, FoldersKey, ids)) {
			return
		}
	}
	if body.AutoMatch != nil || body.WriteBack != nil || body.Providers != nil || body.Focus != nil {
		o := m.options(ctx)
		if body.AutoMatch != nil {
			o.AutoMatch = *body.AutoMatch
		}
		if body.WriteBack != nil {
			o.WriteBack = *body.WriteBack
		}
		if f := body.Focus; f != nil {
			if f.AutoPlay != nil {
				o.Focus.AutoPlay = *f.AutoPlay
			}
			if f.AutoPause != nil {
				o.Focus.AutoPause = *f.AutoPause
			}
			if f.OnlyFocusList != nil {
				o.Focus.OnlyFocusList = *f.OnlyFocusList
			}
			if f.PlaylistId != nil {
				if *f.PlaylistId != 0 {
					if _, err := m.playlistSummary(ctx, m.q, *f.PlaylistId); err != nil {
						httpx.Fail(w, r, httpx.Invalid("播放列表不存在"))
						return
					}
				}
				o.Focus.PlaylistID = *f.PlaylistId
			}
		}
		if body.Providers != nil {
			for name, on := range *body.Providers {
				if _, known := o.Providers[name]; !known {
					httpx.Fail(w, r, httpx.Invalid("不认识的来源: "+name))
					return
				}
				o.Providers[name] = on
			}
		}
		if fail(w, r, m.d.Settings.Set(ctx, OptionsKey, o)) {
			return
		}
	}
	if body.Folders != nil {
		m.requestScan()
	}
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

// checkFolders validates the chosen folders and returns the ids to save: no
// duplicates, and a folder inside another chosen folder is left out.
func (m *Module) checkFolders(ctx context.Context, wanted []int64) ([]int64, error) {
	if len(wanted) > maxFolders {
		return nil, httpx.Invalid("音乐目录太多")
	}
	drive, err := m.drive()
	if err != nil {
		return nil, err
	}
	var folders []contracts.DriveFolder
	for _, id := range wanted {
		if slices.ContainsFunc(folders, func(f contracts.DriveFolder) bool { return f.ID == id }) {
			continue
		}
		f, err := drive.Folder(ctx, id)
		if err != nil {
			return nil, httpx.Invalid("目录不可用，要选云盘里没有隐藏的文件夹")
		}
		folders = append(folders, f)
	}
	ids := []int64{}
	for _, f := range folders {
		inside := slices.ContainsFunc(folders, func(o contracts.DriveFolder) bool {
			return o.ID != f.ID && strings.HasPrefix(f.Path+"/", o.Path+"/")
		})
		if !inside {
			ids = append(ids, f.ID)
		}
	}
	return ids, nil
}
