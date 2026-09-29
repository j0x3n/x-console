package drive_test

import (
	"archive/zip"
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestDownloadDriveZip(t *testing.T) {
	env := testutil.New(t, drive.New)
	first := upload(t, env, "第一份.txt", "alpha", false)
	second := upload(t, env, "second.txt", "beta", false)
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "资料"}, &folder)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(second.Id, 10), map[string]any{"parentId": folder.Id}, nil)
	var empty api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "空文件夹", "parentId": folder.Id}, &empty)

	query := url.Values{}
	query.Add("ids", strconv.FormatInt(first.Id, 10))
	query.Add("ids", strconv.FormatInt(folder.Id, 10))
	resp, err := env.Client.Get(env.URL("/drive/zip?" + query.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/zip" || !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("zip: %d, %q, %s", resp.StatusCode, resp.Header.Get("Content-Disposition"), body)
	}
	_, disposition, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if err != nil || disposition["filename"] != "下载.zip" {
		t.Fatalf("zip filename: %q, %v", disposition["filename"], err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		got[f.Name] = string(data)
	}
	if len(got) != 4 || got["第一份.txt"] != "alpha" || got["资料/second.txt"] != "beta" || got["资料/"] != "" || got["资料/空文件夹/"] != "" {
		t.Fatalf("zip contents: %+v", got)
	}

	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	var hidden api.DriveItem
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(first.Id, 10), map[string]any{"hidden": true}, &hidden)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, _ := env.Do(http.MethodGet, "/drive/zip?ids="+strconv.FormatInt(first.Id, 10), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("locked hidden item: %d", status)
	}
	status, _ = env.Do(http.MethodGet, "/drive/zip?ids=999999", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("missing item: %d", status)
	}
}
