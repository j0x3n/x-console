package contracts

import (
	"context"
	"errors"
	"io"
	"time"
)

// DriveFilesKey is where the drive module registers DriveFiles (B146).
const DriveFilesKey = "drive.files"

// ErrDriveNotFound is returned when a file or folder does not exist, is in the
// trash or is hidden. Callers treat all three the same.
var ErrDriveNotFound = errors.New("drive: not found")

// DriveFile is one file in the drive.
type DriveFile struct {
	ID         int64
	ParentID   int64 // 0 for the drive root
	ParentName string
	Name       string
	Size       int64
	Mime       string
	SHA256     string
	UpdatedAt  time.Time
}

// DriveFolder is one folder in the drive.
type DriveFolder struct {
	ID   int64
	Name string
	Path string // "/音乐/周杰伦"
}

// DriveFiles lets other modules read the drive without knowing how it stores
// content. Hidden (vault) and trashed items are never visible through it,
// whatever the vault lock state is.
type DriveFiles interface {
	// File returns a file. Folders return ErrDriveNotFound.
	File(ctx context.Context, id int64) (DriveFile, error)
	// Folder returns a folder.
	Folder(ctx context.Context, id int64) (DriveFolder, error)
	// Open opens a file's content.
	Open(ctx context.Context, id int64) (io.ReadSeekCloser, DriveFile, error)
	// ListFiles returns every file under the given folders, at any depth.
	// exts are lower-case extensions with the dot (".mp3"); empty means all.
	ListFiles(ctx context.Context, folderIDs []int64, exts []string) ([]DriveFile, error)
}
