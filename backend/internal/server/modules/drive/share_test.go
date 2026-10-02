package drive_test

import (
	"net/http"
	"net/url"
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
	status, _ := env.Do(http.MethodPost, "/drive/shares", map[string]any{"itemId": file.Id, "expiresIn": "7d"}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("share without elevation: %d", status)
	}
	env.Elevate()
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
	env.Elevate()
	for _, body := range []map[string]any{
		{"itemId": file.Id, "expiresIn": "oops"},
		{"itemId": file.Id, "expiresIn": "never", "code": "bad"},
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

func TestHiddenSharedItemLeavesShareListWhileLocked(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "秘密合同.pdf", "x", false)
	env.Elevate()
	env.MustDo(http.MethodPost, "/drive/shares", map[string]any{"itemId": file.Id, "expiresIn": "7d"}, nil)
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	env.MustDo(http.MethodPatch, "/drive/items/"+strconv.FormatInt(file.Id, 10), map[string]any{"hidden": true}, nil)
	var out struct {
		Items []api.DriveShare `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/shares", nil, &out)
	if len(out.Items) != 1 || out.Items[0].Active {
		t.Fatalf("unlocked share list: %+v", out.Items)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	_, raw := env.Do(http.MethodGet, "/drive/shares", nil, nil)
	if strings.Contains(string(raw), "秘密") {
		t.Fatalf("hidden name in locked share list: %s", raw)
	}
}

func TestSharedFlagInFoldersAndSearch(t *testing.T) {
	env := testutil.New(t, drive.New)
	var outer, inner api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "外"}, &outer)
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "内", "parentId": outer.Id}, &inner)
	shared := upload(t, env, "报告-已分享.txt", "a", false)
	plain := upload(t, env, "报告-未分享.txt", "b", false)
	for _, file := range []api.DriveItem{shared, plain} {
		env.MustDo(http.MethodPatch, "/drive/items/"+itoa(file.Id), map[string]any{"parentId": inner.Id}, nil)
	}
	env.Elevate()
	env.MustDo(http.MethodPost, "/drive/shares", map[string]any{"itemId": shared.Id, "expiresIn": "7d"}, nil)
	for _, path := range []string{"/drive/items?parent=" + itoa(inner.Id), "/drive/items?q=" + url.QueryEscape("报告")} {
		var out struct {
			Items []api.DriveItem `json:"items"`
		}
		env.MustDo(http.MethodGet, path, nil, &out)
		if len(out.Items) != 2 {
			t.Fatalf("%s: %+v", path, out.Items)
		}
		for _, item := range out.Items {
			if item.Shared == nil || *item.Shared != (item.Id == shared.Id) {
				t.Fatalf("%s: shared flag of %s: %v", path, item.Name, item.Shared)
			}
		}
	}
}
