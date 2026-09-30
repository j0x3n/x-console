package drive

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
)

type transferSource struct {
	item  db.DriveItem
	count int
	bytes int64
}

type copyEntry struct {
	sourceID int64
	parent   *int64
	top      bool
}

func (m *Module) CopyDriveItems(w http.ResponseWriter, r *http.Request) {
	m.transfer(w, r, api.Copy)
}

func (m *Module) MoveDriveItems(w http.ResponseWriter, r *http.Request) {
	m.transfer(w, r, api.Move)
}

func (m *Module) transfer(w http.ResponseWriter, r *http.Request, kind api.DriveTaskKind) {
	ctx := r.Context()
	var body api.BatchTransfer
	if fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	if len(body.Ids) == 0 || !body.Conflict.Valid() || body.TargetId < 0 {
		httpx.Fail(w, r, httpx.Invalid("批量操作参数不正确"))
		return
	}
	var target *int64
	targetName := "/"
	if body.TargetId != 0 {
		folder, err := m.visibleRow(ctx, body.TargetId)
		if fail(w, r, err) {
			return
		}
		if folder.TrashedAt != nil || folder.IsDir == 0 {
			httpx.Fail(w, r, httpx.NewError(400, "invalid_target", "目标文件夹不可用"))
			return
		}
		target = &folder.ID
		path, err := m.path(ctx, folder)
		if fail(w, r, err) {
			return
		}
		for _, p := range path {
			targetName += p.Name + "/"
		}
		targetName += folder.Name
	}
	selected := make(map[int64]bool, len(body.Ids))
	sources := make([]transferSource, 0, len(body.Ids))
	for _, id := range body.Ids {
		item, err := m.visibleRow(ctx, id)
		if fail(w, r, err) {
			return
		}
		if item.TrashedAt != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		if target != nil {
			folder, err := m.row(ctx, *target)
			if fail(w, r, err) {
				return
			}
			if item.Hidden != folder.Hidden {
				httpx.Fail(w, r, httpx.NewError(400, "hidden_mismatch", "隐藏状态和目标文件夹不一致"))
				return
			}
			if item.IsDir != 0 {
				var cycle int
				err = m.d.DB.QueryRowContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) SELECT count(*) FROM subtree WHERE id=?`, id, *target).Scan(&cycle)
				if fail(w, r, err) {
					return
				}
				if cycle != 0 {
					httpx.Fail(w, r, httpx.NewError(400, "invalid_target", "不能放到自己的子文件夹"))
					return
				}
			}
		}
		if !selected[id] {
			selected[id] = true
			sources = append(sources, transferSource{item: item})
		}
	}
	// If both a folder and one of its children were selected, process the folder once.
	filtered := sources[:0]
	for _, source := range sources {
		ancestor := source.item.ParentID
		covered := false
		for ancestor != nil {
			if selected[*ancestor] {
				covered = true
				break
			}
			row, err := m.row(ctx, *ancestor)
			if fail(w, r, err) {
				return
			}
			ancestor = row.ParentID
		}
		if !covered {
			filtered = append(filtered, source)
		}
	}
	sources = filtered
	totalItems := 0
	var totalBytes int64
	for i := range sources {
		err := m.d.DB.QueryRowContext(ctx, `WITH RECURSIVE subtree(id,is_dir,size) AS (SELECT id,is_dir,size FROM drive_items WHERE id=? UNION ALL SELECT d.id,d.is_dir,d.size FROM drive_items d JOIN subtree s ON d.parent_id=s.id WHERE d.trashed_at IS NULL) SELECT count(*),COALESCE(sum(CASE WHEN is_dir=0 THEN size ELSE 0 END),0) FROM subtree`, sources[i].item.ID).Scan(&sources[i].count, &sources[i].bytes)
		if fail(w, r, err) {
			return
		}
		totalItems += sources[i].count
		totalBytes += sources[i].bytes
	}
	verb := "复制"
	if kind == api.Move {
		verb = "移动"
	}
	title := fmt.Sprintf("%s %d 项到 %s", verb, len(sources), targetName)
	start := m.startTask
	for _, source := range sources {
		if source.item.Hidden != 0 {
			start, title = m.startHiddenTask, fmt.Sprintf("%s隐藏空间里的 %d 项", verb, len(sources))
			break
		}
	}
	task := start(kind, title, totalItems, totalBytes, func(jobCtx context.Context, t *driveTask) error {
		var err error
		if kind == api.Copy {
			err = m.copyItems(jobCtx, t, sources, target, body.Conflict)
		} else {
			err = m.moveItems(jobCtx, t, sources, target, body.Conflict)
		}
		m.d.Audit.Record(context.WithoutCancel(jobCtx), "drive."+string(kind), strconv.FormatInt(body.TargetId, 10), map[string]any{"items": len(sources), "targetId": body.TargetId}, err)
		if err == nil {
			m.triggerSync()
		}
		return err
	}, body.TargetId)
	httpx.JSON(w, http.StatusAccepted, task)
}

// transferName resolves conflicts inside the same transaction as the change.
// An existing item of a different type is renamed rather than sent to trash.
func (m *Module) transferName(ctx context.Context, tx *sql.Tx, item db.DriveItem, parent *int64, policy api.ConflictPolicy, copying bool) (string, bool, error) {
	var existingID, isDir int64
	err := tx.QueryRowContext(ctx, "SELECT id,is_dir FROM drive_items WHERE parent_id IS ? AND hidden=? AND name=? AND trashed_at IS NULL LIMIT 1", parent, item.Hidden, item.Name).Scan(&existingID, &isDir)
	if err != nil && err != sql.ErrNoRows {
		return "", false, err
	}
	if err == sql.ErrNoRows {
		return item.Name, false, nil
	}
	if existingID == item.ID {
		if !copying {
			return item.Name, true, nil
		}
		if policy == api.Skip {
			return item.Name, true, nil
		}
		return m.renamedTransfer(ctx, tx, item, parent)
	}
	switch policy {
	case api.Skip:
		return item.Name, true, nil
	case api.Rename:
		return m.renamedTransfer(ctx, tx, item, parent)
	case api.Overwrite:
		if isDir != item.IsDir {
			return m.renamedTransfer(ctx, tx, item, parent)
		}
		now := time.Now().UTC()
		_, err = tx.ExecContext(ctx, `WITH RECURSIVE subtree(id) AS (SELECT id FROM drive_items WHERE id=? UNION ALL SELECT d.id FROM drive_items d JOIN subtree s ON d.parent_id=s.id) UPDATE drive_items SET trashed_at=?,updated_at=? WHERE id IN (SELECT id FROM subtree) AND trashed_at IS NULL`, existingID, now, now)
		return item.Name, false, err
	}
	return "", false, httpx.Invalid("冲突策略不正确")
}

func (m *Module) renamedTransfer(ctx context.Context, tx *sql.Tx, item db.DriveItem, parent *int64) (string, bool, error) {
	name, err := m.freeName(ctx, tx, parent, item.Hidden != 0, item.Name, 0)
	return name, false, err
}

func (m *Module) moveItems(ctx context.Context, t *driveTask, sources []transferSource, target *int64, policy api.ConflictPolicy) error {
	done := 0
	var bytes int64
	skipped := 0
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}
		wasSkipped := false
		err := m.write(ctx, func(tx *sql.Tx) error {
			item, err := db.New(tx).GetItem(ctx, source.item.ID)
			if err != nil {
				return err
			}
			if item.TrashedAt != nil {
				return httpx.ErrNotFound
			}
			if target != nil {
				cycle, err := m.descendant(ctx, tx, item.ID, *target)
				if err != nil {
					return err
				}
				if cycle {
					return httpx.NewError(400, "invalid_target", "不能放到自己的子文件夹")
				}
			}
			name, skip, err := m.transferName(ctx, tx, item, target, policy, false)
			if err != nil {
				return err
			}
			if skip {
				wasSkipped = true
				return nil
			}
			_, err = tx.ExecContext(ctx, "UPDATE drive_items SET parent_id=?,name=?,hidden_from=NULL,updated_at=? WHERE id=?", target, name, time.Now().UTC(), item.ID)
			return err
		})
		if err != nil {
			return err
		}
		if wasSkipped {
			skipped++
		} else {
			done += source.count
			bytes += source.bytes
		}
		t.progress(source.item.Name, done, bytes)
		m.changeTask(t, false, func(dto *api.DriveTask) { value := skipped; dto.Skipped = &value })
	}
	return nil
}

func (m *Module) copyItems(ctx context.Context, t *driveTask, sources []transferSource, target *int64, policy api.ConflictPolicy) error {
	queue := make([]copyEntry, 0, len(sources))
	for _, source := range sources {
		queue = append(queue, copyEntry{sourceID: source.item.ID, parent: target, top: true})
	}
	done := 0
	var bytes int64
	skipped := 0
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := queue
		if len(batch) > 200 {
			batch = batch[:200]
		}
		var next []copyEntry
		var batchDone int
		var batchBytes int64
		var batchSkipped int
		current := ""
		err := m.write(ctx, func(tx *sql.Tx) error {
			q := db.New(tx)
			for _, entry := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				item, err := q.GetItem(ctx, entry.sourceID)
				if err != nil {
					return err
				}
				if item.TrashedAt != nil {
					return httpx.ErrNotFound
				}
				name := item.Name
				if entry.top {
					var skip bool
					name, skip, err = m.transferName(ctx, tx, item, entry.parent, policy, true)
					if err != nil {
						return err
					}
					if skip {
						batchSkipped++
						continue
					}
				}
				now := time.Now().UTC()
				created, err := q.InsertItem(ctx, db.InsertItemParams{ParentID: entry.parent, Name: name, IsDir: item.IsDir, Size: item.Size, Mime: item.Mime, Sha256: item.Sha256, Hidden: item.Hidden, CreatedAt: now, UpdatedAt: now})
				if err != nil {
					return err
				}
				batchDone++
				if item.IsDir == 0 {
					batchBytes += item.Size
				}
				current = name
				if item.IsDir != 0 {
					rows, err := tx.QueryContext(ctx, "SELECT id FROM drive_items WHERE parent_id=? AND trashed_at IS NULL ORDER BY id", item.ID)
					if err != nil {
						return err
					}
					for rows.Next() {
						var id int64
						if err = rows.Scan(&id); err != nil {
							break
						}
						next = append(next, copyEntry{sourceID: id, parent: &created.ID})
					}
					if err == nil {
						err = rows.Err()
					}
					rows.Close()
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		queue = append(queue[len(batch):], next...)
		done += batchDone
		bytes += batchBytes
		skipped += batchSkipped
		t.progress(current, done, bytes)
		m.changeTask(t, false, func(dto *api.DriveTask) { value := skipped; dto.Skipped = &value })
	}
	return nil
}
