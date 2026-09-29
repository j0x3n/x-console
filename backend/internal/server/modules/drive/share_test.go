package drive_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestDriveShareCreateListDeleteAndState(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "公开.txt", "hello", false)
	var share api.DriveShare
	status, raw := env.Do(http.MethodPost, "/drive/shares", map[string]any{"itemId": file.Id, "expiresIn": "7d", "code": "Ab1234", "maxDownloads": 2}, &share)
	if status != http.StatusCreated || share.Id == 0 || len(share.Token) != 22 || share.Code == nil || *share.Code != "Ab1234" || !share.Active || !strings.HasSuffix(share.Url, "/s/"+share.Token) {
		t.Fatalf("created share: %d %s %+v", status, raw, share)
	}
	var sealed string
	if err := env.App.Deps.DB.QueryRow("SELECT code_sealed FROM drive_shares WHERE id=?", share.Id).Scan(&sealed); err != nil || sealed == "Ab1234" || sealed == "" {
		t.Fatalf("code at rest: %q %v", sealed, err)
	}
	var out struct {
		Items []api.DriveShare `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/shares?itemId="+strconv.FormatInt(file.Id, 10), nil, &out)
	if len(out.Items) != 1 || out.Items[0].Code == nil || *out.Items[0].Code != "Ab1234" {
		t.Fatalf("share list: %+v", out.Items)
	}
	var listed struct {
		Items []api.DriveItem `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/items", nil, &listed)
	if len(listed.Items) != 1 || listed.Items[0].Shared == nil || !*listed.Items[0].Shared {
		t.Fatalf("shared flag: %+v", listed.Items)
	}
	env.MustDo(http.MethodDelete, "/drive/items/"+strconv.FormatInt(file.Id, 10), nil, nil)
	env.MustDo(http.MethodGet, "/drive/shares", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Active {
		t.Fatalf("trashed share state: %+v", out.Items)
	}
	env.MustDo(http.MethodDelete, "/drive/shares/"+strconv.FormatInt(share.Id, 10), nil, nil)
	env.MustDo(http.MethodGet, "/drive/shares", nil, &out)
	if len(out.Items) != 0 {
		t.Fatalf("deleted share: %+v", out.Items)
	}
	status, _ = env.Do(http.MethodDelete, "/drive/shares/"+strconv.FormatInt(share.Id, 10), nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("delete missing share: %d", status)
	}
}

func TestDriveShareRejectsHiddenAndBadInput(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "普通.txt", "x", false)
	for _, body := range []map[string]any{
		{"itemId": file.Id, "expiresIn": "oops"},
		{"itemId": file.Id, "expiresIn": "never", "code": "bad!"},
		{"itemId": file.Id, "expiresIn": "never", "maxDownloads": 0},
	} {
		status, _ := env.Do(http.MethodPost, "/drive/shares", body, nil)
		if status != http.StatusBadRequest {
			t.Fatalf("bad share input %v: %d", body, status)
		}
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	hidden := upload(t, env, "隐藏.txt", "secret", true)
	status, raw := env.Do(http.MethodPost, "/drive/shares", map[string]any{"itemId": hidden.Id, "expiresIn": "never"}, nil)
	if status != http.StatusBadRequest || !strings.Contains(string(raw), "share_not_allowed") {
		t.Fatalf("hidden share: %d %s", status, raw)
	}
}
