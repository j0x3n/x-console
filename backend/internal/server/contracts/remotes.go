package contracts

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

// B69: the WebDAV and Google Drive accounts live in the storage module.
// Backups and the drive page both use them.

const (
	RemoteDrivesKey = "storage.remotes"    // B69 storage provides, backup uses
	RemoteUserKey   = "backup.remote_user" // B69 backup provides, storage uses
)

// Kinds of remote drive accounts.
const (
	RemoteWebDAV = "webdav"
	RemoteGDrive = "gdrive"
)

// RemoteDrive is one saved account, without its secrets.
type RemoteDrive struct {
	ID          int64
	Kind        string // RemoteWebDAV or RemoteGDrive
	Name        string
	Account     string // WebDAV user name, or the Google account
	Ready       bool   // everything needed to connect is saved (Google: authorized)
	Limited     bool   // Google grant can only see files the panel made
	ShowInDrive bool
}

// RemoteImport is an account copied from the B63 backup settings.
type RemoteImport struct {
	Kind, Name                           string
	URL, Username, Password              string // WebDAV
	ClientID, ClientSecret, RefreshToken string // Google Drive
	Account                              string
	Browse                               bool
	// LegacyCallback keeps the old /backups/gdrive/callback address, so the
	// redirect URI saved at Google still matches.
	LegacyCallback bool
}

// RemoteDrives is provided by the storage module.
type RemoteDrives interface {
	List(ctx context.Context) ([]RemoteDrive, error)
	// Get fails with an httpx 404 when the account does not exist.
	Get(ctx context.Context, id int64) (RemoteDrive, error)
	// WebDAV opens a WebDAV account. folder is the root of the store ("" is
	// the top of the address).
	WebDAV(ctx context.Context, id int64, folder string) (*files.WebDAV, error)
	// GDrive opens a Google Drive account with the folder used as its store.
	GDrive(ctx context.Context, id int64, folderID, folderName string) (*files.GDrive, error)
	// Import saves an account taken over from older settings.
	Import(ctx context.Context, in RemoteImport) (int64, error)
	// StartGDriveAuth begins a Google authorization. base is the site
	// address the browser uses, like https://x.example.com.
	StartGDriveAuth(ctx context.Context, id int64, base string) (authURL, redirectURI string, err error)
	// FinishGDriveAuth handles the browser coming back from Google and
	// returns where to send it next.
	FinishGDriveAuth(ctx context.Context, state, code, googleError string) string
	RevokeGDrive(ctx context.Context, id int64) error
}

// RemoteUser is provided by the backup module: an account in use cannot be
// deleted.
type RemoteUser interface {
	UsesRemote(ctx context.Context, id int64) (bool, error)
}
