package repo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (v Retention) Validate() error {
	if v.Last < 1 || v.Last > 365 || v.Daily < 0 || v.Daily > 3650 || v.Weekly < 0 || v.Weekly > 520 || v.Monthly < 0 || v.Monthly > 120 {
		return fmt.Errorf("备份保留规则超出范围")
	}
	return nil
}

func Keep(snapshots []Snapshot, v Retention, now time.Time, loc *time.Location) map[string]bool {
	if loc == nil {
		loc = time.UTC
	}
	all := append([]Snapshot(nil), snapshots...)
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	keep := map[string]bool{}
	for i := 0; i < min(v.Last, len(all)); i++ {
		keep[all[i].ID] = true
	}
	local := now.In(loc)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	week := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	month := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	seenDay, seenWeek, seenMonth := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, s := range all {
		at := s.CreatedAt.In(loc)
		if s.CreatedAt.After(now) {
			keep[s.ID] = true
			continue
		}
		d := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, loc)
		w := d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
		m := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, loc)
		dk, wk, mk := d.Format("2006-01-02"), w.Format("2006-01-02"), m.Format("2006-01")
		if v.Daily > 0 && !d.Before(day.AddDate(0, 0, -v.Daily+1)) && !seenDay[dk] {
			keep[s.ID] = true
			seenDay[dk] = true
		}
		if v.Weekly > 0 && !w.Before(week.AddDate(0, 0, -7*(v.Weekly-1))) && !seenWeek[wk] {
			keep[s.ID] = true
			seenWeek[wk] = true
		}
		if v.Monthly > 0 && !m.Before(month.AddDate(0, -v.Monthly+1, 0)) && !seenMonth[mk] {
			keep[s.ID] = true
			seenMonth[mk] = true
		}
	}
	return keep
}

func references(all []Snapshot) (map[string]Block, error) {
	refs := map[string]Block{}
	for _, s := range all {
		ff := append([]File{s.Database}, s.Files...)
		for _, f := range ff {
			for _, b := range f.Blocks {
				if old, ok := refs[b.Hash]; ok && old != b {
					return nil, ErrCorrupt
				}
				refs[b.Hash] = b
			}
		}
	}
	return refs, nil
}

func (r *Repository) Stats(ctx context.Context, all []Snapshot) (Stats, error) {
	v := Stats{Snapshots: len(all)}
	refs, err := references(all)
	if err != nil {
		return v, err
	}
	for _, s := range all {
		if v.LogicalBytes > (1<<63-1)-s.SizeBytes {
			return v, ErrCorrupt
		}
		v.LogicalBytes += s.SizeBytes
	}
	for _, b := range refs {
		if v.UniqueBytes > (1<<63-1)-b.Size {
			return v, ErrCorrupt
		}
		v.UniqueBytes += b.Size
	}
	if len(all) > 0 {
		at := all[0].CreatedAt
		v.LastSnapshotAt = &at
	}
	for info, err := range r.store.List(ctx, "") {
		if err != nil {
			return v, err
		}
		if info.Size < 0 || v.SizeBytes > (1<<63-1)-info.Size {
			return v, ErrCorrupt
		}
		v.SizeBytes += info.Size
	}
	return v, nil
}

func (r *Repository) Check(ctx context.Context) (CheckResult, error) {
	v := CheckResult{CheckedAt: r.now().UTC()}
	if err := r.writable(ctx); err != nil {
		return v, err
	}
	all, err := r.ListSnapshots(ctx)
	if err != nil {
		return v, err
	}
	v.Snapshots = len(all)
	refs, err := references(all)
	if err != nil {
		return v, err
	}
	for hash, b := range refs {
		info, err := r.store.Stat(ctx, blobPath(hash))
		if err != nil {
			return v, fmt.Errorf("检查块 %s 失败：%w", hash, err)
		}
		if info.Size != b.StoredSize {
			return v, fmt.Errorf("%w：块 %s 大小不一致", ErrCorrupt, hash)
		}
		v.Blocks++
	}
	return v, nil
}

func (r *Repository) Prune(ctx context.Context, retention Retention, now time.Time, loc *time.Location) (PruneResult, error) {
	var result PruneResult
	if err := retention.Validate(); err != nil {
		return result, err
	}
	if err := r.writable(ctx); err != nil {
		return result, err
	}
	all, err := r.ListSnapshots(ctx)
	if err != nil {
		return result, err
	}
	if _, err := references(all); err != nil {
		return result, err
	}
	keep := Keep(all, retention, now, loc)
	for _, s := range all {
		if keep[s.ID] {
			continue
		}
		if err := r.writable(ctx); err != nil {
			return result, err
		}
		if err := r.store.Delete(ctx, snapshotPath(s.ID)); err != nil {
			return result, err
		}
		result.Snapshots++
	}
	remaining, err := r.ListSnapshots(ctx)
	if err != nil {
		return result, err
	}
	if len(remaining) != len(keep) {
		return result, fmt.Errorf("清理后的快照列表不完整，已停止清理块")
	}
	for _, s := range remaining {
		if !keep[s.ID] {
			return result, fmt.Errorf("过期快照删除后仍存在，已停止清理块")
		}
	}
	refs, err := references(remaining)
	if err != nil {
		return result, err
	}
	var stale []struct {
		key  string
		size int64
	}
	for info, err := range r.store.List(ctx, "blobs") {
		if err != nil {
			return result, err
		}
		_, hash, ok := strings.Cut(strings.TrimPrefix(info.Key, "blobs/"), "/")
		if !ok || !validHex(hash, 64) || blobPath(hash) != info.Key {
			return result, ErrCorrupt
		}
		if _, ok := refs[hash]; !ok {
			stale = append(stale, struct {
				key  string
				size int64
			}{info.Key, info.Size})
		}
	}
	for _, b := range stale {
		if err := r.writable(ctx); err != nil {
			return result, err
		}
		if err := r.store.Delete(ctx, b.key); err != nil {
			return result, err
		}
		result.Blocks++
		result.Bytes += b.size
	}
	return result, nil
}
