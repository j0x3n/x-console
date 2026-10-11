package drive

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/actions"
	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

// Drive actions for remote AI (B151): finding files by path, uploading from
// a download link, from a one-time upload address or from a small base64
// body, one-time download links, copying and sharing. Hidden (vault) items
// are never visible or writable here, like the older drive actions.

const (
	maxBase64Upload = 600 << 10
	maxCopyItems    = 10000
)

// where is how an action names a folder: an id or a path, not both. A path
// that does not exist is created unless createParents is false.
type where struct {
	ParentID      *int64 `json:"parentId"`
	ParentPath    string `json:"parentPath"`
	CreateParents *bool  `json:"createParents"`
}

const whereSchema = `"parentId":{"type":"integer","description":"Folder id. Omit for the drive root."},` +
	`"parentPath":{"type":"string","description":"Folder path such as /Music/Jay. Use this or parentId, not both. Missing folders are created unless createParents is false."},` +
	`"createParents":{"type":"boolean"}`

const conflictSchema = `"onConflict":{"type":"string","enum":["fail","rename"],"description":"What to do when the name exists in the folder. fail (default) returns 409; rename adds ' (1)'. Existing files are never overwritten."}`

func (m *Module) registerMCPActions() {
	reg := func(a actions.Action) { m.d.Actions.Register(a) }
	reg(actions.Action{Name: "drive.resolve_path", Title: "按路径找云盘条目", Effect: actions.Read,
		Description: "Find a visible file or folder by its path such as /Music/song.mp3. Returns {found:false} when it does not exist.",
		Input:       actions.Schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`), Run: m.actionResolvePath})
	reg(actions.Action{Name: "drive.info", Title: "云盘条目详情", Effect: actions.Read,
		Description: "Details of a visible file or folder: name, path, size, type, modified time, number of public shares.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`), Run: m.actionInfo})
	reg(actions.Action{Name: "drive.copy", Title: "复制云盘条目", Effect: actions.Write,
		Description: "Copy a visible file or folder into another folder. Content is shared, so copying is cheap. Never overwrites: a name that exists gets ' (1)'. Returns the new item.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"},` + whereSchema + `},"required":["id"],"additionalProperties":false}`), Run: m.actionCopy})
	reg(actions.Action{Name: "drive.upload_from_url", Title: "从网址上传到云盘", Effect: actions.Write,
		Description: "Make the server download a file from an http or https address (port 80 or 443, public internet only) into the drive. " +
			"Use this when you have a download link. Redirects are followed up to 5 times. The call waits up to 20 seconds and returns the new item; " +
			"for a bigger file it returns {state:'running', taskId} and you check drive.task_status. Does not overwrite: see onConflict.",
		Input: actions.Schema(`{"type":"object","properties":{"url":{"type":"string"},"name":{"type":"string","description":"File name. Default: from the response or the address."},` + whereSchema + `,` + conflictSchema + `},"required":["url"],"additionalProperties":false}`), Run: m.actionUploadFromURL})
	reg(actions.Action{Name: "drive.task_status", Title: "云盘任务进度", Effect: actions.Read,
		Description: "Progress of a drive task such as the download started by drive.upload_from_url. When state is done, resultId is the new file.",
		Input:       actions.Schema(`{"type":"object","properties":{"taskId":{"type":"string"}},"required":["taskId"],"additionalProperties":false}`), Run: m.actionTaskStatus})
	reg(actions.Action{Name: "drive.upload_base64", Title: "上传小文件到云盘", Effect: actions.Write,
		Description: "Put a SMALL file (up to 600 KB after decoding, for example a cover image) into the drive. The content is sent base64 encoded in this call. " +
			"For songs and other big files use drive.upload_from_url, or drive.create_upload_link if you can run curl.",
		Input: actions.Schema(`{"type":"object","properties":{"name":{"type":"string"},"dataBase64":{"type":"string"},` + whereSchema + `,` + conflictSchema + `},"required":["name","dataBase64"],"additionalProperties":false}`), Run: m.actionUploadBase64})
	reg(actions.Action{Name: "drive.create_upload_link", Title: "生成一次性上传地址", Effect: actions.Write, MCPOnly: true,
		Description: "For clients that can run shell commands: returns an address that accepts ONE upload of ONE file, valid for 10 minutes. " +
			"Then run: curl -T <local file> '<uploadUrl>'. The file name and folder are fixed here. The link is spent by the first request, even if it fails. " +
			"Without a shell and without a download link, only drive.upload_base64 (600 KB) is possible.",
		Input: actions.Schema(`{"type":"object","properties":{"name":{"type":"string"},"size":{"type":"integer","description":"File size in bytes, if known."},` + whereSchema + `,` + conflictSchema + `},"required":["name"],"additionalProperties":false}`), Run: m.actionCreateUploadLink})
	reg(actions.Action{Name: "drive.create_download_link", Title: "生成一次性下载地址", Effect: actions.Read, MCPOnly: true,
		Description: "For clients that can run shell commands: returns an address that serves ONE download of a visible file, valid for 10 minutes. Run: curl -o <name> '<downloadUrl>'.",
		Input:       actions.Schema(`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`), Run: m.actionCreateDownloadLink})
	reg(actions.Action{Name: "drive.share_list", Title: "列出云盘分享", Effect: actions.Read,
		Description: "List public share links, optionally of one item. Creating a public link needs the user to confirm with their password, so it is done in the drive page, not here.",
		Input:       actions.Schema(`{"type":"object","properties":{"itemId":{"type":"integer"}},"additionalProperties":false}`), Run: m.actionShareList})
	reg(actions.Action{Name: "drive.share_revoke", Title: "撤销云盘分享", Effect: actions.Write, Destructive: true,
		Description: "Revoke a public share link; it stops working at once.",
		Input:       actions.Schema(`{"type":"object","properties":{"shareId":{"type":"integer"}},"required":["shareId"],"additionalProperties":false}`), Run: m.actionShareRevoke})
}

// ---- paths ----

func cleanSegments(p string) ([]string, error) {
	var out []string
	for _, s := range strings.Split(p, "/") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !validName(s) {
			return nil, httpx.Invalid("路径里有不能用的名字: " + s)
		}
		out = append(out, s)
	}
	return out, nil
}

func (m *Module) child(ctx context.Context, parent *int64, name string) (db.DriveItem, bool, error) {
	item, err := scanItem(m.d.DB.QueryRowContext(ctx, "SELECT "+itemColumns+" FROM drive_items WHERE parent_id IS ? AND name=? AND hidden=0 AND trashed_at IS NULL", parent, name))
	if errors.Is(err, sql.ErrNoRows) {
		return item, false, nil
	}
	return item, err == nil, err
}

// pathItem finds the item at a path. The root is nil, ok.
func (m *Module) pathItem(ctx context.Context, p string) (item db.DriveItem, root, found bool, err error) {
	segs, err := cleanSegments(p)
	if err != nil {
		return item, false, false, err
	}
	if len(segs) == 0 {
		return item, true, true, nil
	}
	var parent *int64
	for i, s := range segs {
		it, ok, err := m.child(ctx, parent, s)
		if err != nil || !ok {
			return item, false, false, err
		}
		if i < len(segs)-1 && it.IsDir == 0 {
			return item, false, false, nil
		}
		item, parent = it, &it.ID
	}
	return item, false, true, nil
}

// folderAt resolves where to put a new item. Missing folders are created when
// create is true.
func (m *Module) folderAt(ctx context.Context, w where) (*int64, error) {
	if w.ParentID != nil && w.ParentPath != "" {
		return nil, httpx.Invalid("parentId 和 parentPath 只能给一个")
	}
	if w.ParentPath == "" {
		return m.parent(ctx, w.ParentID, false)
	}
	segs, err := cleanSegments(w.ParentPath)
	if err != nil {
		return nil, err
	}
	create := w.CreateParents == nil || *w.CreateParents
	var parent *int64
	for _, s := range segs {
		it, ok, err := m.child(ctx, parent, s)
		if err != nil {
			return nil, err
		}
		if !ok {
			if !create {
				return nil, httpx.NewError(http.StatusNotFound, "folder_not_found", "文件夹不存在: "+s)
			}
			if it, err = m.insert(ctx, parent, s, true, 0, "", "", false, false); err != nil {
				return nil, err
			}
		} else if it.IsDir == 0 {
			return nil, httpx.Invalid("路径里的 " + s + " 是文件，不是文件夹")
		}
		parent = &it.ID
	}
	return parent, nil
}

func (m *Module) itemResult(ctx context.Context, item db.DriveItem) map[string]any {
	return map[string]any{"id": item.ID, "name": item.Name, "isDir": item.IsDir != 0, "size": item.Size, "mime": item.Mime,
		"path": m.pathString(ctx, item), "updatedAt": item.UpdatedAt}
}

func (m *Module) actionResolvePath(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		Path string `json:"path"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	item, root, found, err := m.pathItem(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	if root {
		return map[string]any{"found": true, "id": 0, "isDir": true, "name": "", "path": "/"}, nil
	}
	if !found {
		return map[string]any{"found": false}, nil
	}
	out := m.itemResult(ctx, item)
	out["found"] = true
	return out, nil
}

func (m *Module) actionInfo(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ID int64 `json:"id"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	item, err := m.visibleRow(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if item.TrashedAt != nil || item.Hidden != 0 {
		return nil, httpx.ErrNotFound
	}
	out := m.itemResult(ctx, item)
	var shares int
	_ = m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM drive_shares WHERE item_id=?", item.ID).Scan(&shares)
	out["shares"] = shares
	if item.IsDir != 0 {
		var children int
		_ = m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM drive_items WHERE parent_id=? AND hidden=0 AND trashed_at IS NULL", item.ID).Scan(&children)
		out["children"] = children
	}
	return out, nil
}

// ---- copy ----

func (m *Module) actionCopy(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ID int64 `json:"id"`
		where
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	src, err := m.visibleRow(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if src.TrashedAt != nil || src.Hidden != 0 {
		return nil, httpx.ErrNotFound
	}
	parent, err := m.folderAt(ctx, in.where)
	if err != nil {
		return nil, err
	}
	// A folder cannot be copied into itself.
	for p := parent; p != nil; {
		if *p == src.ID {
			return nil, httpx.Invalid("不能把文件夹复制到它自己里面")
		}
		up, err := m.row(ctx, *p)
		if err != nil {
			return nil, err
		}
		p = up.ParentID
	}
	count := 0
	item, err := m.copyTree(ctx, src, parent, &count)
	if err != nil {
		return nil, err
	}
	m.audit(ctx, "drive.copy", item.ID, nil)
	return m.itemResult(ctx, item), nil
}

func (m *Module) copyTree(ctx context.Context, src db.DriveItem, parent *int64, count *int) (db.DriveItem, error) {
	if *count++; *count > maxCopyItems {
		return db.DriveItem{}, httpx.Invalid("一次最多复制 10000 个条目")
	}
	if src.IsDir == 0 {
		release := m.lockBlob(src.Sha256) // a delete elsewhere must not drop the content meanwhile
		defer release()
		return m.insert(ctx, parent, src.Name, false, src.Size, src.Mime, src.Sha256, false, true)
	}
	dir, err := m.insert(ctx, parent, src.Name, true, 0, "", "", false, true)
	if err != nil {
		return dir, err
	}
	rows, err := m.d.DB.QueryContext(ctx, "SELECT "+itemColumns+" FROM drive_items WHERE parent_id=? AND hidden=0 AND trashed_at IS NULL ORDER BY id", src.ID)
	if err != nil {
		return dir, err
	}
	var kids []db.DriveItem
	for rows.Next() {
		k, err := scanItem(rows)
		if err != nil {
			rows.Close()
			return dir, err
		}
		kids = append(kids, k)
	}
	rows.Close()
	for _, k := range kids {
		if _, err := m.copyTree(ctx, k, &dir.ID, count); err != nil {
			return dir, err
		}
	}
	return dir, nil
}

// ---- uploads ----

func conflictOf(s string) (string, error) {
	switch s {
	case "", "fail":
		return "fail", nil
	case "rename":
		return "rename", nil
	}
	return "", httpx.Invalid("onConflict 只能是 fail 或 rename")
}

func (m *Module) actionUploadBase64(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		Name       string `json:"name"`
		DataBase64 string `json:"dataBase64"`
		OnConflict string `json:"onConflict"`
		where
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	if !validName(in.Name) {
		return nil, httpx.Invalid("文件名不正确")
	}
	policy, err := conflictOf(in.OnConflict)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(in.DataBase64))
	if err != nil {
		if data, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(in.DataBase64)); err != nil {
			return nil, httpx.Invalid("dataBase64 不是有效的 base64")
		}
	}
	if len(data) == 0 || len(data) > maxBase64Upload {
		return nil, httpx.Invalid("内容不能为空，也不能超过 600 KB。大文件请用 drive.upload_from_url 或 drive.create_upload_link")
	}
	parent, err := m.folderAt(ctx, in.where)
	if err != nil {
		return nil, err
	}
	item, err := m.storeStream(ctx, strings.NewReader(string(data)), parent, in.Name, policy)
	if err != nil {
		return nil, err
	}
	m.audit(ctx, "drive.upload_base64", item.ID, nil)
	return m.itemResult(ctx, item), nil
}

func (m *Module) actionCreateUploadLink(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		OnConflict string `json:"onConflict"`
		where
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	if !validName(in.Name) {
		return nil, httpx.Invalid("文件名不正确")
	}
	policy, err := conflictOf(in.OnConflict)
	if err != nil {
		return nil, err
	}
	parent, err := m.folderAt(ctx, in.where)
	if err != nil {
		return nil, err
	}
	if policy == "fail" {
		if _, taken, err := m.child(ctx, parent, in.Name); err != nil {
			return nil, err
		} else if taken {
			return nil, httpx.NewError(http.StatusConflict, "name_conflict", "同一文件夹已有同名文件，换个名字，或者用 onConflict: rename")
		}
	}
	token, expires, max, err := m.createUploadLink(ctx, parent, in.Name, policy, in.Size, audit.Actor(ctx))
	if err != nil {
		return nil, err
	}
	m.audit(ctx, "drive.create_upload_link", 0, nil)
	link := m.linkURL("/api/v1/drive/upload-links/" + token)
	return map[string]any{"uploadUrl": link, "method": "PUT", "expiresAt": expires, "maxSize": max,
		"curl": fmt.Sprintf("curl -T <local file> '%s'", link),
		"note": "One upload only. The link is used up by the first request. Send the raw file as the request body. If uploadUrl starts with /, put the panel address in front of it."}, nil
}

// publicBase is the address people reach the panel at, when it is configured.
func (m *Module) publicBase() string {
	return strings.TrimRight(m.d.Config.PublicURL, "/")
}

// linkURL is the full address of a link when the panel address is configured,
// otherwise just the path.
func (m *Module) linkURL(p string) string { return m.publicBase() + p }

func (m *Module) actionCreateDownloadLink(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ID int64 `json:"id"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	item, err := m.visibleRow(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if item.IsDir != 0 || item.TrashedAt != nil || item.Hidden != 0 {
		return nil, httpx.Invalid("只能下载没有隐藏、不在回收站的文件")
	}
	token, expires, err := m.createDownloadLink(ctx, item.ID, audit.Actor(ctx))
	if err != nil {
		return nil, err
	}
	link := m.linkURL("/api/v1/drive/download-links/" + token)
	return map[string]any{"downloadUrl": link, "expiresAt": expires, "name": item.Name, "size": item.Size,
		"curl": fmt.Sprintf("curl -o %q '%s'", item.Name, link),
		"note": "One download only. If downloadUrl starts with /, put the panel address in front of it."}, nil
}

// ---- download from a URL ----

type capReader struct {
	r    io.Reader
	left int64
}

var errTooBig = httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", "文件超过上限")

func (c *capReader) Read(p []byte) (int, error) {
	if c.left < 0 {
		return 0, errTooBig
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	if c.left < 0 {
		return n, errTooBig
	}
	return n, err
}

// idleReader cancels a download that stops delivering data.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
	idle  time.Duration
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	i.timer.Reset(i.idle)
	return n, err
}

func nameFromResponse(resp *http.Response, u *url.URL) string {
	if _, params, err := mime.ParseMediaType(resp.Header.Get("Content-Disposition")); err == nil {
		if n := path.Base(strings.ReplaceAll(params["filename"], "\\", "/")); validName(n) {
			return n
		}
	}
	if p, err := url.PathUnescape(path.Base(u.Path)); err == nil && validName(p) && p != "." && p != "/" {
		return p
	}
	return "download"
}

func (m *Module) actionUploadFromURL(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		URL        string `json:"url"`
		Name       string `json:"name"`
		OnConflict string `json:"onConflict"`
		where
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil {
		return nil, httpx.Invalid("网址不正确")
	}
	if err := checkURL(u); err != nil {
		return nil, httpx.Invalid(err.Error())
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !m.fetchGuard.allowAddr(ip) {
		return nil, httpx.Invalid(errBlockedTarget.Error())
	}
	if in.Name != "" && !validName(in.Name) {
		return nil, httpx.Invalid("文件名不正确")
	}
	policy, err := conflictOf(in.OnConflict)
	if err != nil {
		return nil, err
	}
	parent, err := m.folderAt(ctx, in.where)
	if err != nil {
		return nil, err
	}
	max := m.maxUpload()
	by := audit.Actor(ctx)
	host := u.Hostname()
	job := func(jobCtx context.Context, t *driveTask) error {
		jobCtx = audit.WithActor(jobCtx, by)
		return m.downloadJob(jobCtx, t, u, in.Name, parent, policy, max)
	}
	task := m.startTask(api.Download, "下载 "+host, 1, 0, job)
	deadline := time.Now().Add(m.taskWait)
	for time.Now().Before(deadline) {
		if st, ok := m.taskState(task.Id); ok && st.State != api.DriveTaskStateRunning {
			return m.taskResult(ctx, st), nil
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return map[string]any{"state": "running", "taskId": task.Id, "message": "还在下载，用 drive.task_status 查进度"}, nil
}

func (m *Module) downloadJob(ctx context.Context, t *driveTask, u *url.URL, name string, parent *int64, policy string, max int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "X-Console/1 (+https://github.com/j0x3n/x-console)")
	resp, err := m.fetchGuard.client().Do(req)
	if err != nil {
		return httpx.NewError(http.StatusBadGateway, "fetch_failed", "下载失败: "+errorText(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return httpx.NewError(http.StatusBadGateway, "fetch_failed", fmt.Sprintf("对方回了 HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > max {
		return httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("文件太大，上限是 %d MB", max>>20))
	}
	if name == "" {
		name = nameFromResponse(resp, resp.Request.URL)
	}
	t.progress(name, 0, 0)
	idle := &idleReader{r: resp.Body, idle: m.downloadIdle, timer: time.AfterFunc(m.downloadIdle, cancel)}
	defer idle.timer.Stop()
	item, err := m.storeStream(ctx, &capReader{r: idle, left: max}, parent, name, policy)
	if err != nil {
		if errors.Is(err, errTooBig) {
			return httpx.NewError(http.StatusRequestEntityTooLarge, "too_large", fmt.Sprintf("文件太大，上限是 %d MB", max>>20))
		}
		if ctx.Err() != nil {
			return httpx.NewError(http.StatusGatewayTimeout, "fetch_stalled", "对方很久没有发数据，已停止")
		}
		return err
	}
	m.changeTask(t, true, func(d *api.DriveTask) {
		id := item.ID
		d.ResultId = &id
		d.DoneItems, d.DoneBytes = 1, item.Size
	})
	m.audit(ctx, "drive.upload_from_url", item.ID, nil)
	return nil
}

func errorText(err error) string {
	var apiErr *httpx.Error
	if errors.As(err, &apiErr) {
		return apiErr.Message
	}
	if errors.Is(err, errBlockedTarget) {
		return errBlockedTarget.Error()
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

func (m *Module) taskState(id string) (api.DriveTask, bool) {
	m.tasksMu.Lock()
	defer m.tasksMu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return api.DriveTask{}, false
	}
	return t.dto, true
}

func (m *Module) taskResult(ctx context.Context, st api.DriveTask) map[string]any {
	out := map[string]any{"taskId": st.Id, "state": st.State, "doneBytes": st.DoneBytes}
	if st.Error != nil {
		out["error"] = *st.Error
	}
	if st.ErrorCode != nil {
		out["errorCode"] = *st.ErrorCode
	}
	if st.ResultId != nil {
		out["resultId"] = *st.ResultId
		if item, err := m.row(ctx, *st.ResultId); err == nil {
			for k, v := range m.itemResult(ctx, item) {
				out[k] = v
			}
		}
	}
	return out
}

func (m *Module) actionTaskStatus(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		TaskID string `json:"taskId"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	st, ok := m.taskState(in.TaskID)
	if !ok {
		return nil, httpx.ErrNotFound
	}
	return m.taskResult(ctx, st), nil
}

// ---- sharing ----

func (m *Module) patchShare(v any) any {
	list, ok := v.(map[string]any)
	if !ok {
		return v
	}
	fix := func(s map[string]any) {
		token, _ := s["token"].(string)
		delete(s, "code") // access codes stay out of tool results
		delete(s, "url")  // built from a fake request here; the path is what is certain
		s["path"] = "/s/" + token
		if base := m.publicBase(); base != "" {
			s["url"] = base + "/s/" + token
		}
	}
	if items, ok := list["items"].([]any); ok {
		for _, it := range items {
			if s, ok := it.(map[string]any); ok {
				fix(s)
			}
		}
		return list
	}
	fix(list)
	return list
}

func (m *Module) actionShareList(ctx context.Context, raw json.RawMessage) (any, error) {
	ctx = auth.WithoutVault(ctx)
	var in struct {
		ItemID *int64 `json:"itemId"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	out, err := m.actionRequest(ctx, http.MethodGet, "/drive/shares", map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		m.ListDriveShares(w, r, api.ListDriveSharesParams{ItemId: in.ItemID})
	})
	if err != nil {
		return nil, err
	}
	return m.patchShare(out), nil
}

func (m *Module) actionShareRevoke(ctx context.Context, raw json.RawMessage) (any, error) {
	var in struct {
		ShareID int64 `json:"shareId"`
	}
	if err := actionInput(raw, &in); err != nil {
		return nil, err
	}
	return m.actionRequest(ctx, http.MethodDelete, "/drive/shares", map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		m.DeleteDriveShare(w, r, in.ShareID)
	})
}
