package drive

import (
	"context"
	"database/sql"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

// recordVersion stores the content that is about to be replaced. Call it in
// the same transaction as the replacement, after reading the current row.
func (m *Module) recordVersion(ctx context.Context, tx *sql.Tx, item db.DriveItem, nextHash string) error {
	if item.IsDir != 0 || item.Sha256 == "" || item.Sha256 == nextHash {
		return nil
	}
	var latest string
	err := tx.QueryRowContext(ctx, "SELECT sha256 FROM drive_file_versions WHERE item_id=? ORDER BY created_at DESC,id DESC LIMIT 1", item.ID).Scan(&latest)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && latest == item.Sha256 {
		return nil
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO drive_file_versions(item_id,size,sha256,created_at) VALUES(?,?,?,?)", item.ID, item.Size, item.Sha256, time.Now().UTC())
	return err
}
