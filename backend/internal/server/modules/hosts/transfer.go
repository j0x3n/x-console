package hosts

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

// MaxUpload is the largest file accepted by PUT /hosts/{id}/files/content.
const MaxUpload = 1 << 30

// DownloadFile is GET /hosts/{hostId}/files/content. The agent streams the
// file in 64 KB chunks after a JSON header.
func (m *Module) DownloadFile(w http.ResponseWriter, r *http.Request, hostID string, params api.DownloadFileParams) {
	ctx := r.Context()
	a, err := m.agentFor(ctx, hostID, protocol.CapFiles)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, err := m.d.Agents.Open(ctx, a.ID, protocol.MethodFilesRead, protocol.FilesReadParams{Path: params.Path})
	if err != nil {
		httpx.Fail(w, r, agentErr(err))
		return
	}
	defer s.Close(nil)
	first, err := s.Recv(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = httpx.NewError(http.StatusBadGateway, "agent_failed", "代理没有返回文件")
		}
		httpx.Fail(w, r, agentErr(err))
		return
	}
	var head protocol.FileHeader
	if err := json.Unmarshal(first, &head); err != nil {
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "agent_failed", "代理返回的数据格式不对"))
		return
	}
	m.d.Audit.Record(ctx, "host.file.download", hostID, map[string]any{"path": params.Path, "size": head.Size}, nil)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(head.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(head.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	for {
		chunk, err := s.Recv(ctx)
		if err != nil {
			// A failure after the header cannot be reported any more; the
			// short body tells the browser the download broke.
			return
		}
		if _, err := w.Write(chunk); err != nil {
			return
		}
	}
}

// UploadFile is PUT /hosts/{hostId}/files/content. The body is forwarded
// in 64 KB chunks; the agent writes a temporary file and renames it into
// place once all bytes arrived.
func (m *Module) UploadFile(w http.ResponseWriter, r *http.Request, hostID string, params api.UploadFileParams) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	size := r.ContentLength
	if size < 0 {
		httpx.Fail(w, r, httpx.NewError(http.StatusLengthRequired, "length_required", "上传需要 Content-Length"))
		return
	}
	if size > MaxUpload {
		httpx.Fail(w, r, httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件不能超过 1 GB"))
		return
	}
	entry, err := m.upload(ctx, hostID, params.Path, size, r.Body)
	m.d.Audit.Record(ctx, "host.file.upload", hostID, map[string]any{"path": params.Path, "size": size}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toAPIEntry(entry))
}

func (m *Module) upload(ctx context.Context, hostID, path string, size int64, body io.Reader) (protocol.FileEntry, error) {
	a, err := m.agentFor(ctx, hostID, protocol.CapFiles)
	if err != nil {
		return protocol.FileEntry{}, err
	}
	s, err := m.d.Agents.Open(ctx, a.ID, protocol.MethodFilesWrite, protocol.FilesWriteParams{Path: path, Size: size})
	if err != nil {
		return protocol.FileEntry{}, agentErr(err)
	}
	defer s.Close(nil)
	// recvErr reads the agent's answer after it ended the stream early.
	recvErr := func() error {
		_, err := s.Recv(ctx)
		if err == nil || errors.Is(err, io.EOF) {
			return httpx.NewError(http.StatusBadGateway, "agent_failed", "代理提前结束了上传")
		}
		return agentErr(err)
	}
	var sent int64
	buf := make([]byte, protocol.FileChunkSize)
	for sent < size {
		if s.Context().Err() != nil {
			return protocol.FileEntry{}, recvErr()
		}
		n, err := io.ReadFull(body, buf[:min(int64(len(buf)), size-sent)])
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if serr := s.Send(ctx, chunk); serr != nil {
				return protocol.FileEntry{}, agentErr(serr)
			}
			sent += int64(n)
		}
		if err != nil {
			if sent < size {
				return protocol.FileEntry{}, httpx.Invalid("上传的数据不完整")
			}
			break
		}
	}
	ack, err := s.Recv(ctx)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return protocol.FileEntry{}, httpx.NewError(http.StatusBadGateway, "agent_failed", "代理没有确认上传")
		}
		return protocol.FileEntry{}, agentErr(err)
	}
	var e protocol.FileEntry
	if err := json.Unmarshal(ack, &e); err != nil {
		return protocol.FileEntry{}, httpx.NewError(http.StatusBadGateway, "agent_failed", "代理返回的数据格式不对")
	}
	return e, nil
}
