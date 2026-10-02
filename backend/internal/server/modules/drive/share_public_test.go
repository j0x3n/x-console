package drive_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func publicRequest(t *testing.T, env *testutil.Env, method, path string, body any, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		input = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, env.URL(path), input)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	client := http.DefaultClient
	if ip := headers["X-Forwarded-For"]; ip != "" {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			dialer := &net.Dialer{LocalAddr: &net.TCPAddr{IP: net.ParseIP("127.0.0." + parts[3])}}
			transport := &http.Transport{DialContext: dialer.DialContext}
			defer transport.CloseIdleConnections()
			client = &http.Client{Transport: transport}
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, raw
}

func createPublicTestShare(t *testing.T, env *testutil.Env, itemID int64, code string, limit int) api.DriveShare {
	t.Helper()
	env.Elevate()
	body := map[string]any{"itemId": itemID, "expiresIn": "never"}
	if code != "" {
		body["code"] = code
	}
	if limit != 0 {
		body["maxDownloads"] = limit
	}
	var share api.DriveShare
	env.MustDo(http.MethodPost, "/drive/shares", body, &share)
	return share
}

func TestPublicShareContentRangeLimitAndHeaders(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "unsafe.html", "<script>alert(1)</script>", false)
	share := createPublicTestShare(t, env, file.Id, "", 1)
	base := "/public/shares/" + share.Token
	resp, raw := publicRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "unsafe.html") || resp.Header.Get("Content-Security-Policy") != "default-src 'none'; sandbox" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("public metadata: %d %s %+v", resp.StatusCode, raw, resp.Header)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/content", nil, map[string]string{"Range": "bytes=1-3"})
	if resp.StatusCode != http.StatusPartialContent || string(raw) != "scr" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("range content: %d %q %+v", resp.StatusCode, raw, resp.Header)
	}
	var downloads int
	if err := env.App.Deps.DB.QueryRow("SELECT downloads FROM drive_shares WHERE id=?", share.Id).Scan(&downloads); err != nil || downloads != 0 {
		t.Fatalf("range download count: %d %v", downloads, err)
	}
	// The same client keeps its download: more ranges and the whole file
	// are served and not counted again.
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/content", nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "alert") {
		t.Fatalf("download: %d %s", resp.StatusCode, raw)
	}
	// Another client is over the limit.
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/content", nil, map[string]string{"Range": "bytes=1-3", "X-Forwarded-For": "203.0.113.9"})
	if resp.StatusCode != http.StatusGone || !strings.Contains(string(raw), "share_limit_reached") {
		t.Fatalf("limit: %d %s", resp.StatusCode, raw)
	}
	if err := env.App.Deps.DB.QueryRow("SELECT downloads FROM drive_shares WHERE id=?", share.Id).Scan(&downloads); err != nil || downloads != 1 {
		t.Fatalf("download count: %d %v", downloads, err)
	}
}

func TestPublicShareCodeAccessAndLockout(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "private.txt", "private", false)
	share := createPublicTestShare(t, env, file.Id, "A1234", 0)
	base := "/public/shares/" + share.Token
	resp, raw := publicRequest(t, env, http.MethodGet, base, nil, nil)
	if resp.StatusCode != http.StatusUnauthorized || strings.Contains(string(raw), "private.txt") {
		t.Fatalf("missing code: %d %s", resp.StatusCode, raw)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		resp, raw = publicRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"code": "wrong"}, nil)
		want := http.StatusForbidden
		if attempt == 5 {
			want = http.StatusTooManyRequests
		}
		if resp.StatusCode != want {
			t.Fatalf("wrong code attempt %d: %d %s", attempt, resp.StatusCode, raw)
		}
	}
	resp, raw = publicRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"code": "A1234"}, nil)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("locked: %d %s", resp.StatusCode, raw)
	}
	// Another IP can still unlock, and the signed token works without a session.
	resp, raw = publicRequest(t, env, http.MethodPost, base+"/unlock", map[string]string{"code": "A1234"}, map[string]string{"X-Forwarded-For": "192.0.2.5"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlock: %d %s", resp.StatusCode, raw)
	}
	var unlocked struct {
		Access string `json:"access"`
	}
	if err := json.Unmarshal(raw, &unlocked); err != nil || unlocked.Access == "" {
		t.Fatalf("access: %s %v", raw, err)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, base+"?t="+unlocked.Access, nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("signed access: %d", resp.StatusCode)
	}
	resp, _ = publicRequest(t, env, http.MethodGet, base+"?t="+unlocked.Access+"x", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered access: %d", resp.StatusCode)
	}
	env.MustDo(http.MethodDelete, "/drive/shares/"+strconv.FormatInt(share.Id, 10), nil, nil)
	resp, raw = publicRequest(t, env, http.MethodGet, base+"?t="+unlocked.Access, nil, nil)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(string(raw), "share_not_found") {
		t.Fatalf("revoked access: %d %s", resp.StatusCode, raw)
	}
}

func TestPublicShareFolderScopeAndZip(t *testing.T) {
	env := testutil.New(t, drive.New)
	var folder, child api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "公开目录"}, &folder)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "子目录", "parentId": folder.Id}, &child)
	inside := upload(t, env, "inside.txt", "hello", false)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(inside.Id, 10), map[string]any{"parentId": child.Id}, nil)
	outside := upload(t, env, "outside.txt", "outside", false)
	share := createPublicTestShare(t, env, folder.Id, "", 0)
	base := "/public/shares/" + share.Token
	resp, raw := publicRequest(t, env, http.MethodGet, base+"/items?folder="+strconv.FormatInt(child.Id, 10), nil, nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "inside.txt") || !strings.Contains(string(raw), "子目录") {
		t.Fatalf("nested listing: %d %s", resp.StatusCode, raw)
	}
	for _, url := range []string{base + "/items?folder=" + strconv.FormatInt(outside.Id, 10), base + "/content?item=" + strconv.FormatInt(outside.Id, 10)} {
		resp, _ = publicRequest(t, env, http.MethodGet, url, nil, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("out of scope %s: %d", url, resp.StatusCode)
		}
	}
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/content?item="+strconv.FormatInt(inside.Id, 10), nil, nil)
	if resp.StatusCode != http.StatusOK || string(raw) != "hello" {
		t.Fatalf("nested download: %d %s", resp.StatusCode, raw)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/zip", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("public zip: %d %s", resp.StatusCode, raw)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, entry := range zr.File {
		if entry.Name == "公开目录/子目录/inside.txt" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("zip entries: %+v", zr.File)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, base+"/content?item="+strconv.FormatInt(inside.Id, 10), nil, nil)
	if resp.StatusCode != http.StatusOK || string(raw) != "hello" {
		t.Fatalf("download after zip: %d %s", resp.StatusCode, raw)
	}
	// The file (once, the second fetch by the same client within an hour is
	// the same download) and the zip.
	var downloads int
	if err := env.App.Deps.DB.QueryRow("SELECT downloads FROM drive_shares WHERE id=?", share.Id).Scan(&downloads); err != nil || downloads != 2 {
		t.Fatalf("folder downloads: %d %v", downloads, err)
	}
}

func TestPublicShareRateLimitAndHiddenFolder(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "rate.txt", "x", false)
	share := createPublicTestShare(t, env, file.Id, "", 0)
	base := "/public/shares/" + share.Token
	for i := 0; i < 60; i++ {
		resp, _ := publicRequest(t, env, http.MethodGet, base, nil, map[string]string{"X-Forwarded-For": "198.51.100.3"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: %d", i, resp.StatusCode)
		}
	}
	resp, raw := publicRequest(t, env, http.MethodGet, base, nil, map[string]string{"X-Forwarded-For": "198.51.100.3"})
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" || !strings.Contains(string(raw), "rate_limited") {
		t.Fatalf("rate limit: %d %s", resp.StatusCode, raw)
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "普通文件夹"}, &folder)
	child := upload(t, env, "private.txt", "secret", false)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(child.Id, 10), map[string]any{"parentId": folder.Id}, nil)
	share = createPublicTestShare(t, env, folder.Id, "", 0)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(child.Id, 10), map[string]any{"hidden": true}, nil)
	resp, raw = publicRequest(t, env, http.MethodGet, "/public/shares/"+share.Token+"/items", nil, nil)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(raw), "private.txt") {
		t.Fatalf("hidden child leaked: %d %s", resp.StatusCode, raw)
	}
	resp, raw = publicRequest(t, env, http.MethodGet, "/public/shares/"+share.Token+"/zip", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hidden zip: %d %s", resp.StatusCode, raw)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range zr.File {
		if strings.Contains(entry.Name, "private.txt") {
			t.Fatalf("hidden zip entry: %s", entry.Name)
		}
	}
}
