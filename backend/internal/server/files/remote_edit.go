package files

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Mkdir and Rename operate below the configured WebDAV root.
func (w *WebDAV) Mkdir(ctx context.Context, key string) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	return w.mkcol(ctx, key)
}
func (w *WebDAV) Rename(ctx context.Context, from, to string) error {
	if err := CheckKey(from); err != nil {
		return err
	}
	if err := CheckKey(to); err != nil {
		return err
	}
	resp, err := w.do(ctx, "MOVE", from, nil, -1, map[string]string{"Destination": w.url(to), "Overwrite": "F"})
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusPreconditionFailed {
		return errors.New("已经有同名文件")
	}
	if resp.StatusCode/100 != 2 {
		return &davError{"MOVE", resp.StatusCode}
	}
	return nil
}

func remoteName(name string) error {
	if strings.TrimSpace(name) == "" || path.Base(name) != name || strings.ContainsAny(name, "\\\x00") {
		return ErrBadKey
	}
	return CheckKey(name)
}

func (g *GDrive) CreateDir(ctx context.Context, parent, name string) (DirEntry, error) {
	if err := remoteName(name); err != nil {
		return DirEntry{}, err
	}
	if parent == "" {
		parent = "root"
	}
	var out driveMeta
	err := g.callJSON(ctx, http.MethodPost, g.e.API+"/drive/v3/files?fields="+fileFields, map[string]any{"name": name, "parents": []string{parent}, "mimeType": folderMime}, &out)
	return out.entry(), err
}

func (g *GDrive) RenameFile(ctx context.Context, id, name string) (DirEntry, error) {
	if err := remoteName(name); err != nil {
		return DirEntry{}, err
	}
	if id == "" || id == "root" {
		return DirEntry{}, ErrBadKey
	}
	var out driveMeta
	err := g.callJSON(ctx, http.MethodPatch, g.e.API+"/drive/v3/files/"+url.PathEscape(id)+"?fields="+fileFields, map[string]string{"name": name}, &out)
	return out.entry(), err
}

func (g *GDrive) DeleteFile(ctx context.Context, id string) error {
	if id == "" || id == "root" {
		return ErrBadKey
	}
	return g.deleteID(ctx, id)
}

func (g *GDrive) FileInfo(ctx context.Context, id string) (DirEntry, error) {
	f, err := g.meta(ctx, id)
	return f.entry(), err
}

func (g *GDrive) OpenFileRange(ctx context.Context, id, rng string) (*http.Response, DirEntry, error) {
	f, err := g.meta(ctx, id)
	if err != nil {
		return nil, DirEntry{}, err
	}
	e := f.entry()
	if !e.Downloadable() {
		return nil, e, errors.New("Google 在线文档请在原网盘打开")
	}
	headers := map[string]string{}
	if rng != "" {
		headers["Range"] = rng
	}
	resp, err := g.call(ctx, http.MethodGet, g.e.API+"/drive/v3/files/"+url.PathEscape(id)+"?alt=media", nil, -1, headers)
	if err != nil {
		return nil, e, err
	}
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return nil, e, readDriveError(resp)
	}
	return resp, e, nil
}

// WriteFile uses the existing resumable uploader. Updating an id preserves it.
func (g *GDrive) WriteFile(ctx context.Context, id, parent, name string, r io.Reader, size int64) (DirEntry, error) {
	method, target := http.MethodPost, g.e.API+"/upload/drive/v3/files"
	meta := map[string]any{}
	if id != "" {
		f, err := g.meta(ctx, id)
		if err != nil {
			return DirEntry{}, err
		}
		if !f.entry().Downloadable() {
			return DirEntry{}, ErrBadKey
		}
		target += "/" + url.PathEscape(id)
		method = http.MethodPatch
	} else {
		if err := remoteName(name); err != nil {
			return DirEntry{}, err
		}
		if parent == "" {
			parent = "root"
		}
		meta = map[string]any{"name": name, "parents": []string{parent}}
	}
	raw, _ := json.Marshal(meta)
	headers := map[string]string{"Content-Type": "application/json; charset=utf-8", "X-Upload-Content-Type": "application/octet-stream"}
	resp, err := g.call(ctx, method, target+"?uploadType=resumable&fields=id", strings.NewReader(string(raw)), int64(len(raw)), headers)
	if err != nil {
		return DirEntry{}, err
	}
	if resp.StatusCode/100 != 2 {
		return DirEntry{}, readDriveError(resp)
	}
	session := resp.Header.Get("Location")
	drain(resp)
	if session == "" {
		return DirEntry{}, errors.New("Google Drive 没有返回上传地址")
	}
	saved, err := g.upload(ctx, session, r, size)
	if err != nil {
		return DirEntry{}, err
	}
	f, err := g.meta(ctx, saved)
	return f.entry(), err
}
