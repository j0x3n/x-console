package hosts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/logfollow"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

const maxFileRange = 1 << 20

// JSON can expand one byte to a six-byte escape, so 32 KiB keeps a text frame
// below the 256 KiB contract limit even for control-heavy input.
const followChunk = 32 << 10

// A follow ends after followLimit, like the drive's log viewer, and when the
// session is gone (checked every followSessionCheck).
const followLimit = time.Hour

var followSessionCheck = 30 * time.Second // tests shorten it

func (m *Module) ReadFileRange(w http.ResponseWriter, r *http.Request, hostID api.HostId, p api.ReadFileRangeParams) {
	if p.Path == "" || p.Offset < 0 || p.Length <= 0 || p.Length > maxFileRange {
		httpx.Fail(w, r, httpx.Invalid("文件路径、偏移或读取长度不对，最多读取 1 MB"))
		return
	}
	a, err := m.agentFor(r.Context(), hostID, protocol.CapFilesRange)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	size, data, err := m.readFileRange(r.Context(), a.ID, p.Path, p.Offset, int64(p.Length))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	w.Header().Set("X-File-Size", strconv.FormatInt(size, 10))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (m *Module) readFileRange(ctx context.Context, agentID, path string, offset, length int64) (int64, []byte, error) {
	s, err := m.d.Agents.Open(ctx, agentID, protocol.MethodFilesRead, protocol.FilesReadParams{Path: path, Offset: offset, Length: length})
	if err != nil {
		return 0, nil, agentErr(err)
	}
	defer s.Close(nil)
	first, err := s.Recv(ctx)
	if err != nil {
		return 0, nil, agentErr(err)
	}
	var h protocol.FileHeader
	if json.Unmarshal(first, &h) != nil || h.Size < 0 {
		return 0, nil, httpx.NewError(http.StatusBadGateway, "agent_failed", "代理返回的文件信息不对")
	}
	data := make([]byte, 0, min(length, maxFileRange))
	for {
		chunk, err := s.Recv(ctx)
		if errors.Is(err, io.EOF) {
			return h.Size, data, nil
		}
		if err != nil {
			return 0, nil, agentErr(err)
		}
		if int64(len(data))+int64(len(chunk)) > length {
			return 0, nil, httpx.NewError(http.StatusBadGateway, "agent_failed", "代理返回的数据超过读取长度")
		}
		data = append(data, chunk...)
	}
}

func (m *Module) fileSize(ctx context.Context, agentID, path string) (int64, error) {
	var entry protocol.FileEntry
	if err := m.d.Agents.Call(ctx, agentID, protocol.MethodFilesStat, protocol.FilesPathParams{Path: path}, &entry); err != nil {
		return 0, agentErr(err)
	}
	if entry.Type != "file" || entry.Size < 0 {
		return 0, httpx.Invalid("只能跟随普通文件")
	}
	return entry.Size, nil
}

func (m *Module) FollowHostFile(w http.ResponseWriter, r *http.Request, hostID api.HostId, p api.FollowHostFileParams) {
	if p.Path == "" || p.Offset < 0 {
		httpx.Fail(w, r, httpx.Invalid("文件路径或偏移不对"))
		return
	}
	a, err := m.agentFor(r.Context(), hostID, protocol.CapFilesRange)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if _, err := m.fileSize(r.Context(), a.ID, p.Path); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer ws.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithTimeout(ws.CloseRead(r.Context()), followLimit)
	defer cancel()
	offset := p.Offset
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	checked := time.Now()
	for {
		if time.Since(checked) >= followSessionCheck {
			if !m.d.Auth.SessionActive(ctx) {
				_ = ws.Close(websocket.StatusPolicyViolation, "登录已失效")
				return
			}
			checked = time.Now()
		}
		if err := m.followFileOnce(ctx, ws, a, p.Path, &offset); err != nil {
			if ctx.Err() == nil {
				_ = ws.Close(websocket.StatusInternalError, "读取文件失败")
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Module) followFileOnce(ctx context.Context, ws *websocket.Conn, a agenthub.Agent, path string, offset *int64) error {
	size, err := m.fileSize(ctx, a.ID, path)
	if err != nil {
		return err
	}
	if size < *offset {
		if err := ws.Write(context.Background(), websocket.MessageText, logfollow.Reset()); err != nil {
			return err
		}
		*offset = 0
	}
	for *offset < size {
		length := min(size-*offset, followChunk)
		actualSize, data, err := m.readFileRange(ctx, a.ID, path, *offset, length)
		if err != nil {
			return err
		}
		if actualSize < *offset {
			if err := ws.Write(context.Background(), websocket.MessageText, logfollow.Reset()); err != nil {
				return err
			}
			*offset = 0
			return nil
		}
		usable := logfollow.UTF8Prefix(data)
		if usable == 0 {
			return nil // empty, or a rune still being written; read it again next time
		}
		if err := ws.Write(context.Background(), websocket.MessageText, logfollow.Append(*offset, data[:usable])); err != nil {
			return err
		}
		*offset += int64(usable)
	}
	return nil
}
