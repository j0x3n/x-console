package repo_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/webdav"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakes3"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func TestRepositoryOnRemoteStores(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"s3", "webdav", "gdrive"} {
		t.Run(kind, func(t *testing.T) {
			var target files.Store
			var err error
			switch kind {
			case "s3":
				srv := fakes3.New(t, "backup")
				target, err = files.NewS3(files.S3Config{Endpoint: srv.URL, Region: "us-east-1", Bucket: "backup", Prefix: "site", AccessKeyID: "AK", SecretAccessKey: "SK", PathStyle: true})
			case "webdav":
				h := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.NewMemFS(), LockSystem: webdav.NewMemLS()}
				srv := httptest.NewServer(h)
				t.Cleanup(srv.Close)
				target, err = files.NewWebDAV(files.WebDAVConfig{URL: srv.URL + "/dav/"})
			case "gdrive":
				srv := fakegdrive.New(t)
				var drive *files.GDrive
				drive, err = files.NewGDrive(files.GDriveConfig{ClientID: srv.ClientID, ClientSecret: srv.Secret, RefreshToken: srv.RefreshToken, FolderName: "backups", Endpoints: srv.Endpoints()})
				if err == nil {
					_, err = drive.Folder(ctx)
				}
				target = drive
			}
			if err != nil {
				t.Fatal(err)
			}
			target = files.Scoped(files.Scoped(target, "backups"), "x-console-repo")
			r, lease, err := repo.Acquire(ctx, target, bytes.Repeat([]byte{1}, 32), t.TempDir(), "backup", true)
			if err != nil {
				t.Fatal(err)
			}
			defer lease.Close()
			source := files.Local{Root: t.TempDir()}
			content := "unchanged"
			if err := source.Put(ctx, "notes/attachment", strings.NewReader(content), int64(len(content))); err != nil {
				t.Fatal(err)
			}
			var entries []files.Info
			for info, err := range source.List(ctx, "") {
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, info)
			}
			at := time.Now().UTC()
			in := func() repo.Input {
				return repo.Input{Database: strings.NewReader("database"), DatabaseSize: 8, Files: source, Entries: entries, Migration: "1", CreatedAt: at}
			}
			first, err := r.CreateSnapshot(lease.Context(), in())
			if err != nil {
				t.Fatal(err)
			}
			at = at.Add(time.Minute)
			second, err := r.CreateSnapshot(lease.Context(), in())
			if err != nil {
				t.Fatal(err)
			}
			info, err := target.Stat(ctx, "snapshots/"+second.ID+".json")
			if err != nil || second.UploadedBytes != info.Size {
				t.Fatal("unchanged backup uploaded blocks", second.UploadedBytes, info.Size, err)
			}
			if _, err := r.Check(lease.Context()); err != nil {
				t.Fatal(err)
			}
			var db bytes.Buffer
			dest := files.Local{Root: t.TempDir()}
			if _, err := r.Restore(lease.Context(), first.ID, &db, dest, nil); err != nil || db.String() != "database" {
				t.Fatal(db.String(), err)
			}
			rc, _, err := dest.Get(ctx, "notes/attachment")
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(rc)
			rc.Close()
			if err != nil || string(body) != content {
				t.Fatal(string(body), err)
			}
			if result, err := r.Prune(lease.Context(), repo.Retention{Last: 1}, at, time.UTC); err != nil || result.Snapshots != 1 || result.Blocks != 0 {
				t.Fatal(result, err)
			}
		})
	}
}

func TestRemoteStoreListingFailureIsReported(t *testing.T) {
	for _, kind := range []string{"s3", "webdav", "gdrive"} {
		t.Run(kind, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"access_token":"token","expires_in":3600}`)
					return
				}
				w.WriteHeader(http.StatusForbidden)
			}))
			defer srv.Close()
			var s files.Store
			var err error
			switch kind {
			case "s3":
				s, err = files.NewS3(files.S3Config{Endpoint: srv.URL, Region: "us-east-1", Bucket: "backup", AccessKeyID: "AK", SecretAccessKey: "SK", PathStyle: true})
			case "webdav":
				s, err = files.NewWebDAV(files.WebDAVConfig{URL: srv.URL + "/dav/"})
			case "gdrive":
				s, err = files.NewGDrive(files.GDriveConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", FolderID: "folder", Endpoints: files.GoogleEndpoints{API: srv.URL, Token: srv.URL + "/token"}})
			}
			if err != nil {
				t.Fatal(err)
			}
			failed := false
			for _, err := range s.List(context.Background(), "blobs") {
				if err != nil {
					failed = true
				}
			}
			if !failed {
				t.Fatal("remote listing failure became an empty list")
			}
		})
	}
}
