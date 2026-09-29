package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

// Names inside a backup package.
const (
	manifestName  = "manifest.json"
	dbName        = "x-console.db"
	filesPrefix   = "files/"
	formatVersion = 1
)

// manifest is the first entry of every package.
type manifest struct {
	Format    int       `json:"format"`
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	Files     int64     `json:"files"`
	Bytes     int64     `json:"bytes"`
	DBSha256  string    `json:"dbSha256"`
	Migration string    `json:"migration"`
}

// sidecar is written next to a package as <name>.json so the list can show
// the kind without opening the archive.
type sidecar struct {
	manifest
	Kind string `json:"kind"`
}

// errNotBackup means the bytes are not a package this program made.
var errNotBackup = errors.New("不是 X Console 的备份文件")

// latestMigration is the newest migration applied to the database. Packages
// carry it as a string so it survives JSON without rounding.
func latestMigration(ctx context.Context, db *sql.DB) (string, error) {
	var v int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version WHERE is_applied = 1`).Scan(&v)
	return fmt.Sprint(v), err
}

// newerMigration reports whether a package comes from a newer version than the database.
func newerMigration(pkg, current string) bool {
	var p, c int64
	fmt.Sscan(pkg, &p)
	fmt.Sscan(current, &c)
	return p > c
}

// snapshotDB writes a consistent copy of the database to path and returns its SHA-256.
func snapshotDB(ctx context.Context, db *sql.DB, path string) (sum string, size int64, err error) {
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return "", 0, fmt.Errorf("备份数据库失败：%w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), size, err
}

// packInput is everything writeArchive puts into a package.
type packInput struct {
	Manifest manifest
	DBPath   string
	Store    files.Store
	Files    []files.Info
	// Progress is called after every file with the running totals.
	Progress func(doneFiles, doneBytes int64)
}

// writeArchive streams a package to w without holding files in memory.
func writeArchive(ctx context.Context, w io.Writer, in packInput) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	raw, err := json.MarshalIndent(in.Manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := addBytes(tw, manifestName, raw, in.Manifest.CreatedAt); err != nil {
		return err
	}
	dbFile, err := os.Open(in.DBPath)
	if err != nil {
		return err
	}
	st, err := dbFile.Stat()
	if err == nil {
		err = addStream(tw, dbName, dbFile, st.Size(), in.Manifest.CreatedAt)
	}
	dbFile.Close()
	if err != nil {
		return err
	}
	var doneBytes int64
	for i, info := range in.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := addStoreFile(ctx, tw, in.Store, info)
		if err != nil {
			return fmt.Errorf("打包 %s 失败：%w", info.Key, err)
		}
		doneBytes += n
		if in.Progress != nil {
			in.Progress(int64(i+1), doneBytes)
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func addBytes(tw *tar.Writer, name string, data []byte, at time.Time) error {
	return addStream(tw, name, bytes.NewReader(data), int64(len(data)), at)
}

func addStream(tw *tar.Writer, name string, r io.Reader, size int64, at time.Time) error {
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size, ModTime: at, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	n, err := io.Copy(tw, r)
	if err == nil && n != size {
		err = fmt.Errorf("%s 的大小在打包时变了", name)
	}
	return err
}

// addStoreFile copies one site file into the package. A file that was deleted
// since the list was made is skipped. It returns the bytes written.
func addStoreFile(ctx context.Context, tw *tar.Writer, s files.Store, info files.Info) (int64, error) {
	rc, got, err := s.Get(ctx, info.Key)
	if errors.Is(err, files.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	if err := addStream(tw, filesPrefix+info.Key, rc, got.Size, got.ModTime); err != nil {
		return 0, err
	}
	return got.Size, nil
}

// archiveReader walks the entries of a package.
type archiveReader struct {
	gz *gzip.Reader
	tr *tar.Reader
}

func openArchive(r io.Reader) (*archiveReader, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, errNotBackup
	}
	return &archiveReader{gz: gz, tr: tar.NewReader(gz)}, nil
}

func (a *archiveReader) Close() error { return a.gz.Close() }

// readManifest reads and checks the first entry.
func (a *archiveReader) readManifest() (manifest, error) {
	var m manifest
	hdr, err := a.tr.Next()
	if err != nil || hdr.Name != manifestName || hdr.Size > 1<<20 {
		return m, errNotBackup
	}
	if err := json.NewDecoder(io.LimitReader(a.tr, 1<<20)).Decode(&m); err != nil {
		return m, errNotBackup
	}
	if m.Format != formatVersion || m.DBSha256 == "" {
		return m, errNotBackup
	}
	return m, nil
}

// next returns the next entry, io.EOF at the end.
func (a *archiveReader) next() (*tar.Header, error) { return a.tr.Next() }
