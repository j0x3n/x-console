package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files/fakegdrive"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func remoteRequest(t *testing.T, env *testutil.Env, method, path, body, etag string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, env.URL(path), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Requested-With", "x-console")
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestRemoteFileOperations(t *testing.T) {
	for _, kind := range []string{contracts.RemoteWebDAV, contracts.RemoteGDrive} {
		t.Run(kind, func(t *testing.T) {
			env := testutil.New(t, storage.New)
			service, _ := module.Lookup[contracts.RemoteDrives](env.App.Deps.Registry, contracts.RemoteDrivesKey)
			config := contracts.RemoteImport{Kind: kind, Name: "测试网盘"}
			if kind == contracts.RemoteWebDAV {
				srv := davServer(t, "existing.txt")
				config.URL, config.Username, config.Password = srv.URL+"/dav/", "me", davPassword
			} else {
				fake := fakegdrive.New(t)
				m, _ := module.Lookup[*storage.Module](env.App.Deps.Registry, storage.ServiceKey)
				m.UseGoogle(fake.Endpoints())
				config.ClientID, config.ClientSecret, config.RefreshToken = fake.ClientID, fake.Secret, fake.RefreshToken
			}
			id, err := service.Import(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			base := fmt.Sprintf("/storage/remotes/%d", id)
			var folder api.StorageRemoteEntry
			env.MustDo("POST", base+"/items", map[string]any{"name": "目录"}, &folder)
			if !folder.IsDir || folder.Ref == "" {
				t.Fatal(folder)
			}
			if status, _ := env.Do("POST", base+"/items", map[string]any{"name": "../越界"}, nil); status != 400 {
				t.Fatal(status)
			}
			put := remoteRequest(t, env, "PUT", base+"/content?parent="+url.QueryEscape(folder.Ref)+"&name=hello.txt", "first", "")
			if put.StatusCode != 200 {
				raw, _ := io.ReadAll(put.Body)
				t.Fatalf("upload %d %s", put.StatusCode, raw)
			}
			var listing api.StorageRemoteListing
			env.MustDo("GET", base+"/items?ref="+url.QueryEscape(folder.Ref), nil, &listing)
			if len(listing.Items) != 1 {
				t.Fatal(listing)
			}
			file := listing.Items[0]
			query := "?ref=" + url.QueryEscape(file.Ref)
			get := remoteRequest(t, env, "GET", base+"/download"+query+"&inline=true", "", "")
			raw, _ := io.ReadAll(get.Body)
			if string(raw) != "first" || !strings.HasPrefix(get.Header.Get("Content-Disposition"), "inline") || get.Header.Get("ETag") == "" {
				t.Fatalf("preview %s %v", raw, get.Header)
			}
			match := get.Header.Get("ETag")
			changed := remoteRequest(t, env, "PUT", base+"/content"+query, "second", match)
			if changed.StatusCode != 200 {
				t.Fatal(changed.StatusCode)
			}
			stale := remoteRequest(t, env, "PUT", base+"/content"+query, "lost", match)
			if stale.StatusCode != 409 {
				t.Fatal(stale.StatusCode)
			}
			var renamed api.StorageRemoteEntry
			env.MustDo("PATCH", base+"/items", map[string]any{"ref": file.Ref, "name": "renamed.txt"}, &renamed)
			if renamed.Name != "renamed.txt" {
				t.Fatal(renamed)
			}
			deleteURL := base + "/items?ref=" + url.QueryEscape(renamed.Ref)
			if code, _ := env.Do("DELETE", deleteURL, nil, nil); code != 403 {
				t.Fatalf("delete without elevation: %d", code)
			}
			env.Elevate()
			env.MustDo("DELETE", deleteURL, nil, nil)
			if code, _ := env.Do("GET", base+"/download?ref="+url.QueryEscape(renamed.Ref), nil, nil); code != 404 {
				t.Fatal(code)
			}
			if kind == contracts.RemoteWebDAV {
				for _, ref := range []string{"../escape", "/absolute", "docs/../../outside"} {
					if code, _ := env.Do("GET", base+"/download?ref="+url.QueryEscape(ref), nil, nil); code != 400 {
						t.Fatalf("traversal %q %d", ref, code)
					}
				}
			}
		})
	}
}
