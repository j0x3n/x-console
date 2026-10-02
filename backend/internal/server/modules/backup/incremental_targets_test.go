package backup_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func TestIncrementalWebDAVAndGoogleDrive(t *testing.T) {
	for _, kind := range []string{"webdav", "gdrive"} {
		t.Run(kind, func(t *testing.T) {
			env, _ := setup(t)
			env.Elevate()
			var id int64
			var fake *fakegdrive.Server
			if kind == "webdav" {
				srv, _ := davServer(t)
				id = addWebDAV(t, env, srv)
			} else {
				fake = fakegdrive.New(t)
				storageModule(t, env).UseGoogle(fake.Endpoints())
				id = addGDrive(t, env, fake)
			}
			env.MustDo(http.MethodPut, "/backups/settings", map[string]any{"target": "remote", "remoteId": id, "mode": "incremental", "retention": map[string]int{"last": 1, "daily": 0, "weekly": 0, "monthly": 0}}, nil)
			putFile(t, env, "projects/a", "one")
			for _, content := range []string{"one", "two"} {
				putFile(t, env, "projects/a", content)
				env.MustDo(http.MethodPost, "/backups/run", nil, nil)
				if j := waitJob(t, env); j.State != api.Done {
					t.Fatalf("%s: %+v", kind, j)
				}
			}
			var list struct {
				Items []map[string]any
				Stats repo.Stats
			}
			env.MustDo(http.MethodGet, "/backups/snapshots", nil, &list)
			if len(list.Items) != 1 || list.Stats.Snapshots != 1 {
				t.Fatal(list)
			}
			env.MustDo(http.MethodPost, "/backups/check", nil, nil)
			if j := waitJob(t, env); j.State != api.Done {
				t.Fatal(j)
			}
			if fake != nil {
				for _, name := range fake.Names("X Console 备份") {
					if !strings.HasPrefix(name, "x-console-repo/") {
						t.Fatal(name)
					}
				}
			}
			if _, err := env.App.Deps.Files.Store().Stat(context.Background(), "projects/a"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
