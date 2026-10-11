package storage_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestConcurrentRemoteSavesKeepTheFirstVersion(t *testing.T) {
	env := testutil.New(t, storage.New)
	service, _ := module.Lookup[contracts.RemoteDrives](env.App.Deps.Registry, contracts.RemoteDrivesKey)
	srv := davServer(t, "existing.txt")
	id, err := service.Import(context.Background(), contracts.RemoteImport{Kind: contracts.RemoteWebDAV, Name: "测试", URL: srv.URL + "/dav/", Username: "me", Password: davPassword})
	if err != nil {
		t.Fatal(err)
	}
	base := fmt.Sprintf("/storage/remotes/%d", id)
	preview := remoteRequest(t, env, "GET", base+"/download?ref=docs/existing.txt&inline=true", "", "")
	io.Copy(io.Discard, preview.Body)
	preview.Body.Close()
	etag := preview.Header.Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}
	statuses := make(chan int, 2)
	for _, text := range []string{"first change", "second change"} {
		req, err := http.NewRequest("PUT", env.URL(base+"/content?ref=docs/existing.txt"), strings.NewReader(text))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Requested-With", "x-console")
		req.Header.Set("If-Match", etag)
		go func() {
			resp, err := env.Client.Do(req)
			if err != nil {
				statuses <- 0
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	a, b := <-statuses, <-statuses
	if !((a == 200 && b == 409) || (a == 409 && b == 200)) {
		t.Fatalf("concurrent writes: %d, %d", a, b)
	}
}
