package drive_test

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func listVersions(t *testing.T, env *testutil.Env, id int64) []api.DriveVersion {
	t.Helper()
	var out struct {
		Items []api.DriveVersion `json:"items"`
	}
	env.MustDo(http.MethodGet, "/drive/items/"+strconv.FormatInt(id, 10)+"/versions", nil, &out)
	return out.Items
}

func TestDriveVersionListRangeAndRestore(t *testing.T) {
	env := testutil.New(t, drive.New)
	file := upload(t, env, "version.txt", "first", false)
	resp, body := saveContent(t, env, file.Id, "second", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save second: %d %s", resp.StatusCode, body)
	}
	resp, body = saveContent(t, env, file.Id, "third", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save third: %d %s", resp.StatusCode, body)
	}
	versions := listVersions(t, env, file.Id)
	if len(versions) != 2 || versions[0].Size != 6 || versions[1].Size != 5 {
		t.Fatalf("version order: %+v", versions)
	}
	path := "/drive/items/" + strconv.FormatInt(file.Id, 10) + "/versions/" + strconv.FormatInt(versions[1].Id, 10) + "/content"
	req, err := http.NewRequest(http.MethodGet, env.URL(path), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=1-3")
	res, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusPartialContent || string(data) != "irs" || res.Header.Get("Content-Range") != "bytes 1-3/5" {
		t.Fatalf("version range: %d %q %+v", res.StatusCode, data, res.Header)
	}
	status, _ := env.Do(http.MethodGet, "/drive/items/"+strconv.FormatInt(file.Id, 10)+"/versions/999999/content", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("missing version: %d", status)
	}
	var restored api.DriveItem
	respStatus, raw := env.Do(http.MethodPost, "/drive/items/"+strconv.FormatInt(file.Id, 10)+"/versions/"+strconv.FormatInt(versions[1].Id, 10)+"/restore", nil, &restored)
	if respStatus != http.StatusOK || restored.Size != 5 {
		t.Fatalf("restore: %d %s", respStatus, raw)
	}
	content, tag := readContent(t, env, file.Id)
	if content != "first" || tag == "" {
		t.Fatalf("restored content: %q %q", content, tag)
	}
	versions = listVersions(t, env, file.Id)
	if len(versions) != 3 || versions[0].Size != 5 || versions[1].Size != 6 || versions[2].Size != 5 {
		t.Fatalf("restored versions: %+v", versions)
	}
	var folder api.DriveItem
	env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"name": "无版本"}, &folder)
	status, _ = env.Do(http.MethodGet, "/drive/items/"+strconv.FormatInt(folder.Id, 10)+"/versions", nil, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("folder versions: %d", status)
	}
}

func TestDriveVersionSettingsPruneAndVault(t *testing.T) {
	env := testutil.New(t, drive.New)
	var settings api.DriveVersionSettings
	env.MustDo(http.MethodGet, "/drive/version-settings", nil, &settings)
	if settings.KeepCount != 50 || settings.KeepDays != 30 {
		t.Fatalf("default settings: %+v", settings)
	}
	status, _ := env.Do(http.MethodPut, "/drive/version-settings", map[string]any{"keepCount": 0, "keepDays": 30}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("bad settings: %d", status)
	}
	file := upload(t, env, "prune.txt", "one", false)
	for _, value := range []string{"two", "three", "four"} {
		resp, body := saveContent(t, env, file.Id, value, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("save: %d %s", resp.StatusCode, body)
		}
	}
	if len(listVersions(t, env, file.Id)) != 3 {
		t.Fatal("expected three old versions")
	}
	env.MustDo(http.MethodPut, "/drive/version-settings", map[string]any{"keepCount": 1, "keepDays": 30}, &settings)
	if settings.KeepCount != 1 {
		t.Fatalf("saved settings: %+v", settings)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(listVersions(t, env, file.Id)) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(listVersions(t, env, file.Id)) != 1 {
		t.Fatalf("prune count: %+v", listVersions(t, env, file.Id))
	}
	env.MustDo(http.MethodPost, "/vault/setup", map[string]string{"password": "secret123"}, nil)
	hidden := upload(t, env, "private.txt", "secret", true)
	resp, body := saveContent(t, env, hidden.Id, "secret2", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("hidden save: %d %s", resp.StatusCode, body)
	}
	env.MustDo(http.MethodPost, "/vault/lock", nil, nil)
	status, raw := env.Do(http.MethodGet, "/drive/items/"+strconv.FormatInt(hidden.Id, 10)+"/versions", nil, nil)
	if status != http.StatusNotFound || !strings.Contains(string(raw), "not_found") {
		t.Fatalf("locked versions: %d %s", status, raw)
	}
}
