package contracts

import (
	"context"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

const (
	MaintenanceStoragePrefix  = "maintenance.storage."
	MaintenanceCleanerPrefix  = "maintenance.cleaner."
	MaintenanceStorageKey     = "maintenance.storage_access"
	MaintenanceConnectionsKey = "maintenance.connections"
)

type StorageUsage struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Bytes     int64  `json:"bytes"`
	Files     int64  `json:"files"`
	Location  string `json:"location"`
	Module    string `json:"module,omitempty"`
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

type StorageReporter interface {
	Usage(context.Context) ([]StorageUsage, error)
}

type CleanupItem struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Module      string `json:"module,omitempty"`
	Hidden      bool   `json:"-"`
	RecordTable string `json:"-"`
	RecordID    int64  `json:"-"`
	Name        string `json:"name"`
	Bytes       int64  `json:"bytes"`
	Reason      string `json:"reason"`
}

type CleanupResult struct {
	Deleted int64 `json:"deleted"`
	Bytes   int64 `json:"bytes"`
	Skipped int64 `json:"skipped"`
	Failed  int64 `json:"failed"`
}

type Cleaner interface {
	Scan(context.Context) ([]CleanupItem, error)
	Clean(context.Context, []string) (CleanupResult, error)
}

type MaintenanceStorage interface {
	Stat(context.Context, string, string) (files.Info, error)
	Location() string
	WithCleanup(context.Context, func(context.Context) error) error
}

type MaintenanceConnections interface {
	BrowserConnections() int
	AgentConnections() int
}

type ActiveTemporaryFiles interface {
	TemporaryFileActive(string) bool
}

const ActiveTemporaryFilesKey = "maintenance.active_tmp"
