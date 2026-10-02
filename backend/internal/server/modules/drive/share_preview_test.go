package drive_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestSharePreviewAndDownloadHistory(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "preview.md", "# hello", false)
	share := createPublicTestShare(t, env, file.Id, "密码 !🙂长度123", 1)
	base := "/public/shares/" + share.Token
	resp, raw := publicRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"code": "密码 !🙂长度123"}, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d %s", resp.StatusCode, raw)
	}
	var unlocked struct{ Access string }
	if err := json.Unmarshal(raw, &unlocked); err != nil {
		t.Fatal(err)
	}
	content := base + "/content?t=" + unlocked.Access
	for _, preview := range []string{"&preview=true", "&inline=1", "&preview=true"} {
		resp, raw = publicRequest(t, env, http.MethodGet, content+preview, nil, nil)
		if resp.StatusCode != http.StatusOK || string(raw) != "# hello" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") || resp.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("preview: %d %s %v", resp.StatusCode, raw, resp.Header)
		}
	}
	var history struct{ Items []api.DriveShareDownload }
	env.MustDo(http.MethodGet, "/drive/shares/"+itoa(share.Id)+"/downloads", nil, &history)
	if len(history.Items) != 0 {
		t.Fatalf("preview counted: %+v", history.Items)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"User-Agent": "Mozilla/5.0 Chrome/123.0 Safari/537.36", "X-Forwarded-For": "203.0.113.44"})
	if resp.StatusCode != http.StatusOK || string(raw) != "# hello" {
		t.Fatalf("download: %d %s", resp.StatusCode, raw)
	}
	env.MustDo(http.MethodGet, "/drive/shares/"+itoa(share.Id)+"/downloads", nil, &history)
	if len(history.Items) != 1 || history.Items[0].Ip != "127.0.*.*" || history.Items[0].UserAgent != "Chrome" || history.Items[0].ItemName == nil || *history.Items[0].ItemName != "preview.md" {
		t.Fatalf("history: %+v", history.Items)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, content+"&preview=true", nil, nil)
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("spent preview: %d %s", resp.StatusCode, raw)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"Range": "bytes=2-", "User-Agent": "Mozilla/5.0 Chrome/123.0 Safari/537.36", "X-Forwarded-For": "203.0.113.44"})
	if resp.StatusCode != http.StatusPartialContent || string(raw) != "hello" {
		t.Fatalf("resume: %d %s", resp.StatusCode, raw)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, "/drive/shares/"+itoa(share.Id)+"/downloads", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("public history: %d %s", resp.StatusCode, raw)
	}
}

func TestShareDownloadCountsOnlyValidStarts(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "ranges.txt", "0123456789", false)
	share := createPublicTestShare(t, env, file.Id, "", 0)
	content := "/public/shares/" + share.Token + "/content"
	resp, body := publicRequest(t, env, http.MethodHead, content, nil, nil)
	if resp.StatusCode != http.StatusOK || len(body) != 0 || resp.Header.Get("Content-Length") != "10" {
		t.Fatalf("HEAD: %d %q %v", resp.StatusCode, body, resp.Header)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"Range": "bytes=90-100"})
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("bad range: %d", resp.StatusCode)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"If-None-Match": "*"})
	if resp.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional: %d", resp.StatusCode)
	}
	for _, value := range []string{"bytes=2-", "bytes=-2"} {
		resp, _ = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"Range": value})
		if resp.StatusCode != http.StatusPartialContent {
			t.Fatalf("resume %s: %d", value, resp.StatusCode)
		}
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT downloads FROM drive_shares WHERE id=?", share.Id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("non-start count %d: %v", count, err)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"Range": "bytes=0-2", "User-Agent": "Firefox/123"})
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, content, nil, map[string]string{"Range": "bytes=3-", "If-Range": "\"stale\"", "User-Agent": "Safari/123"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stale if-range: %d", resp.StatusCode)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT downloads FROM drive_shares WHERE id=?", share.Id).Scan(&count); err != nil || count != 2 {
		t.Fatalf("starts count %d: %v", count, err)
	}
}

func TestShareConcurrentRequestsCountOnce(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "parallel.txt", "parallel", false)
	share := createPublicTestShare(t, env, file.Id, "", 1)
	var group sync.WaitGroup
	statuses := make(chan int, 12)
	for i := 0; i < 12; i++ {
		group.Go(func() {
			resp, _ := publicRequest(t, env, http.MethodGet, "/public/shares/"+share.Token+"/content", nil, nil)
			statuses <- resp.StatusCode
		})
	}
	group.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK {
			t.Fatalf("concurrent download status %d", status)
		}
	}
	var downloads, records int
	err := env.App.Deps.DB.QueryRow("SELECT downloads,(SELECT count(*) FROM drive_share_downloads WHERE share_id=drive_shares.id) FROM drive_shares WHERE id=?", share.Id).Scan(&downloads, &records)
	if err != nil || downloads != 1 || records != 1 {
		t.Fatalf("concurrent totals %d/%d: %v", downloads, records, err)
	}
}

func TestShareThumbnailScopePasswordAndHiddenHistory(t *testing.T) {
	env := testutil.New(t, drive.New)
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "images"}, &folder)
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&data, img); err != nil {
		t.Fatal(err)
	}
	inside := upload(t, env, "inside.png", data.String(), false)
	outside := upload(t, env, "outside.png", data.String(), false)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(inside.Id), map[string]any{"parentId": folder.Id}, nil)
	share := createPublicTestShare(t, env, folder.Id, "四个汉字", 0)
	base := "/public/shares/" + share.Token
	resp, _ := publicRequest(t, env, http.MethodGet, base+"/thumbnail?item="+itoa(inside.Id), nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("thumbnail missing password %d", resp.StatusCode)
	}
	resp, raw := publicRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"code": "四个汉字"}, nil)
	var unlocked struct{ Access string }
	if err := json.Unmarshal(raw, &unlocked); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d %s %v", resp.StatusCode, raw, err)
	}
	for id, expected := range map[int64]int{inside.Id: http.StatusOK, outside.Id: http.StatusNotFound} {
		resp, raw = publicRequest(t, env, http.MethodGet, base+"/thumbnail?item="+itoa(id)+"&t="+unlocked.Access, nil, nil)
		if resp.StatusCode != expected {
			t.Fatalf("thumbnail %d: %d %s", id, resp.StatusCode, raw)
		}
	}
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/items?t="+unlocked.Access, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"thumbnail":true`) {
		t.Fatalf("listing thumbnail: %d %s", resp.StatusCode, raw)
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodPatch, "/drive/items/"+itoa(folder.Id), map[string]any{"hidden": true}, nil)
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, raw := env.Do(http.MethodGet, "/drive/shares/"+itoa(share.Id)+"/downloads", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("hidden history: %d %s", status, raw)
	}
}

func TestShareHistoryReturnsTwentyAndMasksIPv6(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "history.txt", "x", false)
	share := createPublicTestShare(t, env, file.Id, "", 0)
	for i := 0; i < 105; i++ {
		_, err := env.App.Deps.DB.Exec("INSERT INTO drive_share_downloads(share_id,at,ip,user_agent,item_id,item_name) VALUES(?,?,?,?,?,?)", share.Id, time.Now().UTC().Add(time.Duration(i)*time.Second), "2001:db8:1234:5678::1", "Firefox/123", file.Id, fmt.Sprintf("item-%d", i))
		if err != nil {
			t.Fatal(err)
		}
	}
	var out struct{ Items []api.DriveShareDownload }
	env.MustDo(http.MethodGet, "/drive/shares/"+strconv.FormatInt(share.Id, 10)+"/downloads", nil, &out)
	if len(out.Items) != 20 || out.Items[0].Ip != "2001:db8:*" || out.Items[0].UserAgent != "Firefox" || out.Items[0].ItemName == nil || *out.Items[0].ItemName != "item-104" {
		t.Fatalf("recent history: %+v", out.Items)
	}
	resp, raw := publicRequest(t, env, http.MethodGet, "/public/shares/"+share.Token+"/content", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download: %d %s", resp.StatusCode, raw)
	}
	var count int
	if err := env.App.Deps.DB.QueryRow("SELECT count(*) FROM drive_share_downloads WHERE share_id=?", share.Id).Scan(&count); err != nil || count != 100 {
		t.Fatalf("retained %d: %v", count, err)
	}
}
