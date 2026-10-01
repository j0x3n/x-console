package files_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
)

func newGDrive(t *testing.T, fake *fakegdrive.Server, chunk int) *files.GDrive {
	t.Helper()
	g, err := files.NewGDrive(files.GDriveConfig{ClientID: fake.ClientID, ClientSecret: fake.Secret, RefreshToken: fake.RefreshToken,
		FolderName: "X Console 备份", Endpoints: fake.Endpoints(), ChunkSize: chunk})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestGDrive(t *testing.T) {
	fake := fakegdrive.New(t)
	runStoreTests(t, newGDrive(t, fake, 4)) // small chunks, so uploads take several
	if got := fake.Folders(); len(got) != 1 || got[0] != "X Console 备份" {
		t.Fatalf("folders: %v", got)
	}
	if fake.Sessions() != 0 {
		t.Fatalf("failed uploads left %d sessions", fake.Sessions())
	}
	// Replacing a file leaves one file of that name.
	n := 0
	for _, name := range fake.Names("X Console 备份") {
		if name == "notes/attachments/12" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("files named notes/attachments/12: %d", n)
	}
}

func TestGDriveFindsItsFolderAgain(t *testing.T) {
	fake := fakegdrive.New(t)
	put(t, newGDrive(t, fake, 0), "a.tar.gz", "one")
	second := newGDrive(t, fake, 0)
	if got := keys(t, second, ""); len(got) != 1 || got[0] != "a.tar.gz" {
		t.Fatalf("list: %v", got)
	}
	folder, err := second.Folder(context.Background())
	if err != nil || folder == "" {
		t.Fatalf("folder: %q %v", folder, err)
	}
	if got := fake.Folders(); len(got) != 1 {
		t.Fatalf("folders: %v", got)
	}
	account, err := second.Account(context.Background())
	if err != nil || account != "me@gmail.com" {
		t.Fatalf("account: %q %v", account, err)
	}
}

func TestGDriveExpiredToken(t *testing.T) {
	fake := fakegdrive.New(t)
	fake.Expire()
	g := newGDrive(t, fake, 0)
	if err := g.Put(context.Background(), "a", strings.NewReader("x"), 1); !errors.Is(err, files.ErrGDriveAuth) {
		t.Fatalf("put: %v", err)
	}
	if err := g.Check(context.Background()); err == nil || err.Error() != files.ErrGDriveAuth.Error() {
		t.Fatalf("check: %v", err)
	}
}

func TestGDriveOAuth(t *testing.T) {
	fake := fakegdrive.New(t)
	ctx := context.Background()
	q := fakegdrive.AuthURL(t, files.GoogleAuthURL(fake.Endpoints(), "client", "https://x/cb", "st"))
	if q.Get("scope") != files.GoogleDriveScope || q.Get("state") != "st" || q.Get("access_type") != "offline" || q.Get("redirect_uri") != "https://x/cb" {
		t.Fatalf("auth url: %v", q)
	}
	token, err := files.GoogleExchange(ctx, fake.Endpoints(), "client", "secret", "code", "https://x/cb")
	if err != nil || token != fake.RefreshToken {
		t.Fatalf("exchange: %q %v", token, err)
	}
	if _, err := files.GoogleExchange(ctx, fake.Endpoints(), "client", "secret", "wrong", "https://x/cb"); err == nil {
		t.Fatal("wrong code should fail")
	}
	if _, err := files.GoogleExchange(ctx, fake.Endpoints(), "client", "nope", "code", "https://x/cb"); err == nil || !strings.Contains(err.Error(), "密钥") {
		t.Fatalf("wrong secret: %v", err)
	}
	if err := files.GoogleRevoke(ctx, fake.Endpoints(), token); err != nil {
		t.Fatal(err)
	}
	if got := fake.Revoked(); len(got) != 1 || got[0] != token {
		t.Fatalf("revoked: %v", got)
	}
}

func TestGDriveCheck(t *testing.T) {
	fake := fakegdrive.New(t)
	if err := newGDrive(t, fake, 0).Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := fake.Names("X Console 备份"); len(got) != 0 {
		t.Fatalf("check left files: %v", got)
	}
	for _, c := range []files.GDriveConfig{{ClientSecret: "s", RefreshToken: "r", FolderName: "f"}, {ClientID: "c", ClientSecret: "s", FolderName: "f"},
		{ClientID: "c", ClientSecret: "s", RefreshToken: "r"}} {
		if _, err := files.NewGDrive(c); err == nil {
			t.Errorf("config %+v should be rejected", c)
		}
	}
	// An empty file still makes a file.
	g := newGDrive(t, fake, 0)
	if err := g.Put(context.Background(), "empty", strings.NewReader(""), 0); err != nil {
		t.Fatal(err)
	}
	rc, info, err := g.Get(context.Background(), "empty")
	if err != nil || info.Size != 0 {
		t.Fatalf("empty: %+v %v", info, err)
	}
	io.Copy(io.Discard, rc)
	rc.Close()
}
