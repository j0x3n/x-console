package repo

import (
	"context"
	"errors"
	"io"
	"iter"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

const ChunkSize = 4 << 20
const Workers = 4
const MaxChanges = 200
const maxManifest = 32 << 20
const maxBlob = ChunkSize + (64 << 10)

var ErrMissing = errors.New("增量备份仓库还没有建立")
var ErrKey = errors.New("主密钥不匹配，或备份仓库配置已损坏")
var ErrCorrupt = errors.New("备份仓库已损坏")
var ErrLocked = errors.New("备份仓库已有任务，或属于其他写入实例")
var ErrLostLock = errors.New("备份仓库锁已失效，已停止修改")

type Store interface {
	Put(context.Context, string, io.Reader, int64) error
	Get(context.Context, string) (io.ReadCloser, files.Info, error)
	Stat(context.Context, string) (files.Info, error)
	Delete(context.Context, string) error
	List(context.Context, string) iter.Seq2[files.Info, error]
}

type Block struct {
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	StoredSize int64  `json:"storedSize"`
}

type File struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
	SHA256  string    `json:"sha256"`
	Blocks  []Block   `json:"blocks"`
}

type Change struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Snapshot struct {
	Format           int       `json:"format"`
	ID               string    `json:"id"`
	CreatedAt        time.Time `json:"createdAt"`
	Version          string    `json:"version"`
	Migration        string    `json:"migration"`
	Database         File      `json:"database"`
	Files            []File    `json:"files"`
	SizeBytes        int64     `json:"sizeBytes"`
	UploadedBytes    int64     `json:"uploadedBytes"`
	Added            int       `json:"added"`
	Modified         int       `json:"modified"`
	Deleted          int       `json:"deleted"`
	Changes          []Change  `json:"changes"`
	ChangesTruncated bool      `json:"changesTruncated"`
}

type Input struct {
	Database     io.Reader
	DatabaseSize int64
	Files        files.Store
	Entries      []files.Info
	CreatedAt    time.Time
	Version      string
	Migration    string
	Progress     func(int64, int64)
}

type Retention struct {
	Last    int `json:"last"`
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
}

type Stats struct {
	Snapshots      int        `json:"snapshots"`
	SizeBytes      int64      `json:"sizeBytes"`
	LogicalBytes   int64      `json:"logicalBytes"`
	UniqueBytes    int64      `json:"uniqueBytes"`
	LastSnapshotAt *time.Time `json:"lastSnapshotAt,omitempty"`
}

type CheckResult struct {
	Snapshots int       `json:"snapshots"`
	Blocks    int       `json:"blocks"`
	CheckedAt time.Time `json:"checkedAt"`
}

type PruneResult struct {
	Snapshots int   `json:"snapshots"`
	Blocks    int   `json:"blocks"`
	Bytes     int64 `json:"bytes"`
}
