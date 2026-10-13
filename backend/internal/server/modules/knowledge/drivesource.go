package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
)

// DriveSource is the source name of drive files (B199). It is not one of the
// switchable sources: it is on while folders are chosen.
const DriveSource = "drive"

// maxDriveFolders is how many folders can be chosen.
const maxDriveFolders = 20

// indexOrder is the order sources are synced and listed in.
func indexOrder() []string { return append(append([]string{}, AllSources...), DriveSource) }

// fileDoc is a document whose text is read only when its version changed, so
// that a sync does not open every PDF again.
type fileDoc struct {
	contracts.KnowledgeDoc
	Version string
	Load    func() (string, error)
}

// fileSource is a source of fileDocs.
type fileSource interface {
	EachFile(ctx context.Context, opts api.KnowledgeSettings, fn func(fileDoc) error) error
}

// unreadableError marks a file whose text cannot be read. It is not tried
// again until the file changes.
type unreadableError struct{ err error }

func (e unreadableError) Error() string { return e.err.Error() }

// driveSource offers the files in the chosen drive folders. The drive never
// lists hidden or trashed items through contracts.DriveFiles.
type driveSource struct{ files contracts.DriveFiles }

var _ contracts.KnowledgeSource = driveSource{}

func (driveSource) Module() string { return "drive" }

// Each offers nothing: the texts come from EachFile.
func (driveSource) Each(context.Context, func(contracts.KnowledgeDoc) error) error { return nil }

func (s driveSource) EachFile(ctx context.Context, opts api.KnowledgeSettings, fn func(fileDoc) error) error {
	list, err := s.files.ListFiles(ctx, driveFolderIDs(opts), nil)
	if err != nil {
		return err
	}
	for _, f := range list {
		kind := fileKind(f.Name, f.Size)
		if kind == "" {
			continue
		}
		link := "/drive"
		if f.ParentID > 0 {
			link = fmt.Sprintf("/drive?folder=%d", f.ParentID)
		}
		id := f.ID
		doc := fileDoc{
			KnowledgeDoc: contracts.KnowledgeDoc{Ref: strconv.FormatInt(f.ID, 10), Title: f.Name, Link: link, At: f.UpdatedAt},
			Version:      f.SHA256 + "/" + kind + "/" + fileTextVersion,
			Load: func() (string, error) {
				rc, _, err := s.files.Open(ctx, id)
				if err != nil {
					return "", err
				}
				defer rc.Close()
				text, err := fileText(kind, rc)
				if err != nil {
					return "", unreadableError{err}
				}
				return text, nil
			},
		}
		if err := fn(doc); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) driveFiles() (contracts.DriveFiles, bool) {
	return module.Lookup[contracts.DriveFiles](m.d.Registry, contracts.DriveFilesKey)
}

func driveFolderIDs(o api.KnowledgeSettings) []int64 {
	if o.DriveFolders == nil {
		return nil
	}
	ids := make([]int64, 0, len(*o.DriveFolders))
	for _, f := range *o.DriveFolders {
		ids = append(ids, f.Id)
	}
	return ids
}

// cleanDriveFolders checks the chosen folders and keeps only their ids.
func (m *Module) cleanDriveFolders(ctx context.Context, o *api.KnowledgeSettings) error {
	ids := driveFolderIDs(*o)
	if len(ids) == 0 {
		o.DriveFolders = nil
		return nil
	}
	if len(ids) > maxDriveFolders {
		return errors.New("最多选 20 个云盘文件夹")
	}
	files, ok := m.driveFiles()
	if !ok {
		return errors.New("云盘没有开启")
	}
	seen := map[int64]bool{}
	out := []api.KnowledgeDriveFolder{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if _, err := files.Folder(ctx, id); err != nil {
			return errors.New("找不到这个云盘文件夹")
		}
		out = append(out, api.KnowledgeDriveFolder{Id: id})
	}
	o.DriveFolders = &out
	return nil
}

// withFolderPaths fills the read-only paths for the settings page.
func (m *Module) withFolderPaths(ctx context.Context, o api.KnowledgeSettings) api.KnowledgeSettings {
	if o.DriveFolders == nil {
		return o
	}
	files, _ := m.driveFiles()
	out := make([]api.KnowledgeDriveFolder, len(*o.DriveFolders))
	for i, f := range *o.DriveFolders {
		out[i] = api.KnowledgeDriveFolder{Id: f.Id}
		if files != nil {
			if folder, err := files.Folder(ctx, f.Id); err == nil {
				out[i].Path = &folder.Path
			}
		}
	}
	o.DriveFolders = &out
	return o
}
