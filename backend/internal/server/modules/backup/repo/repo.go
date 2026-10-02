package repo

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

type Repository struct {
	store       Store
	cfg         config
	blobKey     cipher.AEAD
	snapshotKey cipher.AEAD
	lockKey     cipher.AEAD
	now         func() time.Time
	guard       func(context.Context) error
}

func Open(ctx context.Context, s Store, master []byte) (*Repository, error) {
	if len(master) != 32 {
		return nil, ErrKey
	}
	raw, err := readObject(ctx, s, "config.json", 64<<10)
	if errors.Is(err, files.ErrNotFound) {
		return nil, ErrMissing
	}
	if err != nil {
		return nil, err
	}
	var c config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, ErrCorrupt
	}
	if c.Format != 1 || c.ChunkSize != ChunkSize || c.KDF != "HKDF-SHA256" || c.Cipher != "AES-256-GCM" || c.Compression != "gzip" || len(c.Salt) != 32 || !validHex(c.ID, 32) || !validHex(c.Owner, 32) {
		return nil, fmt.Errorf("%w：配置版本或参数不支持", ErrCorrupt)
	}
	key, err := derive(master, c.Salt, "config")
	if err != nil {
		return nil, err
	}
	if !hmac.Equal(c.Auth, configAuth(c, key)) {
		return nil, ErrKey
	}
	r := &Repository{store: s, cfg: c, now: time.Now}
	for purpose, dst := range map[string]*cipher.AEAD{"blob": &r.blobKey, "snapshot": &r.snapshotKey, "lock": &r.lockKey} {
		key, err := derive(master, c.Salt, purpose)
		if err != nil {
			return nil, err
		}
		*dst, err = newAEAD(key)
		if err != nil {
			return nil, err
		}
	}
	return r, nil
}

func Init(ctx context.Context, s Store, master []byte, owner string) (*Repository, error) {
	if !validHex(owner, 32) || len(master) != 32 {
		return nil, ErrKey
	}
	if _, err := s.Stat(ctx, "config.json"); err == nil {
		return Open(ctx, s, master)
	} else if !errors.Is(err, files.ErrNotFound) {
		return nil, err
	}
	for _, err := range s.List(ctx, "") {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w：目录不是空的，不能创建新仓库", ErrCorrupt)
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	c := config{Format: 1, ID: id, Owner: owner, Salt: make([]byte, 32), ChunkSize: ChunkSize, KDF: "HKDF-SHA256", Cipher: "AES-256-GCM", Compression: "gzip"}
	if _, err := rand.Read(c.Salt); err != nil {
		return nil, err
	}
	key, err := derive(master, c.Salt, "config")
	if err != nil {
		return nil, err
	}
	c.Auth = configAuth(c, key)
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if err := s.Put(ctx, "config.json", bytes.NewReader(raw), int64(len(raw))); err != nil {
		return nil, err
	}
	return Open(ctx, s, master)
}

func readObject(ctx context.Context, s Store, key string, limit int64) ([]byte, error) {
	rc, info, err := s.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	if info.Size < 0 || info.Size > limit {
		return nil, fmt.Errorf("%w：对象超过大小限制", ErrCorrupt)
	}
	raw, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) != info.Size || int64(len(raw)) > limit {
		return nil, fmt.Errorf("%w：对象大小不一致", ErrCorrupt)
	}
	return raw, nil
}

func validHex(v string, length int) bool {
	if len(v) != length || strings.ToLower(v) != v {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

func ValidID(id string) bool {
	if len(id) < 33 || len(id) > 100 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c == '-' || c == 'T' || c == 'Z') {
			return false
		}
	}
	return true
}

func blobPath(hash string) string   { return "blobs/" + hash[:2] + "/" + hash }
func snapshotPath(id string) string { return "snapshots/" + id + ".json" }

func ValidatePath(key string) error {
	if err := files.CheckKey(key); err != nil {
		return err
	}
	if files.Module(key) == "backups" || !strings.Contains(key, "/") {
		return files.ErrBadKey
	}
	for _, seg := range strings.Split(key, "/") {
		if strings.ContainsAny(seg, ":<>\"|?*") || strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return files.ErrBadKey
		}
		base := strings.ToUpper(strings.SplitN(seg, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return files.ErrBadKey
		}
	}
	return nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func validateFile(f File) error {
	if f.Size < 0 || !validHex(f.SHA256, 64) {
		return ErrCorrupt
	}
	if int64(len(f.Blocks)) != f.Size/ChunkSize+int64(boolInt(f.Size%ChunkSize != 0)) {
		return ErrCorrupt
	}
	var sum int64
	for i, b := range f.Blocks {
		if !validHex(b.Hash, 64) || b.Size <= 0 || b.Size > ChunkSize || b.StoredSize < 28 || b.StoredSize > maxBlob {
			return ErrCorrupt
		}
		if i < len(f.Blocks)-1 && b.Size != ChunkSize {
			return ErrCorrupt
		}
		sum += b.Size
	}
	if sum != f.Size {
		return ErrCorrupt
	}
	return nil
}

func validateSnapshot(s Snapshot) error {
	if s.Format != 1 || !ValidID(s.ID) || s.CreatedAt.IsZero() || s.SizeBytes < 0 || s.UploadedBytes < 0 || s.Added < 0 || s.Modified < 0 || s.Deleted < 0 || len(s.Changes) > MaxChanges {
		return ErrCorrupt
	}
	if migration, err := strconv.ParseInt(s.Migration, 10, 64); err != nil || migration < 0 {
		return ErrCorrupt
	}
	if err := validateFile(s.Database); err != nil || s.Database.Size == 0 {
		return ErrCorrupt
	}
	seen := map[string]bool{}
	var total = s.Database.Size
	for _, f := range s.Files {
		if ValidatePath(f.Path) != nil || validateFile(f) != nil {
			return ErrCorrupt
		}
		key := strings.ToLower(f.Path)
		if seen[key] {
			return ErrCorrupt
		}
		seen[key] = true
		if total > (1<<63-1)-f.Size {
			return ErrCorrupt
		}
		total += f.Size
	}
	for key := range seen {
		for p, _, ok := strings.Cut(key, "/"); ok; {
			if seen[p] {
				return ErrCorrupt
			}
			rest := strings.TrimPrefix(key, p+"/")
			next, _, more := strings.Cut(rest, "/")
			if !more {
				break
			}
			p += "/" + next
		}
	}
	if total != s.SizeBytes {
		return ErrCorrupt
	}
	for _, c := range s.Changes {
		if ValidatePath(c.Path) != nil || c.Kind != "added" && c.Kind != "modified" && c.Kind != "deleted" {
			return ErrCorrupt
		}
	}
	return nil
}

func (r *Repository) ReadSnapshot(ctx context.Context, id string) (Snapshot, error) {
	var s Snapshot
	if !ValidID(id) {
		return s, files.ErrBadKey
	}
	raw, err := readObject(ctx, r.store, snapshotPath(id), maxManifest*2)
	if err != nil {
		return s, err
	}
	var env envelope
	if json.Unmarshal(raw, &env) != nil || env.Format != 1 {
		return s, ErrCorrupt
	}
	plain, err := unseal(r.snapshotKey, env.Data, r.aad("snapshot", id, 0))
	if err != nil {
		return s, err
	}
	if len(plain) > maxManifest || json.Unmarshal(plain, &s) != nil || s.ID != id {
		return s, ErrCorrupt
	}
	return s, validateSnapshot(s)
}

func (r *Repository) ListSnapshots(ctx context.Context) ([]Snapshot, error) {
	out := []Snapshot{}
	for info, err := range r.store.List(ctx, "snapshots") {
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(strings.TrimPrefix(info.Key, "snapshots/"), ".json")
		if snapshotPath(id) != info.Key || !ValidID(id) {
			return nil, fmt.Errorf("%w：快照名称不正确", ErrCorrupt)
		}
		s, err := r.ReadSnapshot(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}
