package drive

import (
	"context"
	"io"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/logfollow"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

const driveFollowChunk = 256 << 10

func (m *Module) FollowDriveItem(w http.ResponseWriter, r *http.Request, itemID api.ItemId, params api.FollowDriveItemParams) {
	if params.Offset < 0 {
		httpx.Fail(w, r, httpx.Invalid("offset 不能小于 0"))
		return
	}
	item, err := m.visibleRow(r.Context(), itemID)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.TrashedAt != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(ws.CloseRead(r.Context()), time.Hour)
	defer cancel()
	offset := params.Offset
	lastHash := ""
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		current, err := m.row(ctx, itemID)
		if err != nil || current.TrashedAt != nil || current.IsDir != 0 || (current.Hidden != 0 && !auth.VaultUnlocked(ctx)) || (current.Hidden != 0 && item.Hidden == 0) {
			return
		}
		if current.Sha256 != lastHash {
			if current.Size < offset {
				if err := ws.Write(context.Background(), websocket.MessageText, logfollow.Reset()); err != nil {
					return
				}
				offset = 0
			}
			if err := m.sendDriveFollow(ctx, ws, current, &offset); err != nil {
				if ctx.Err() == nil {
					_ = ws.Close(websocket.StatusInternalError, "读取日志失败")
				}
				return
			}
			lastHash = current.Sha256
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Module) sendDriveFollow(ctx context.Context, ws *websocket.Conn, item db.DriveItem, offset *int64) error {
	if item.Sha256 == "" {
		return nil
	}
	for *offset < item.Size {
		if err := ctx.Err(); err != nil {
			return err
		}
		length := min(item.Size-*offset, int64(driveFollowChunk))
		stream, _, err := m.store.GetRange(ctx, blobKey(item.Sha256), *offset, length)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(io.LimitReader(stream, length))
		closeErr := stream.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if len(data) == 0 {
			return nil
		}
		usable := utf8Prefix(data)
		if usable == 0 {
			return nil
		}
		if err := ws.Write(context.Background(), websocket.MessageText, logfollow.Append(*offset, data[:usable])); err != nil {
			return err
		}
		*offset += int64(usable)
	}
	return nil
}

func utf8Prefix(data []byte) int {
	i := 0
	for i < len(data) {
		if !utf8.FullRune(data[i:]) {
			break
		}
		_, size := utf8.DecodeRune(data[i:])
		i += size
	}
	return i
}
