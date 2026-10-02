package files

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WebDAVConfig is what it takes to reach a WebDAV folder (B63).
type WebDAVConfig struct {
	URL      string // https://dav.jianguoyun.com/dav/
	Username string
	Password string
	Folder   string // keys live in this folder below URL, "" for URL itself
}

// ValidateWebDAV checks the fields that do not need a connection.
func ValidateWebDAV(c WebDAVConfig) error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.RawQuery != "" || u.User != nil {
		return errors.New("WebDAV 地址必须是 http 或 https 开头")
	}
	if c.Folder != "" {
		if _, err := cleanPrefix(c.Folder); err != nil {
			return errors.New("WebDAV 目录不正确")
		}
	}
	return nil
}

// WebDAV keeps files in a WebDAV folder. Each key is a path below the folder.
// A file is uploaded as name.part and moved into place once it is complete,
// so a failed upload never shows up as a file.
type WebDAV struct {
	base     *url.URL // folder URL, path ends with "/"
	root     *url.URL // URL without the folder
	folder   string
	username string
	password string
	client   *http.Client

	mu   sync.Mutex
	dirs map[string]bool // folders known to exist, relative to base
}

var _ Store = (*WebDAV)(nil)

// partSuffix marks uploads that are not finished. List skips them.
const partSuffix = ".part"

// NewWebDAV builds a client for the folder. It does not connect yet; call Check.
func NewWebDAV(c WebDAVConfig) (*WebDAV, error) {
	if err := ValidateWebDAV(c); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.URL)
	rootPath := strings.TrimSuffix(u.EscapedPath(), "/") + "/"
	root := withPath(*u, rootPath)
	folder, _ := cleanPrefix(c.Folder)
	base := rootPath
	if folder != "" {
		base += escapeKey(folder) + "/"
	}
	u = withPath(*u, base)
	dial := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dial,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 60 * time.Second, MaxIdleConnsPerHost: 4},
		// Redirects would drop the body of a PUT; WebDAV servers do not need them.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &WebDAV{base: u, root: root, folder: folder, username: c.Username, password: c.Password, client: client, dirs: map[string]bool{}}, nil
}

// withPath returns u with the escaped path p.
func withPath(u url.URL, p string) *url.URL {
	u.Path, _ = url.PathUnescape(p)
	u.RawPath = p
	return &u
}

func escapeKey(key string) string {
	segs := strings.Split(key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

// url returns the address of rel, a key or a folder ending with "/".
func (w *WebDAV) url(rel string) string { return join(w.base, rel) }

// join appends rel (a key, or a folder ending with "/") to the folder URL u.
func join(u *url.URL, rel string) string {
	p := u.RawPath
	if rel != "" {
		p += escapeKey(strings.TrimSuffix(rel, "/"))
		if strings.HasSuffix(rel, "/") {
			p += "/"
		}
	}
	return withPath(*u, p).String()
}

func (w *WebDAV) do(ctx context.Context, method, rel string, body io.Reader, size int64, header map[string]string) (*http.Response, error) {
	return w.doURL(ctx, method, w.url(rel), body, size, header)
}

func (w *WebDAV) doURL(ctx context.Context, method, target string, body io.Reader, size int64, header map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if body != nil && size >= 0 {
		req.ContentLength = size
		if size == 0 {
			req.Body = http.NoBody
		}
	}
	if w.username != "" || w.password != "" {
		req.SetBasicAuth(w.username, w.password)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	return w.client.Do(req)
}

// davError is an unexpected answer from the server.
type davError struct {
	Method string
	Status int
}

func (e *davError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized:
		return "用户名或密码不对"
	case http.StatusForbidden:
		return "没有权限，检查账号的权限和目录"
	case http.StatusNotFound, http.StatusConflict:
		return "地址或目录不对"
	case http.StatusInsufficientStorage:
		return "网盘空间不够"
	}
	return fmt.Sprintf("WebDAV %s 失败：%d %s", e.Method, e.Status, http.StatusText(e.Status))
}

func drain(resp *http.Response) {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
}

// mkdirs makes the folders of key, from the top down.
func (w *WebDAV) mkdirs(ctx context.Context, key string) error {
	dir := path.Dir(key)
	if dir == "." {
		dir = ""
	}
	var todo []string
	for d := dir; ; d = path.Dir(d) {
		if d == "." {
			d = ""
		}
		w.mu.Lock()
		known := w.dirs[d]
		w.mu.Unlock()
		if known {
			break
		}
		todo = append(todo, d)
		if d == "" {
			break
		}
	}
	for i := len(todo) - 1; i >= 0; i-- {
		if err := w.mkcol(ctx, todo[i]); err != nil {
			return err
		}
	}
	return nil
}

// mkcol makes one folder, relative to base ("" is base itself, with every
// folder above it). A folder that is already there is fine.
func (w *WebDAV) mkcol(ctx context.Context, dir string) error {
	var targets []string
	if dir != "" {
		targets = []string{w.url(dir + "/")}
	} else if w.folder != "" {
		segs := strings.Split(w.folder, "/")
		for i := range segs {
			targets = append(targets, join(w.root, strings.Join(segs[:i+1], "/")+"/"))
		}
	}
	for _, target := range targets {
		resp, err := w.doURL(ctx, "MKCOL", target, nil, -1, nil)
		if err != nil {
			return err
		}
		drain(resp)
		// 405 means the folder is there already.
		if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusMethodNotAllowed {
			return &davError{"MKCOL", resp.StatusCode}
		}
	}
	w.mu.Lock()
	w.dirs[dir] = true
	w.mu.Unlock()
	return nil
}

func (w *WebDAV) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	if err := w.mkdirs(ctx, key); err != nil {
		return err
	}
	part := key + partSuffix
	err := w.upload(ctx, part, r, size)
	if err == nil {
		err = w.move(ctx, part, key)
	}
	if err != nil {
		w.dropPart(context.WithoutCancel(ctx), part)
	}
	return err
}

// partRetry is how long after a failed upload the .part file is deleted a
// second time. The server may still be handling the broken PUT when the
// first DELETE arrives, and write the half file after it.
var partRetry = time.Second

// dropPart deletes a failed upload now and once more a moment later. A .part
// file that is left anyway is never listed or read.
func (w *WebDAV) dropPart(ctx context.Context, part string) {
	w.Delete(ctx, part) //nolint:errcheck // best effort
	go func() {
		time.Sleep(partRetry)
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		w.Delete(ctx, part) //nolint:errcheck // best effort
	}()
}

// counter counts the bytes read and fails at EOF when they are not size.
type counter struct {
	r    io.Reader
	n    int64
	size int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if err == io.EOF && c.size >= 0 && c.n != c.size {
		return n, fmt.Errorf("files: got %d bytes, want %d", c.n, c.size)
	}
	return n, err
}

func (w *WebDAV) upload(ctx context.Context, key string, r io.Reader, size int64) error {
	body := &counter{r: r, size: size}
	resp, err := w.do(ctx, http.MethodPut, key, io.NopCloser(body), size, nil)
	if err != nil {
		return err
	}
	drain(resp)
	if resp.StatusCode/100 != 2 {
		return &davError{"PUT", resp.StatusCode}
	}
	if size >= 0 && body.n != size {
		return fmt.Errorf("files: got %d bytes, want %d", body.n, size)
	}
	return nil
}

func (w *WebDAV) move(ctx context.Context, from, to string) error {
	resp, err := w.do(ctx, "MOVE", from, nil, -1, map[string]string{"Destination": w.url(to), "Overwrite": "T"})
	if err != nil {
		return err
	}
	drain(resp)
	if resp.StatusCode/100 != 2 {
		return &davError{"MOVE", resp.StatusCode}
	}
	return nil
}

func (w *WebDAV) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	return w.GetRange(ctx, key, 0, -1)
}

func (w *WebDAV) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	info, err := w.Stat(ctx, key)
	if err != nil {
		return nil, Info{}, err
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= info.Size || length == 0 {
		return io.NopCloser(strings.NewReader("")), info, nil
	}
	header := map[string]string{}
	end := info.Size
	if length > 0 {
		end = min(offset+length, info.Size)
	}
	if offset > 0 || end < info.Size {
		header["Range"] = fmt.Sprintf("bytes=%d-%d", offset, end-1)
	}
	resp, err := w.do(ctx, http.MethodGet, key, nil, -1, header)
	if err != nil {
		return nil, Info{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if offset == 0 && end == info.Size {
			return resp.Body, info, nil
		}
		// The server ignored Range: skip and cut ourselves.
		if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
			resp.Body.Close()
			return nil, Info{}, err
		}
		return readCloser{io.LimitReader(resp.Body, end-offset), resp.Body}, info, nil
	case http.StatusPartialContent:
		return resp.Body, info, nil
	case http.StatusNotFound:
		drain(resp)
		return nil, Info{}, ErrNotFound
	}
	drain(resp)
	return nil, Info{}, &davError{"GET", resp.StatusCode}
}

type readCloser struct {
	io.Reader
	io.Closer
}

// davEntry is one file or folder in a PROPFIND answer.
type davEntry struct {
	rel     string // relative to base, folders end with "/"
	dir     bool
	size    int64
	modTime time.Time
}

type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Status string `xml:"status"`
			Prop   struct {
				Length       string `xml:"getcontentlength"`
				LastModified string `xml:"getlastmodified"`
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
		} `xml:"propstat"`
	} `xml:"response"`
}

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/><d:getcontentlength/><d:getlastmodified/></d:prop></d:propfind>`

// propfind lists rel (a key, or a folder ending with "/") at the given depth.
// A missing path gives ErrNotFound.
func (w *WebDAV) propfind(ctx context.Context, rel string, depth string) ([]davEntry, error) {
	resp, err := w.do(ctx, "PROPFIND", rel, strings.NewReader(propfindBody), int64(len(propfindBody)),
		map[string]string{"Depth": depth, "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, &davError{"PROPFIND", resp.StatusCode}
	}
	var ms multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("WebDAV 返回的目录读不懂：%w", err)
	}
	basePath := w.base.Path
	var out []davEntry
	for _, r := range ms.Responses {
		u, err := url.Parse(r.Href)
		if err != nil {
			return nil, fmt.Errorf("WebDAV 返回的路径无法读取")
		}
		var e davEntry
		switch p := u.Path; {
		case p+"/" == basePath:
		case strings.HasPrefix(p, basePath):
			e.rel = strings.TrimPrefix(p, basePath)
		default:
			continue
		}
		haveProperties := false
		for _, ps := range r.Propstat {
			if ps.Status != "" && !strings.Contains(ps.Status, " 200") {
				continue
			}
			haveProperties = true
			if ps.Prop.ResourceType.Collection != nil {
				e.dir = true
			}
			if ps.Prop.Length != "" {
				e.size, _ = strconv.ParseInt(strings.TrimSpace(ps.Prop.Length), 10, 64)
			}
			if ps.Prop.LastModified != "" {
				e.modTime, _ = http.ParseTime(strings.TrimSpace(ps.Prop.LastModified))
			}
		}
		if !haveProperties {
			return nil, fmt.Errorf("WebDAV 返回的条目属性读取失败")
		}
		if e.dir && !strings.HasSuffix(e.rel, "/") && e.rel != "" {
			e.rel += "/"
		}
		out = append(out, e)
	}
	return out, nil
}

func (w *WebDAV) Stat(ctx context.Context, key string) (Info, error) {
	if err := CheckKey(key); err != nil {
		return Info{}, err
	}
	list, err := w.propfind(ctx, key, "0")
	if err != nil {
		return Info{}, err
	}
	for _, e := range list {
		if strings.TrimSuffix(e.rel, "/") == key {
			if e.dir {
				return Info{}, ErrNotFound
			}
			return Info{Key: key, Size: e.size, ModTime: e.modTime}, nil
		}
	}
	return Info{}, ErrNotFound
}

func (w *WebDAV) Delete(ctx context.Context, key string) error {
	if err := CheckKey(key); err != nil {
		return err
	}
	resp, err := w.do(ctx, http.MethodDelete, key, nil, -1, nil)
	if err != nil {
		return err
	}
	drain(resp)
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return &davError{"DELETE", resp.StatusCode}
}

func (w *WebDAV) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return failed(err)
	}
	return func(yield func(Info, error) bool) {
		dir := ""
		if prefix != "" {
			dir = prefix + "/"
		}
		w.walk(ctx, dir, true, yield)
	}
}

// walk yields the files below dir in sorted order. It returns false when the
// caller stopped.
func (w *WebDAV) walk(ctx context.Context, dir string, root bool, yield func(Info, error) bool) bool {
	list, err := w.propfind(ctx, dir, "1")
	if root && errors.Is(err, ErrNotFound) {
		return true
	}
	if err != nil {
		return yield(Info{}, err)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].rel < list[j].rel })
	for _, e := range list {
		if e.rel == dir || !strings.HasPrefix(e.rel, dir) {
			continue
		}
		if e.dir {
			if !w.walk(ctx, e.rel, false, yield) {
				return false
			}
			continue
		}
		if strings.HasSuffix(e.rel, partSuffix) || CheckKey(e.rel) != nil {
			continue
		}
		if !yield(Info{Key: e.rel, Size: e.size, ModTime: e.modTime}, nil) {
			return false
		}
	}
	return true
}

func (w *WebDAV) Copy(ctx context.Context, from, to string) error {
	if err := CheckKey(from); err != nil {
		return err
	}
	if err := CheckKey(to); err != nil {
		return err
	}
	if _, err := w.Stat(ctx, from); err != nil {
		return err
	}
	if err := w.mkdirs(ctx, to); err != nil {
		return err
	}
	resp, err := w.do(ctx, "COPY", from, nil, -1, map[string]string{"Destination": w.url(to), "Overwrite": "T"})
	if err != nil {
		return err
	}
	drain(resp)
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return &davError{"COPY", resp.StatusCode}
	}
	return nil
}

// Check makes the folder, writes, reads and deletes a small file, and returns
// a short Chinese message for the settings page that says what is wrong.
func (w *WebDAV) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return probe(ctx, w, explainNet)
}

// probe writes, reads back and deletes a small file.
func probe(ctx context.Context, s Store, explain func(error) error) error {
	const key = ".x-console-test"
	const body = "x-console"
	if err := s.Put(ctx, key, strings.NewReader(body), int64(len(body))); err != nil {
		return explain(err)
	}
	rc, _, err := s.Get(ctx, key)
	if err != nil {
		return explain(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(got) != body {
		return errors.New("写进去的内容读回来不一样")
	}
	if err := s.Delete(ctx, key); err != nil {
		return explain(err)
	}
	return nil
}

// explainNet turns a connection error into a short message.
func explainNet(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return errors.New("连接超时")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("解析不了这个地址")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("连接超时")
	}
	return err
}

// ReadDir lists one folder below the WebDAV address, for the drive page
// (B68). "" is the top.
func (w *WebDAV) ReadDir(ctx context.Context, dir string) ([]DirEntry, error) {
	rel := ""
	if dir != "" {
		if err := CheckKey(dir); err != nil {
			return nil, err
		}
		rel = dir + "/"
	}
	list, err := w.propfind(ctx, rel, "1")
	if err != nil {
		return nil, err
	}
	var out []DirEntry
	for _, e := range list {
		name := strings.TrimSuffix(strings.TrimPrefix(e.rel, rel), "/")
		if e.rel == rel || !strings.HasPrefix(e.rel, rel) || name == "" || strings.Contains(name, "/") {
			continue
		}
		out = append(out, DirEntry{Ref: strings.TrimSuffix(e.rel, "/"), Name: name, Dir: e.dir, Size: e.size, ModTime: e.modTime})
	}
	sortEntries(out)
	return out, nil
}
