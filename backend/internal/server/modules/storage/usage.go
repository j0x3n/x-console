package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
)

// usageCache keeps the per-module numbers for ten minutes. Counting files in
// a bucket means listing it, which is slow and costs requests.
type usageCache struct {
	mu   sync.Mutex
	at   time.Time
	list []api.ModuleUsage
}

func (c *usageCache) get(now time.Time) ([]api.ModuleUsage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.at.IsZero() || now.Sub(c.at) >= usageTTL {
		return nil, false
	}
	return c.list, true
}

func (c *usageCache) put(now time.Time, list []api.ModuleUsage) {
	c.mu.Lock()
	c.at, c.list = now, list
	c.mu.Unlock()
}

func (c *usageCache) clear() {
	c.mu.Lock()
	c.at = time.Time{}
	c.mu.Unlock()
}

// countUsage lists every file and adds them up per module.
func countUsage(ctx context.Context, s files.Store) ([]api.ModuleUsage, error) {
	byModule := map[string]*api.ModuleUsage{}
	for info, err := range s.List(ctx, "") {
		if err != nil {
			return nil, err
		}
		name := files.Module(info.Key)
		u, ok := byModule[name]
		if !ok {
			u = &api.ModuleUsage{Module: name}
			byModule[name] = u
		}
		u.Files++
		u.Bytes += info.Size
	}
	out := make([]api.ModuleUsage, 0, len(byModule))
	for _, u := range byModule {
		out = append(out, *u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Module < out[j].Module })
	return out, nil
}
