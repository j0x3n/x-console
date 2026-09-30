package drive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

const (
	maxExtractBytes = 10 << 30
	maxExtractItems = 100_000
)

var (
	errUnsafeArchive = httpx.NewError(400, "archive_unsafe_path", "压缩包包含不安全的路径")
	errLargeArchive  = httpx.NewError(400, "archive_too_large", "压缩包超过大小或条目数量限制")
	errBadArchive    = httpx.NewError(400, "archive_unsupported", "压缩包格式不支持或内容已损坏")
)

type extractEntry struct {
	name string
	size int64
	dir  bool
	skip bool
	zip  *zip.File
}

type storeReaderAt struct {
	ctx   context.Context
	store files.Store
	key   string
	size  int64
}

func (r storeReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	if offset < 0 || offset >= r.size {
		return 0, io.EOF
	}
	stream, _, err := r.store.GetRange(r.ctx, r.key, offset, int64(len(p)))
	if err != nil {
		return 0, err
	}
	defer stream.Close()
	return io.ReadFull(stream, p)
}

func extractKind(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	}
	return ""
}

func extractFolderName(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range []string{".tar.gz", ".tgz", ".tar", ".zip"} {
		if strings.HasSuffix(lower, suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}

func (m *Module) ExtractDriveItem(w http.ResponseWriter, r *http.Request, itemID api.ItemId) {
	ctx := r.Context()
	var body api.ExtractRequest
	if r.ContentLength != 0 {
		if fail(w, r, httpx.Decode(r, &body)) {
			return
		}
	}
	policy := api.Rename
	if body.Conflict != nil {
		policy = *body.Conflict
	}
	if !policy.Valid() {
		httpx.Fail(w, r, httpx.Invalid("同名处理方式不正确"))
		return
	}
	item, err := m.visibleRow(ctx, itemID)
	if fail(w, r, err) {
		return
	}
	if item.IsDir != 0 || item.TrashedAt != nil || item.Sha256 == "" {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	kind := extractKind(item.Name)
	if kind == "" {
		httpx.Fail(w, r, errBadArchive)
		return
	}
	key := blobKey(item.Sha256)
	if err := m.checkArchiveMagic(ctx, key, kind); fail(w, r, err) {
		return
	}
	var target *int64
	if body.TargetId != nil {
		if *body.TargetId < 0 {
			httpx.Fail(w, r, httpx.NewError(400, "invalid_target", "目标文件夹不可用"))
			return
		}
		target, err = m.archiveParent(ctx, body.TargetId, item.Hidden != 0)
		if fail(w, r, err) {
			return
		}
	}
	title := "解压 " + item.Name
	job := func(jobCtx context.Context, t *driveTask) error {
		err := m.extractArchive(jobCtx, t, item, kind, target, body.TargetId != nil, policy)
		m.d.Audit.Record(context.WithoutCancel(jobCtx), "drive.extract", strconv.FormatInt(item.ID, 10), map[string]any{"targetId": body.TargetId}, err)
		return err
	}
	start := m.startTask
	if item.Hidden != 0 {
		start, title = m.startHiddenTask, "解压隐藏空间里的压缩包"
	}
	var task api.DriveTask
	if body.TargetId != nil {
		task = start(api.Extract, title, 0, 0, job, *body.TargetId)
	} else {
		task = start(api.Extract, title, 0, 0, job)
	}
	httpx.JSON(w, http.StatusAccepted, task)
}

func (m *Module) checkArchiveMagic(ctx context.Context, key, kind string) error {
	stream, _, err := m.store.GetRange(ctx, key, 0, 512)
	if err != nil {
		return err
	}
	defer stream.Close()
	magic, err := io.ReadAll(io.LimitReader(stream, 512))
	if err != nil {
		return err
	}
	switch kind {
	case "zip":
		if len(magic) >= 4 && string(magic[:4]) == "PK\x03\x04" {
			return nil
		}
	case "tar.gz":
		if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
			return nil
		}
	case "tar":
		if len(magic) == 512 {
			checksum, err := strconv.ParseInt(strings.Trim(string(magic[148:156]), " \x00"), 8, 64)
			var sum int64
			for i, value := range magic {
				if i >= 148 && i < 156 {
					value = ' '
				}
				sum += int64(value)
			}
			if err == nil && checksum == sum {
				return nil
			}
		}
	}
	return errBadArchive
}

func safeArchiveName(raw string) (string, error) {
	name := strings.ReplaceAll(raw, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsAny(name, ":\x00") {
		return "", errUnsafeArchive
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", errUnsafeArchive
		}
	}
	name = path.Clean(name)
	if name == "." {
		return "", nil // a root directory entry carries no content
	}
	if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		return "", errUnsafeArchive
	}
	parts := strings.Split(name, "/")
	if len(parts) > 64 {
		return "", errUnsafeArchive
	}
	for _, part := range parts {
		if !validName(part) {
			return "", errUnsafeArchive
		}
	}
	return name, nil
}

func addExtractEntry(entries *[]extractEntry, total *int64, entry extractEntry) error {
	if len(*entries) >= maxExtractItems {
		return errLargeArchive
	}
	if entry.size < 0 || entry.size > maxExtractBytes-*total {
		return errLargeArchive
	}
	*total += entry.size
	*entries = append(*entries, entry)
	return nil
}

func (m *Module) planExtraction(ctx context.Context, key, kind string) ([]extractEntry, int64, error) {
	entries := []extractEntry{}
	var total int64
	if kind == "zip" {
		info, err := m.store.Stat(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		zr, err := zip.NewReader(storeReaderAt{ctx: ctx, store: m.store, key: key, size: info.Size}, info.Size)
		if err != nil {
			return nil, 0, errBadArchive
		}
		for _, file := range zr.File {
			name := file.Name
			if file.Flags&0x800 == 0 {
				name, err = simplifiedchinese.GBK.NewDecoder().String(name)
				if err != nil {
					return nil, 0, errBadArchive
				}
			}
			name, err = safeArchiveName(name)
			if err != nil {
				return nil, 0, err
			}
			if name == "" {
				continue
			}
			mode := file.Mode()
			isDir := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
			skip := !isDir && mode.Type() != 0
			size := int64(0)
			if !isDir && !skip {
				if file.UncompressedSize64 > maxExtractBytes {
					return nil, 0, errLargeArchive
				}
				size = int64(file.UncompressedSize64)
			}
			if err := addExtractEntry(&entries, &total, extractEntry{name: name, size: size, dir: isDir, skip: skip, zip: file}); err != nil {
				return nil, 0, err
			}
		}
		return entries, total, nil
	}
	tr, closeStream, err := m.openTar(ctx, key, kind)
	if err != nil {
		return nil, 0, errBadArchive
	}
	defer closeStream()
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, errBadArchive
		}
		name, err := safeArchiveName(header.Name)
		if err != nil {
			return nil, 0, err
		}
		if name == "" {
			continue
		}
		isDir := header.Typeflag == tar.TypeDir
		skip := !isDir && header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA
		size := int64(0)
		if !isDir && !skip {
			size = header.Size
		}
		if err := addExtractEntry(&entries, &total, extractEntry{name: name, size: size, dir: isDir, skip: skip}); err != nil {
			return nil, 0, err
		}
	}
	return entries, total, nil
}

func (m *Module) openTar(ctx context.Context, key, kind string) (*tar.Reader, func(), error) {
	src, _, err := m.store.Get(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if kind == "tar" {
		return tar.NewReader(src), func() { src.Close() }, nil
	}
	gz, err := gzip.NewReader(src)
	if err != nil {
		src.Close()
		return nil, nil, err
	}
	return tar.NewReader(gz), func() { gz.Close(); src.Close() }, nil
}

type extractor struct {
	m       *Module
	t       *driveTask
	ctx     context.Context
	root    *int64
	hidden  bool
	policy  api.ConflictPolicy
	dirs    map[string]int64
	created []int64
	done    int
	bytes   int64
	skipped int
}

func (m *Module) extractArchive(ctx context.Context, t *driveTask, item db.DriveItem, kind string, target *int64, direct bool, policy api.ConflictPolicy) (retErr error) {
	key := blobKey(item.Sha256)
	entries, totalBytes, err := m.planExtraction(ctx, key, kind)
	if err != nil {
		return err
	}
	m.changeTask(t, true, func(dto *api.DriveTask) {
		dto.TotalItems = len(entries)
		dto.TotalBytes = totalBytes
	})
	x := &extractor{m: m, t: t, ctx: ctx, root: target, hidden: item.Hidden != 0, policy: policy, dirs: make(map[string]int64)}
	defer func() {
		if retErr != nil {
			x.cleanup()
		}
	}()
	if !direct {
		folderName := extractFolderName(item.Name)
		if !validName(folderName) {
			return errBadArchive
		}
		id, err := x.insertFolder(item.ParentID, folderName, true)
		if err != nil {
			return err
		}
		x.root = &id
		m.changeTask(t, true, func(dto *api.DriveTask) {
			result := id
			dto.ResultId = &result
			dto.TargetId = &result
		})
	}
	if kind == "zip" {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.skip || entry.dir {
				if err := x.process(entry, nil); err != nil {
					return err
				}
				continue
			}
			reader, err := entry.zip.Open()
			if err != nil {
				return errBadArchive
			}
			processErr := x.process(entry, reader)
			closeErr := reader.Close()
			if processErr != nil {
				return processErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	} else {
		tr, closeStream, err := m.openTar(ctx, key, kind)
		if err != nil {
			return errBadArchive
		}
		defer closeStream()
		index := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			header, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return errBadArchive
			}
			name, err := safeArchiveName(header.Name)
			if err != nil {
				return err
			}
			if name == "" {
				continue
			}
			if index >= len(entries) || name != entries[index].name {
				return errBadArchive
			}
			entry := entries[index]
			index++
			if err := x.process(entry, tr); err != nil {
				return err
			}
		}
		if index != len(entries) {
			return errBadArchive
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.triggerSync()
	return nil
}

func (x *extractor) process(entry extractEntry, reader io.Reader) error {
	if entry.skip {
		x.skipped++
	} else if entry.dir {
		if _, err := x.ensureFolder(entry.name); err != nil {
			return err
		}
	} else {
		parentPath := path.Dir(entry.name)
		if parentPath == "." {
			parentPath = ""
		}
		parent, err := x.ensureFolder(parentPath)
		if err != nil {
			return err
		}
		if err := x.insertFile(parent, path.Base(entry.name), entry.size, reader); err != nil {
			return err
		}
	}
	x.done++
	x.t.progress(entry.name, x.done, x.bytes)
	x.m.changeTask(x.t, false, func(dto *api.DriveTask) { skipped := x.skipped; dto.Skipped = &skipped })
	return nil
}

func (x *extractor) ensureFolder(name string) (*int64, error) {
	if name == "" {
		return x.root, nil
	}
	if id, ok := x.dirs[name]; ok {
		return &id, nil
	}
	parentPath := path.Dir(name)
	if parentPath == "." {
		parentPath = ""
	}
	parent, err := x.ensureFolder(parentPath)
	if err != nil {
		return nil, err
	}
	id, err := x.insertFolder(parent, path.Base(name), false)
	if err != nil {
		return nil, err
	}
	x.dirs[name] = id
	return &id, nil
}

func (x *extractor) insertFolder(parent *int64, name string, alwaysNew bool) (int64, error) {
	var id int64
	created := false
	err := x.m.write(x.ctx, func(tx *sql.Tx) error {
		var existingID, isDir int64
		err := tx.QueryRowContext(x.ctx, "SELECT id,is_dir FROM drive_items WHERE parent_id IS ? AND hidden=? AND name=? AND trashed_at IS NULL LIMIT 1", parent, intBool(x.hidden), name).Scan(&existingID, &isDir)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && isDir != 0 && !alwaysNew {
			id = existingID
			return nil
		}
		if err == nil {
			name, err = x.m.freeName(x.ctx, tx, parent, x.hidden, name, 0)
			if err != nil {
				return err
			}
		}
		now := time.Now().UTC()
		row, err := db.New(tx).InsertItem(x.ctx, db.InsertItemParams{ParentID: parent, Name: name, IsDir: 1, Hidden: intBool(x.hidden), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		id = row.ID
		created = true
		return nil
	})
	if err != nil {
		return 0, err
	}
	if created {
		x.created = append(x.created, id)
	}
	return id, nil
}

func (x *extractor) insertFile(parent *int64, name string, declared int64, reader io.Reader) error {
	tmp, err := os.CreateTemp(x.m.tmpDir, "extract-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	hash := sha256.New()
	limit := int64(maxExtractBytes) - x.bytes + 1
	if declared+1 < limit {
		limit = declared + 1
	}
	n, copyErr := io.CopyBuffer(io.MultiWriter(tmp, hash), &extractReader{ctx: x.ctx, r: io.LimitReader(reader, limit)}, make([]byte, 32*1024))
	if copyErr == nil && n > int64(maxExtractBytes)-x.bytes {
		copyErr = errLargeArchive
	}
	if copyErr == nil && n != declared {
		copyErr = errBadArchive
	}
	if copyErr == nil {
		var extra [1]byte
		more, readErr := io.ReadFull(reader, extra[:])
		if more != 0 || (readErr != nil && readErr != io.EOF) {
			copyErr = errBadArchive
		}
	}
	mime := "application/octet-stream"
	if copyErr == nil {
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			copyErr = err
		} else {
			var sniff [512]byte
			read, err := tmp.Read(sniff[:])
			if err != nil && err != io.EOF {
				copyErr = err
			} else {
				mime = http.DetectContentType(sniff[:read])
			}
		}
	}
	if closeErr := tmp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	release, err := x.m.putBlobFile(x.ctx, digest, tmp.Name(), n)
	if err != nil {
		return err
	}
	defer release()
	committed := false
	defer func() {
		if !committed {
			x.m.dropBlobLocked(context.WithoutCancel(x.ctx), digest)
		}
	}()
	var id int64
	skipped := false
	err = x.m.write(x.ctx, func(tx *sql.Tx) error {
		item := db.DriveItem{Name: name, Hidden: intBool(x.hidden)}
		finalName, skip, err := x.m.transferName(x.ctx, tx, item, parent, x.policy, true)
		if err != nil {
			return err
		}
		if skip {
			skipped = true
			return nil
		}
		now := time.Now().UTC()
		row, err := db.New(tx).InsertItem(x.ctx, db.InsertItemParams{ParentID: parent, Name: finalName, Size: n, Mime: mime, Sha256: digest, Hidden: intBool(x.hidden), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return err
		}
		id = row.ID
		return nil
	})
	if err != nil {
		return err
	}
	if skipped {
		x.skipped++
		return nil
	}
	committed = true
	x.created = append(x.created, id)
	x.bytes += n
	return nil
}

type extractReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *extractReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (x *extractor) cleanup() {
	for i := len(x.created) - 1; i >= 0; i-- {
		_ = x.m.permanentDelete(context.WithoutCancel(x.ctx), x.created[i])
	}
}
