package drive_test

import (
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestDriveTaskAPI(t *testing.T) {
	env := testutil.New(t, drive.New)
	var out struct{ Items []api.DriveTask }
	env.MustDo(http.MethodGet, "/drive/tasks", nil, &out)
	if out.Items == nil || len(out.Items) != 0 {
		t.Fatalf("empty task list: %+v", out.Items)
	}
	status, _ := env.Do(http.MethodPost, "/drive/tasks/missing/cancel", nil, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cancel missing task: %d", status)
	}
}
