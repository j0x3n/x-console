package drive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

func (m *Module) ListDriveShareDownloads(w http.ResponseWriter, r *http.Request, shareID int64) {
	ctx := r.Context()
	var itemID int64
	err := m.d.DB.QueryRowContext(ctx, "SELECT item_id FROM drive_shares WHERE id=?", shareID).Scan(&itemID)
	if errors.Is(err, sql.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	if fail(w, r, err) {
		return
	}
	if _, err = m.visibleRow(ctx, itemID); fail(w, r, err) {
		return
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT at,ip,user_agent,item_name FROM drive_share_downloads WHERE share_id=? ORDER BY at DESC,id DESC LIMIT 20", shareID)
	if fail(w, r, err) {
		return
	}
	defer rows.Close()
	items := []api.DriveShareDownload{}
	for rows.Next() {
		var entry api.DriveShareDownload
		var ip, agent, name string
		if err = rows.Scan(&entry.At, &ip, &agent, &name); err != nil {
			break
		}
		entry.Ip = maskedShareIP(ip)
		entry.UserAgent = shareBrowser(agent)
		if name != "" {
			entry.ItemName = &name
		}
		items = append(items, entry)
	}
	if err == nil {
		err = rows.Err()
	}
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func maskedShareIP(raw string) string {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return "*"
	}
	ip = ip.Unmap()
	if ip.Is4() {
		bytes := ip.As4()
		return fmt.Sprintf("%d.%d.*.*", bytes[0], bytes[1])
	}
	bytes := ip.As16()
	return fmt.Sprintf("%x:%x:*", uint16(bytes[0])<<8|uint16(bytes[1]), uint16(bytes[2])<<8|uint16(bytes[3]))
}

func shareBrowser(agent string) string {
	switch {
	case strings.Contains(agent, "Edg/") || strings.Contains(agent, "EdgA/") || strings.Contains(agent, "EdgiOS/"):
		return "Edge"
	case strings.Contains(agent, "OPR/") || strings.Contains(agent, "Opera/"):
		return "Opera"
	case strings.Contains(agent, "Firefox/") || strings.Contains(agent, "FxiOS/"):
		return "Firefox"
	case strings.Contains(agent, "Chrome/") || strings.Contains(agent, "CriOS/"):
		return "Chrome"
	case strings.Contains(agent, "Safari/"):
		return "Safari"
	default:
		return "其他"
	}
}

func (m *Module) recordShareDownload(ctx context.Context, share db.DriveShare, item db.DriveItem, r *http.Request, key string) error {
	m.shareMu.Lock()
	defer m.shareMu.Unlock()
	if at, ok := m.shareFetches[key]; key != "" && ok && time.Since(at) < shareFetchWindow {
		return nil
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, "UPDATE drive_shares SET downloads=downloads+1,last_access_at=? WHERE id=? AND (max_downloads IS NULL OR downloads<max_downloads)", now, share.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return errShareLimit
	}
	agent := r.UserAgent()
	if len(agent) > 512 {
		agent = agent[:512]
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO drive_share_downloads(share_id,at,ip,user_agent,item_id,item_name) VALUES(?,?,?,?,?,?)", share.ID, now, publicClientIP(r), agent, item.ID, item.Name); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM drive_share_downloads WHERE share_id=? AND id NOT IN (SELECT id FROM drive_share_downloads WHERE share_id=? ORDER BY at DESC,id DESC LIMIT 100)", share.ID, share.ID); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if key != "" {
		m.shareFetches[key] = now
	}
	m.d.Bus.Publish("drive_share.changed", map[string]any{"id": share.ID, "itemId": share.ItemID})
	return nil
}

type shareDownloadWriter struct {
	http.ResponseWriter
	request *http.Request
	count   func(int) error
	wrote   bool
	blocked bool
}

func (w *shareDownloadWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	if status == http.StatusOK || status == http.StatusPartialContent {
		if err := w.count(status); err != nil {
			w.blocked = true
			for _, name := range []string{"Content-Length", "Content-Range", "Content-Type", "Content-Disposition", "ETag", "Last-Modified", "Accept-Ranges"} {
				w.Header().Del(name)
			}
			httpx.Fail(w.ResponseWriter, w.request, err)
			return
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *shareDownloadWriter) Write(data []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.blocked {
		return len(data), nil
	}
	return w.ResponseWriter.Write(data)
}

func (w *shareDownloadWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
