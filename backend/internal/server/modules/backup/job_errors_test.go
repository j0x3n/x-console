package backup

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/backup/repo"
)

func TestBackupJobErrorsDoNotExposeHiddenPaths(t *testing.T) {
	for _, cause := range []error{
		errors.New("notes/private-name storage request failed"),
		fmt.Errorf("notes/private-name: %w", repo.ErrCorrupt),
		fmt.Errorf("drive/private-name: %w", repo.ErrLocked),
	} {
		err := publicBackupError(cause)
		if err == nil || strings.Contains(err.Error(), "private-name") {
			t.Fatal("hidden path leaked", err)
		}
	}
}
