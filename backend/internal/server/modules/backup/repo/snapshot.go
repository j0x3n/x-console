package repo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type uploadBlock struct {
	hash  string
	plain []byte
}

func (r *Repository) CreateSnapshot(ctx context.Context, in Input) (Snapshot, error) {
	var s Snapshot
	if err := r.writable(ctx); err != nil {
		return s, err
	}
	all, err := r.ListSnapshots(ctx)
	if err != nil {
		return s, err
	}
	refs, err := references(all)
	if err != nil {
		return s, err
	}
	have := map[string]int64{}
	for info, err := range r.store.List(ctx, "blobs") {
		if err != nil {
			return s, err
		}
		hash := strings.TrimPrefix(info.Key, "blobs/")
		_, hash, ok := strings.Cut(hash, "/")
		if !ok || !validHex(hash, 64) || blobPath(hash) != info.Key || info.Size < 28 || info.Size > maxBlob {
			return s, ErrCorrupt
		}
		have[hash] = info.Size
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = r.now().UTC()
	}
	suffix, err := randomID()
	if err != nil {
		return s, err
	}
	s = Snapshot{Format: 1, ID: in.CreatedAt.UTC().Format("20060102T150405Z") + "-" + suffix, CreatedAt: in.CreatedAt.UTC(), Version: in.Version, Migration: in.Migration, Files: []File{}, Changes: []Change{}}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	queue := make(chan uploadBlock, Workers)
	stored := map[string]int64{}
	queued := map[string]bool{}
	var mu sync.Mutex
	var uploaded atomic.Int64
	var wg sync.WaitGroup
	var firstErr error
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancel()
	}
	for range Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := range queue {
				if workCtx.Err() != nil {
					continue
				}
				raw, err := r.encodeBlock(b.plain, b.hash)
				if err == nil {
					err = r.writable(workCtx)
				}
				if err == nil {
					err = r.store.Put(workCtx, blobPath(b.hash), bytes.NewReader(raw), int64(len(raw)))
				}
				if err != nil {
					fail(err)
					continue
				}
				mu.Lock()
				stored[b.hash] = int64(len(raw))
				mu.Unlock()
				uploaded.Add(int64(len(raw)))
			}
		}()
	}
	var done int64
	total := in.DatabaseSize
	if total <= 0 {
		cancel()
		close(queue)
		wg.Wait()
		return s, ErrCorrupt
	}
	for _, info := range in.Entries {
		if info.Size < 0 || total > (1<<63-1)-info.Size {
			cancel()
			close(queue)
			wg.Wait()
			return s, ErrCorrupt
		}
		total += info.Size
	}
	consume := func(reader io.Reader, size int64) (File, error) {
		f := File{Size: size, Blocks: []Block{}}
		if size < 0 {
			return f, ErrCorrupt
		}
		h := sha256.New()
		var read int64
		for {
			if err := workCtx.Err(); err != nil {
				return f, err
			}
			buf := make([]byte, ChunkSize)
			n, err := io.ReadFull(reader, buf)
			if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
				return f, err
			}
			if n > 0 {
				read += int64(n)
				if read > size {
					return f, fmt.Errorf("文件读取期间大小发生变化")
				}
				h.Write(buf[:n])
				sum := sha256.Sum256(buf[:n])
				hash := hex.EncodeToString(sum[:])
				if old, ok := refs[hash]; ok && (old.Size != int64(n) || old.StoredSize != have[hash]) {
					return f, fmt.Errorf("%w：已有块缺失或大小不一致", ErrCorrupt)
				}
				if _, ok := have[hash]; ok {
					if _, trusted := refs[hash]; !trusted {
						raw, err := readObject(workCtx, r.store, blobPath(hash), maxBlob)
						if err != nil {
							return f, err
						}
						b := Block{Hash: hash, Size: int64(n), StoredSize: have[hash]}
						if _, err := r.decodeBlock(raw, b); err != nil {
							return f, err
						}
						refs[hash] = b
					}
				}
				f.Blocks = append(f.Blocks, Block{Hash: hash, Size: int64(n), StoredSize: have[hash]})
				if _, ok := have[hash]; !ok && !queued[hash] {
					queued[hash] = true
					select {
					case queue <- uploadBlock{hash: hash, plain: buf[:n]}:
					case <-workCtx.Done():
						return f, workCtx.Err()
					}
				}
				done += int64(n)
				if in.Progress != nil {
					in.Progress(done, total)
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
		}
		if read != size {
			return f, fmt.Errorf("文件读取期间大小发生变化")
		}
		f.SHA256 = hex.EncodeToString(h.Sum(nil))
		return f, nil
	}
	s.Database, err = consume(in.Database, in.DatabaseSize)
	entries := append([]files.Info(nil), in.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	for _, info := range entries {
		if err != nil {
			break
		}
		if ValidatePath(info.Key) != nil {
			err = files.ErrBadKey
			break
		}
		var rc io.ReadCloser
		var got files.Info
		rc, got, err = in.Files.Get(workCtx, info.Key)
		if err != nil {
			break
		}
		if got.Size != info.Size || !got.ModTime.Equal(info.ModTime) {
			rc.Close()
			err = fmt.Errorf("文件在备份期间发生变化，已停止这次备份")
			break
		}
		var f File
		f, err = consume(rc, got.Size)
		rc.Close()
		if err != nil {
			break
		}
		var after files.Info
		after, err = in.Files.Stat(workCtx, info.Key)
		if err != nil {
			break
		}
		if after.Size != got.Size || !after.ModTime.Equal(got.ModTime) {
			err = fmt.Errorf("文件在备份期间发生变化，已停止这次备份")
			break
		}
		f.Path, f.ModTime = info.Key, got.ModTime
		s.Files = append(s.Files, f)
	}
	if err != nil {
		cancel()
	}
	close(queue)
	wg.Wait()
	if firstErr != nil {
		return Snapshot{}, firstErr
	}
	if err != nil {
		return Snapshot{}, err
	}
	fill := func(f *File) {
		for i := range f.Blocks {
			if f.Blocks[i].StoredSize == 0 {
				f.Blocks[i].StoredSize = stored[f.Blocks[i].Hash]
			}
		}
	}
	fill(&s.Database)
	s.SizeBytes = s.Database.Size
	for i := range s.Files {
		fill(&s.Files[i])
		s.SizeBytes += s.Files[i].Size
	}
	var previous *Snapshot
	if len(all) > 0 {
		previous = &all[0]
	}
	setChanges(&s, previous)
	s.UploadedBytes = uploaded.Load()
	if err := validateSnapshot(s); err != nil {
		return Snapshot{}, err
	}
	if err := r.writable(ctx); err != nil {
		return Snapshot{}, err
	}
	raw, err := json.Marshal(s)
	if err != nil || len(raw) > maxManifest {
		return Snapshot{}, fmt.Errorf("快照清单超过大小限制")
	}
	data, err := seal(r.snapshotKey, raw, r.aad("snapshot", s.ID, 0))
	if err != nil {
		return Snapshot{}, err
	}
	raw, err = json.Marshal(envelope{Format: 1, Data: data})
	if err != nil {
		return Snapshot{}, err
	}
	baseUploaded := s.UploadedBytes
	for range 4 {
		s.UploadedBytes = baseUploaded + int64(len(raw))
		plain, err := json.Marshal(s)
		if err != nil {
			return Snapshot{}, err
		}
		data, err := seal(r.snapshotKey, plain, r.aad("snapshot", s.ID, 0))
		if err != nil {
			return Snapshot{}, err
		}
		raw, err = json.Marshal(envelope{Format: 1, Data: data})
		if err != nil {
			return Snapshot{}, err
		}
		if s.UploadedBytes == baseUploaded+int64(len(raw)) {
			break
		}
	}
	if err := r.store.Put(ctx, snapshotPath(s.ID), bytes.NewReader(raw), int64(len(raw))); err != nil {
		verifyCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer stop()
		got, readErr := r.ReadSnapshot(verifyCtx, s.ID)
		gotRaw, marshalErr := json.Marshal(got)
		wantRaw, _ := json.Marshal(s)
		if readErr != nil || marshalErr != nil || !bytes.Equal(gotRaw, wantRaw) {
			return Snapshot{}, err
		}
	}
	return s, nil
}

func setChanges(s *Snapshot, previous *Snapshot) {
	before := map[string]File{}
	if previous != nil {
		for _, f := range previous.Files {
			before[f.Path] = f
		}
	}
	changes := []Change{}
	for _, f := range s.Files {
		old, ok := before[f.Path]
		switch {
		case !ok:
			s.Added++
			changes = append(changes, Change{Path: f.Path, Kind: "added"})
		case old.SHA256 != f.SHA256 || old.Size != f.Size:
			s.Modified++
			changes = append(changes, Change{Path: f.Path, Kind: "modified"})
		}
		delete(before, f.Path)
	}
	for key := range before {
		s.Deleted++
		changes = append(changes, Change{Path: key, Kind: "deleted"})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	s.ChangesTruncated = len(changes) > MaxChanges
	s.Changes = changes[:min(len(changes), MaxChanges)]
}
