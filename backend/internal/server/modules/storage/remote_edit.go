package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
)

const remoteEditLimit = 10 << 20
const remoteUploadLimit = 1 << 30

func checkRemoteName(name string) error {
	if strings.TrimSpace(name) != name || utf8.RuneCountInString(name) > 255 || path.Base(name) != name || strings.ContainsAny(name, "\\\x00") {
		return httpx.Invalid("名称不正确")
	}
	if err := files.CheckKey(name); err != nil {
		return httpx.Invalid("名称不正确")
	}
	return nil
}

func (b browser) entry(ctx context.Context, ref string) (files.DirEntry, error) {
	if ref == "" || (b.gdrive != nil && ref == "root") {
		return files.DirEntry{}, files.ErrBadKey
	}
	if b.gdrive != nil {
		return b.gdrive.FileInfo(ctx, ref)
	}
	if err := files.CheckKey(ref); err != nil {
		return files.DirEntry{}, err
	}
	parent := path.Dir(ref)
	if parent == "." {
		parent = ""
	}
	list, err := b.webdav.ReadDir(ctx, parent)
	if err != nil {
		return files.DirEntry{}, err
	}
	for _, e := range list {
		if e.Ref == ref {
			return e, nil
		}
	}
	return files.DirEntry{}, files.ErrNotFound
}

func (m *Module) remoteChanged(r *http.Request, id int64, action, ref string) {
	m.d.Audit.Record(r.Context(), "storage.remote."+action, fmt.Sprint(id), map[string]any{"ref": ref}, nil)
	m.d.Bus.Publish("storage.remotes.updated", map[string]any{"id": id, "ref": ref})
}

func (m *Module) CreateStorageRemoteFolder(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	m.remoteWrite.Lock()
	defer m.remoteWrite.Unlock()
	b, err := m.openBrowser(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.CreateStorageRemoteFolderJSONRequestBody
	if err = httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = checkRemoteName(body.Name); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var entry files.DirEntry
	if b.webdav != nil {
		ref := path.Join(value(body.Parent), body.Name)
		// Check the original parent before Join can clean traversal away.
		if parent := value(body.Parent); parent != "" {
			err = files.CheckKey(parent)
		}
		if err == nil {
			err = b.webdav.Mkdir(r.Context(), ref)
		}
		entry = files.DirEntry{Ref: ref, Name: body.Name, Dir: true}
	} else {
		entry, err = b.gdrive.CreateDir(r.Context(), value(body.Parent), body.Name)
	}
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	m.remoteChanged(r, id, "mkdir", entry.Ref)
	httpx.JSON(w, 201, entryAPI(entry))
}

func (m *Module) RenameStorageRemoteItem(w http.ResponseWriter, r *http.Request, id api.RemoteId) {
	m.remoteWrite.Lock()
	defer m.remoteWrite.Unlock()
	b, err := m.openBrowser(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var body api.RenameStorageRemoteItemJSONRequestBody
	if err = httpx.Decode(r, &body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = checkRemoteName(body.Name); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	entry, err := b.entry(r.Context(), body.Ref)
	if err == nil {
		if b.webdav != nil {
			next := path.Join(path.Dir(body.Ref), body.Name)
			err = b.webdav.Rename(r.Context(), body.Ref, next)
			entry.Ref = next
			entry.Name = body.Name
		} else {
			entry, err = b.gdrive.RenameFile(r.Context(), body.Ref, body.Name)
		}
	}
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	m.remoteChanged(r, id, "rename", entry.Ref)
	httpx.JSON(w, 200, entryAPI(entry))
}

func (m *Module) DeleteStorageRemoteItem(w http.ResponseWriter, r *http.Request, id api.RemoteId, params api.DeleteStorageRemoteItemParams) {
	m.remoteWrite.Lock()
	defer m.remoteWrite.Unlock()
	b, err := m.openBrowser(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err = auth.RequireElevated(r.Context()); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	_, err = b.entry(r.Context(), params.Ref)
	if err == nil {
		if b.webdav != nil {
			err = b.webdav.Delete(r.Context(), params.Ref)
		} else {
			err = b.gdrive.DeleteFile(r.Context(), params.Ref)
		}
	}
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	m.remoteChanged(r, id, "delete", params.Ref)
	w.WriteHeader(204)
}

func remoteETag(body []byte) string { return fmt.Sprintf(`"%x"`, sha256.Sum256(body)) }

func (b browser) readSmall(ctx context.Context, ref string) ([]byte, error) {
	var reader io.ReadCloser
	var err error
	if b.webdav != nil {
		reader, _, err = b.webdav.Get(ctx, ref)
	} else {
		reader, _, err = b.gdrive.OpenFile(ctx, ref)
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	body, err := io.ReadAll(io.LimitReader(reader, remoteEditLimit+1))
	if len(body) > remoteEditLimit {
		return nil, httpx.Invalid("这个文件太大，不能直接编辑")
	}
	return body, err
}

func (m *Module) PutStorageRemoteContent(w http.ResponseWriter, r *http.Request, id api.RemoteId, params api.PutStorageRemoteContentParams) {
	m.remoteWrite.Lock()
	defer m.remoteWrite.Unlock()
	b, err := m.openBrowser(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ref := value(params.Ref)
	parent, name := value(params.Parent), value(params.Name)
	if ref != "" {
		entry, e := b.entry(r.Context(), ref)
		if e != nil {
			httpx.Fail(w, r, browseError(e))
			return
		}
		if entry.Dir || !entry.Downloadable() {
			httpx.Fail(w, r, httpx.Invalid("这个文件不能直接修改"))
			return
		}
		if match := r.Header.Get("If-Match"); match != "" {
			body, e := b.readSmall(r.Context(), ref)
			if e != nil {
				httpx.Fail(w, r, browseError(e))
				return
			}
			if match != remoteETag(body) {
				httpx.Fail(w, r, httpx.NewError(409, "conflict", "文件已经被修改，请重新打开后再保存"))
				return
			}
		}
	} else {
		if err = checkRemoteName(name); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if b.webdav != nil {
			if parent != "" {
				if err = files.CheckKey(parent); err != nil {
					httpx.Fail(w, r, browseError(err))
					return
				}
			}
			ref = path.Join(parent, name)
			if _, e := b.entry(r.Context(), ref); e == nil {
				httpx.Fail(w, r, httpx.NewError(409, "conflict", "已经有同名文件"))
				return
			} else if !isRemoteNotFound(e) {
				httpx.Fail(w, r, browseError(e))
				return
			}
		}
	}
	limit := int64(remoteUploadLimit)
	if value(params.Ref) != "" {
		limit = remoteEditLimit
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	var entry files.DirEntry
	if b.webdav != nil {
		err = b.webdav.Put(r.Context(), ref, r.Body, r.ContentLength)
		if err == nil {
			entry, err = b.entry(r.Context(), ref)
		}
	} else {
		entry, err = b.gdrive.WriteFile(r.Context(), value(params.Ref), parent, name, r.Body, r.ContentLength)
	}
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	m.remoteChanged(r, id, "write", entry.Ref)
	if entry.Size <= remoteEditLimit {
		if saved, err := b.readSmall(r.Context(), entry.Ref); err == nil {
			w.Header().Set("ETag", remoteETag(saved))
		}
	}
	httpx.JSON(w, 200, entryAPI(entry))
}

func remoteHeaders(w http.ResponseWriter, name, mimeType string, inline bool) {
	disposition := "attachment"
	if inline {
		disposition = "inline"
	} else {
		mimeType = "application/octet-stream"
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(path.Ext(name))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	mediaType, _, parseErr := mime.ParseMediaType(mimeType)
	if parseErr != nil {
		mediaType = "application/octet-stream"
	}
	if mediaType != "application/pdf" && !strings.HasPrefix(mediaType, "audio/") && !strings.HasPrefix(mediaType, "video/") && !(strings.HasPrefix(mediaType, "image/") && mediaType != "image/svg+xml") && !strings.HasPrefix(mediaType, "text/plain") {
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Cache-Control", "private, no-store")
}

func (m *Module) serveRemote(w http.ResponseWriter, r *http.Request, b browser, ref string, inline bool) {
	entry, err := b.entry(r.Context(), ref)
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	if entry.Dir || !entry.Downloadable() {
		httpx.Fail(w, r, httpx.Invalid("这个文件不能直接下载"))
		return
	}
	if entry.Size <= remoteEditLimit {
		body, err := b.readSmall(r.Context(), ref)
		if err != nil {
			httpx.Fail(w, r, browseError(err))
			return
		}
		remoteHeaders(w, entry.Name, entry.MimeType, inline)
		w.Header().Set("ETag", remoteETag(body))
		http.ServeContent(w, r, entry.Name, entry.ModTime, bytes.NewReader(body))
		return
	}
	remoteHeaders(w, entry.Name, entry.MimeType, inline)
	if b.webdav != nil {
		reader, info, err := files.OpenSeeker(r.Context(), b.webdav, ref)
		if err != nil {
			httpx.Fail(w, r, browseError(err))
			return
		}
		defer reader.Close()
		http.ServeContent(w, r, entry.Name, info.ModTime, reader)
		return
	}
	resp, _, err := b.gdrive.OpenFileRange(r.Context(), ref, r.Header.Get("Range"))
	if err != nil {
		httpx.Fail(w, r, browseError(err))
		return
	}
	defer resp.Body.Close()
	for _, key := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if val := resp.Header.Get(key); val != "" {
			w.Header().Set(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func isRemoteNotFound(err error) bool { return errors.Is(err, files.ErrNotFound) }
