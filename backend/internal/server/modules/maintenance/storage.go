package maintenance

import (
	"context"
	"errors"
	"regexp"
	"strconv"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

type StoreReporter struct {
	Store                                files.Store
	Registry                             *module.Registry
	Key, Label, Module, Location, Prefix string
}

func (r StoreReporter) Usage(ctx context.Context) ([]contracts.StorageUsage, error) {
	u := contracts.StorageUsage{Key: r.Key, Label: r.Label, Module: r.Module, Location: r.Location, Available: true}
	if u.Location == "" {
		u.Location = "local"
		if s, ok := module.Lookup[contracts.MaintenanceStorage](r.Registry, contracts.MaintenanceStorageKey); ok {
			u.Location = s.Location()
		}
	}
	for info, err := range r.Store.List(ctx, r.Prefix) {
		if err != nil {
			return nil, err
		}
		u.Bytes += info.Size
		u.Files++
	}
	return []contracts.StorageUsage{u}, nil
}

func StoredStat(ctx context.Context, d *module.Deps, scope, key string) (files.Info, error) {
	if raw, ok := module.Lookup[contracts.MaintenanceStorage](d.Registry, contracts.MaintenanceStorageKey); ok {
		return raw.Stat(ctx, scope, key)
	}
	return d.Files.For(scope).Stat(ctx, key)
}

func DeleteObject(ctx context.Context, d *module.Deps, scope, key string) (int64, error) {
	info, err := StoredStat(ctx, d, scope, key)
	if err != nil && !errors.Is(err, files.ErrNotFound) {
		return 0, err
	}
	if e := d.Files.For(scope).Delete(ctx, key); e != nil {
		return 0, e
	}
	if errors.Is(err, files.ErrNotFound) {
		return 0, nil
	}
	return info.Size, nil
}

var noteReference = regexp.MustCompile(`/api/v1/notes/attachments/([0-9]+)(?:\b)`)
var uploadReference = regexp.MustCompile(`/api/v1/files/([0-9]+)(?:\b)`)

func References(text string, id int64, notes bool) bool {
	r := uploadReference
	if notes {
		r = noteReference
	}
	for _, match := range r.FindAllStringSubmatch(text, -1) {
		n, err := strconv.ParseInt(match[1], 10, 64)
		if err == nil && n == id {
			return true
		}
	}
	return false
}
