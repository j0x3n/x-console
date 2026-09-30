package projects

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/audit"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects/db"
)

// B46 多看板：项目 → 看板 → 列表 → 卡片。卡片就是 issue，编号不变。
// 一列就是一个列表；列表可以对应一个状态，卡片进来时状态改成它。

// statusNames are the Chinese list names for the six statuses.
var statusNames = map[string]string{
	"backlog": "待规划", "todo": "待办", "in_progress": "进行中",
	"in_review": "待审核", "done": "已完成", "canceled": "已取消",
}

// presetLists are the lists a new board starts with.
func presetLists(preset string) []struct{ name, status string } {
	switch preset {
	case "empty":
		return nil
	case "simple":
		return []struct{ name, status string }{{"待处理", "todo"}, {"进行中", "in_progress"}, {"已完成", "done"}}
	}
	out := make([]struct{ name, status string }, 0, len(statuses))
	for _, s := range statuses {
		out = append(out, struct{ name, status string }{statusNames[s], s})
	}
	return out
}

// createBoardTx adds a board with preset lists at the end of the project.
func (m *Module) createBoardTx(ctx context.Context, q *db.Queries, projectID int64, name, icon, preset string) (db.ProjectBoard, error) {
	pos, err := q.MaxBoardPosition(ctx, projectID)
	if err != nil {
		return db.ProjectBoard{}, err
	}
	now := m.now()
	b, err := q.CreateBoard(ctx, db.CreateBoardParams{ProjectID: projectID, Name: name, Icon: icon,
		Position: pos + sortGap, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return b, err
	}
	for i, l := range presetLists(preset) {
		status := l.status
		if _, err := q.CreateList(ctx, db.CreateListParams{BoardID: b.ID, Name: l.name, Position: float64(i+1) * sortGap,
			Status: &status, CreatedAt: now}); err != nil {
			return b, err
		}
	}
	return b, nil
}

// placeFor picks the board and list of a new card. A given list wins, then a
// given board, then the first board of the project. Inside a board it takes
// the list for the status, else the first list. The project gets a default
// board when it has none.
func (m *Module) placeFor(ctx context.Context, q *db.Queries, projectID int64, boardID, listID *int64, status string) (db.BoardList, error) {
	if listID != nil {
		l, err := q.GetList(ctx, *listID)
		if err != nil {
			return db.BoardList{}, notFound(err)
		}
		if l.ProjectID != projectID {
			return db.BoardList{}, httpx.Invalid("列表不在这个项目里")
		}
		if l.ArchivedAt != nil {
			return db.BoardList{}, httpx.Invalid("列表已归档")
		}
		return listOf(l), nil
	}
	var board db.ProjectBoard
	var err error
	if boardID != nil {
		board, err = q.GetBoard(ctx, *boardID)
		if err != nil {
			return db.BoardList{}, notFound(err)
		}
		if board.ProjectID != projectID {
			return db.BoardList{}, httpx.Invalid("看板不在这个项目里")
		}
	} else {
		board, err = q.FirstBoard(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			board, err = m.createBoardTx(ctx, q, projectID, "看板", "", "statuses")
		}
		if err != nil {
			return db.BoardList{}, err
		}
	}
	l, err := q.ListForStatus(ctx, db.ListForStatusParams{BoardID: board.ID, Status: &status})
	if errors.Is(err, sql.ErrNoRows) {
		l, err = q.FirstList(ctx, board.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return db.BoardList{}, httpx.Invalid("看板“" + board.Name + "”里还没有列表")
		}
	}
	return l, err
}

func listOf(r db.GetListRow) db.BoardList {
	return db.BoardList{ID: r.ID, BoardID: r.BoardID, Name: r.Name, Position: r.Position, Status: r.Status,
		Color: r.Color, WipLimit: r.WipLimit, Collapsed: r.Collapsed, ArchivedAt: r.ArchivedAt, CreatedAt: r.CreatedAt}
}

// topOfList is the sort order that puts a card first in a list.
func topOfList(ctx context.Context, q *db.Queries, listID, self int64) (float64, error) {
	n, err := q.CountInColumn(ctx, db.CountInColumnParams{ListID: &listID, ID: self})
	if err != nil || n == 0 {
		return 0, err
	}
	top, err := q.MinSortOrder(ctx, db.MinSortOrderParams{ListID: &listID, ID: self})
	if err != nil {
		return 0, err
	}
	v, _ := between(nil, &top)
	return v, nil
}

// bottomOfList is the sort order that puts a card last in a list.
func bottomOfList(ctx context.Context, q *db.Queries, listID, self int64) (float64, error) {
	n, err := q.CountInColumn(ctx, db.CountInColumnParams{ListID: &listID, ID: self})
	if err != nil || n == 0 {
		return 0, err
	}
	last, err := q.MaxSortOrder(ctx, db.MaxSortOrderParams{ListID: &listID, ID: self})
	if err != nil {
		return 0, err
	}
	v, _ := between(&last, nil)
	return v, nil
}

// rebalanceList renumbers a list (without self) with even gaps.
func rebalanceList(ctx context.Context, q *db.Queries, listID, self int64) error {
	ids, err := q.ListColumn(ctx, db.ListColumnParams{ListID: &listID, ID: self})
	if err != nil {
		return err
	}
	for i, v := range spread(len(ids)) {
		if err := q.SetSortOrder(ctx, db.SetSortOrderParams{SortOrder: v, ID: ids[i]}); err != nil {
			return err
		}
	}
	return nil
}

// slotIn finds the sort order for a card between two neighbour cards of a
// list. Keys that are empty mean "no neighbour"; both empty means the end.
func (m *Module) slotIn(ctx context.Context, q *db.Queries, listID, self int64, afterKey, beforeKey *string) (float64, error) {
	anchor := func(k *string) (*float64, error) {
		if k == nil || *k == "" {
			return nil, nil
		}
		a, err := m.findIssue(ctx, q, *k)
		if err != nil {
			return nil, err
		}
		if a.Issue.ListID == nil || *a.Issue.ListID != listID || a.Issue.ID == self || a.Issue.ArchivedAt != nil {
			return nil, httpx.Invalid("落点旁边的卡片不在这个列表里")
		}
		return &a.Issue.SortOrder, nil
	}
	col := func(v float64, err error) (*float64, error) {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &v, nil
	}
	for attempt := 0; ; attempt++ {
		prev, err := anchor(afterKey)
		if err != nil {
			return 0, err
		}
		next, err := anchor(beforeKey)
		if err != nil {
			return 0, err
		}
		switch {
		case prev != nil && next == nil:
			if next, err = col(q.NextInColumn(ctx, db.NextInColumnParams{ListID: &listID, ID: self, SortOrder: *prev})); err != nil {
				return 0, err
			}
		case next != nil && prev == nil:
			if prev, err = col(q.PrevInColumn(ctx, db.PrevInColumnParams{ListID: &listID, ID: self, SortOrder: *next})); err != nil {
				return 0, err
			}
		case prev == nil && next == nil:
			return bottomOfList(ctx, q, listID, self)
		}
		if sort, ok := between(prev, next); ok {
			return sort, nil
		}
		if attempt > 0 {
			return 0, httpx.Invalid("落点不正确")
		}
		if err := rebalanceList(ctx, q, listID, self); err != nil {
			return 0, err
		}
	}
}

// placeCard moves a card into a list at sort, changing project (and number),
// board and status as needed. It returns the card's old status.
func (m *Module) placeCard(ctx context.Context, q *db.Queries, i db.Issue, target db.BoardList, sort float64) (string, error) {
	board, err := q.GetBoard(ctx, target.BoardID)
	if err != nil {
		return "", notFound(err)
	}
	from := i.Status
	status := i.Status
	if target.Status != nil {
		status = *target.Status
	}
	now := m.now()
	number := i.Number
	if board.ProjectID != i.ProjectID {
		next, err := q.TakeIssueNumber(ctx, board.ProjectID)
		if err != nil {
			return "", err
		}
		number = next - 1
		// Milestones, categories and project labels belong to the old project.
		if err := q.UpdateIssue(ctx, db.UpdateIssueParams{
			Title: i.Title, Description: i.Description, Status: i.Status, Priority: i.Priority, DueDate: i.DueDate,
			MilestoneID: nil, SortOrder: i.SortOrder, UpdatedAt: now, CompletedAt: i.CompletedAt, ID: i.ID,
			CategoryID: nil, DueAt: i.DueAt, DueRemind: i.DueRemind, DueNotifiedAt: i.DueNotifiedAt,
		}); err != nil {
			return "", err
		}
		if err := q.RemoveForeignLabels(ctx, db.RemoveForeignLabelsParams{IssueID: i.ID, ProjectID: &board.ProjectID}); err != nil {
			return "", err
		}
	}
	listID, boardID := target.ID, board.ID
	return from, q.SetIssuePlace(ctx, db.SetIssuePlaceParams{
		ProjectID: board.ProjectID, Number: number, BoardID: &boardID, ListID: &listID, Status: status,
		SortOrder: sort, CompletedAt: completedAt(i.CompletedAt, i.Status, status, now), UpdatedAt: now, ID: i.ID,
	})
}

// activity records what happened to a card. actor comes from the context.
func activity(ctx context.Context, q *db.Queries, issueID int64, kind string, data map[string]any, at time.Time) error {
	actor := audit.Actor(ctx)
	if actor == "" {
		actor = "me"
	}
	raw := []byte("{}")
	if data != nil {
		raw, _ = json.Marshal(data)
	}
	return q.AddActivity(ctx, db.AddActivityParams{IssueID: issueID, At: at, Actor: actor, Kind: kind, Data: string(raw)})
}

// moveToList is the B46 move: into any list, between two neighbours.
func (m *Module) moveToList(ctx context.Context, key string, listID int64, afterKey, beforeKey *string) (out api.Issue, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "issue.move", key, map[string]any{"listId": listID}, err)
	}()
	var id int64
	var from string
	err = m.tx(ctx, func(q *db.Queries) error {
		row, err := m.findIssue(ctx, q, key)
		if err != nil {
			return err
		}
		i := row.Issue
		id = i.ID
		if i.ArchivedAt != nil {
			return httpx.Invalid("卡片已归档，先恢复")
		}
		l, err := q.GetList(ctx, listID)
		if err != nil {
			return notFound(err)
		}
		if l.ArchivedAt != nil {
			return httpx.Invalid("列表已归档")
		}
		sort, err := m.slotIn(ctx, q, listID, i.ID, afterKey, beforeKey)
		if err != nil {
			return err
		}
		oldList := i.ListID
		if from, err = m.placeCard(ctx, q, i, listOf(l), sort); err != nil {
			return err
		}
		if oldList == nil || *oldList != listID {
			return activity(ctx, q, i.ID, "moved", map[string]any{"toList": l.Name, "listId": listID}, m.now())
		}
		return nil
	})
	if err != nil {
		return out, err
	}
	out, err = m.issueByID(ctx, id)
	if err != nil {
		return out, err
	}
	m.publishUpdate(out, from)
	m.d.Bus.Publish("issue.moved", map[string]any{"key": out.Key, "listId": listID})
	return out, nil
}

// moveIssue is the old move by status: into the list of the card's board
// that stands for the status.
func (m *Module) moveIssue(ctx context.Context, key, status string, afterKey, beforeKey *string) (api.Issue, error) {
	if !validStatus(status) {
		return api.Issue{}, httpx.Invalid("状态不正确")
	}
	row, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return api.Issue{}, err
	}
	if row.Issue.BoardID == nil {
		return api.Issue{}, httpx.Invalid("卡片不在看板上")
	}
	l, err := m.q.ListForStatus(ctx, db.ListForStatusParams{BoardID: *row.Issue.BoardID, Status: &status})
	if errors.Is(err, sql.ErrNoRows) {
		return api.Issue{}, httpx.Invalid("这个看板没有对应“" + statusNames[status] + "”的列表")
	}
	if err != nil {
		return api.Issue{}, err
	}
	return m.moveToList(ctx, key, l.ID, afterKey, beforeKey)
}

// followStatus moves a card whose status changed from outside (a patch,
// Linear, an agent) into the first list of its board for the new status,
// on top. Without such a list the card stays.
func (m *Module) followStatus(ctx context.Context, q *db.Queries, id int64, status string) error {
	r, err := q.GetIssue(ctx, id)
	if err != nil {
		return err
	}
	i := r.Issue
	if i.BoardID == nil || i.ArchivedAt != nil {
		return nil
	}
	if i.ListID != nil {
		cur, err := q.GetList(ctx, *i.ListID)
		if err == nil && cur.Status != nil && *cur.Status == status {
			return nil
		}
	}
	l, err := q.ListForStatus(ctx, db.ListForStatusParams{BoardID: *i.BoardID, Status: &status})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	sort, err := topOfList(ctx, q, l.ID, i.ID)
	if err != nil {
		return err
	}
	listID := l.ID
	if err := q.SetIssuePlace(ctx, db.SetIssuePlaceParams{ProjectID: i.ProjectID, Number: i.Number, BoardID: i.BoardID,
		ListID: &listID, Status: i.Status, SortOrder: sort, CompletedAt: i.CompletedAt, UpdatedAt: i.UpdatedAt, ID: i.ID}); err != nil {
		return err
	}
	return activity(ctx, q, i.ID, "moved", map[string]any{"toList": l.Name, "listId": l.ID, "byStatus": status}, m.now())
}

// ---- boards ----

func toBoard(b db.ProjectBoard, lists []api.BoardList) api.Board {
	if lists == nil {
		lists = []api.BoardList{}
	}
	return api.Board{Id: b.ID, ProjectId: b.ProjectID, Name: b.Name, Icon: b.Icon, Position: b.Position,
		Starred: b.Starred == 1, ArchivedAt: b.ArchivedAt, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, Lists: lists}
}

func toList(l db.BoardList, count int) api.BoardList {
	out := api.BoardList{Id: l.ID, BoardId: l.BoardID, Name: l.Name, Position: l.Position, Color: l.Color,
		WipLimit: int(l.WipLimit), Collapsed: l.Collapsed == 1, ArchivedAt: l.ArchivedAt, CardCount: count}
	if l.Status != nil {
		s := api.IssueStatus(*l.Status)
		out.Status = &s
	}
	return out
}

func archivedFlag(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// boardWithLists loads a board with its lists and card counts.
func (m *Module) boardWithLists(ctx context.Context, q *db.Queries, b db.ProjectBoard, archived bool) (api.Board, error) {
	lists, err := q.ListLists(ctx, db.ListListsParams{BoardID: b.ID, Archived: archivedFlag(archived)})
	if err != nil {
		return api.Board{}, err
	}
	counts, err := q.CountListCards(ctx, &b.ID)
	if err != nil {
		return api.Board{}, err
	}
	n := map[int64]int{}
	for _, c := range counts {
		if c.ListID != nil {
			n[*c.ListID] = int(c.N)
		}
	}
	out := make([]api.BoardList, 0, len(lists))
	for _, l := range lists {
		out = append(out, toList(l, n[l.ID]))
	}
	return toBoard(b, out), nil
}

func (m *Module) listBoards(ctx context.Context, projectID int64, archived bool) ([]api.Board, error) {
	if _, err := m.q.GetProject(ctx, projectID); err != nil {
		return nil, notFound(err)
	}
	boards, err := m.q.ListBoards(ctx, db.ListBoardsParams{ProjectID: projectID, Archived: archivedFlag(archived)})
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 && !archived {
		// A project made before B46 without issues, or one whose boards
		// were all archived: give it a board to work in.
		var b db.ProjectBoard
		if err := m.tx(ctx, func(q *db.Queries) error {
			b, err = m.createBoardTx(ctx, q, projectID, "看板", "", "statuses")
			return err
		}); err != nil {
			return nil, err
		}
		boards = []db.ProjectBoard{b}
	}
	out := make([]api.Board, 0, len(boards))
	for _, b := range boards {
		v, err := m.boardWithLists(ctx, m.q, b, archived)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func cleanName(s string, what string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", httpx.Invalid(what + "名称不能为空")
	}
	if len([]rune(s)) > 60 {
		return "", httpx.Invalid(what + "名称最多 60 个字")
	}
	return s, nil
}

func (m *Module) createBoard(ctx context.Context, projectID int64, in api.CreateBoard) (out api.Board, err error) {
	defer func() { m.d.Audit.Record(ctx, "board.create", in.Name, map[string]any{"projectId": projectID}, err) }()
	name, err := cleanName(in.Name, "看板")
	if err != nil {
		return out, err
	}
	preset := "statuses"
	if in.Preset != nil {
		preset = string(*in.Preset)
	}
	err = m.tx(ctx, func(q *db.Queries) error {
		p, err := q.GetProject(ctx, projectID)
		if err != nil {
			return notFound(err)
		}
		if p.Project.ArchivedAt != nil {
			return httpx.Invalid("项目已归档")
		}
		b, err := m.createBoardTx(ctx, q, projectID, name, deref(in.Icon), preset)
		if err != nil {
			return err
		}
		out, err = m.boardWithLists(ctx, q, b, false)
		return err
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
	}
	return out, err
}

// positionAfter returns a position right after afterID among the siblings
// (0 means first). positions and ids are in order.
func positionAfter(afterID int64, ids []int64, positions []float64, self int64) (float64, bool) {
	var prev, next *float64
	found := afterID == 0
	for i, id := range ids {
		if id == self {
			continue
		}
		if found {
			next = &positions[i]
			break
		}
		if id == afterID {
			prev = &positions[i]
			found = true
		}
	}
	if !found {
		return 0, false
	}
	v, ok := between(prev, next)
	return v, ok
}

func (m *Module) updateBoard(ctx context.Context, id int64, in api.UpdateBoard) (out api.Board, err error) {
	defer func() { m.d.Audit.Record(ctx, "board.update", "", map[string]any{"id": id}, err) }()
	err = m.tx(ctx, func(q *db.Queries) error {
		b, err := q.GetBoard(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if in.Name != nil {
			if b.Name, err = cleanName(*in.Name, "看板"); err != nil {
				return err
			}
		}
		if in.Icon != nil {
			b.Icon = strings.TrimSpace(*in.Icon)
		}
		if in.Starred != nil {
			b.Starred = archivedFlag(*in.Starred)
		}
		if in.Archived != nil {
			if *in.Archived && b.ArchivedAt == nil {
				open, err := q.ListBoards(ctx, db.ListBoardsParams{ProjectID: b.ProjectID})
				if err != nil {
					return err
				}
				if len(open) <= 1 {
					return httpx.Invalid("项目至少要留一个看板")
				}
				now := m.now()
				b.ArchivedAt = &now
			} else if !*in.Archived {
				b.ArchivedAt = nil
			}
		}
		if in.AfterId != nil {
			boards, err := q.ListBoards(ctx, db.ListBoardsParams{ProjectID: b.ProjectID, Archived: 1})
			if err != nil {
				return err
			}
			ids, pos := make([]int64, len(boards)), make([]float64, len(boards))
			for i, x := range boards {
				ids[i], pos[i] = x.ID, x.Position
			}
			p, ok := positionAfter(*in.AfterId, ids, pos, b.ID)
			if !ok {
				// Too close or unknown neighbour: renumber, then try again.
				for i, x := range boards {
					pos[i] = float64(i+1) * sortGap
					if err := q.SetBoardPosition(ctx, db.SetBoardPositionParams{Position: pos[i], ID: x.ID}); err != nil {
						return err
					}
				}
				if p, ok = positionAfter(*in.AfterId, ids, pos, b.ID); !ok {
					return httpx.Invalid("排序位置不正确")
				}
			}
			b.Position = p
		}
		nb, err := q.UpdateBoard(ctx, db.UpdateBoardParams{Name: b.Name, Icon: b.Icon, Position: b.Position,
			Starred: b.Starred, ArchivedAt: b.ArchivedAt, UpdatedAt: m.now(), ID: b.ID})
		if err != nil {
			return err
		}
		out, err = m.boardWithLists(ctx, q, nb, false)
		return err
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": out.ProjectId})
	}
	return out, err
}

// deleteBoard moves the board's cards to the project's first other board
// and deletes it.
func (m *Module) deleteBoard(ctx context.Context, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "board.delete", "", map[string]any{"id": id}, err) }()
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		b, err := q.GetBoard(ctx, id)
		if err != nil {
			return notFound(err)
		}
		projectID = b.ProjectID
		boards, err := q.ListBoards(ctx, db.ListBoardsParams{ProjectID: b.ProjectID})
		if err != nil {
			return err
		}
		var target *db.ProjectBoard
		for i := range boards {
			if boards[i].ID != id {
				target = &boards[i]
				break
			}
		}
		if target == nil {
			return httpx.Invalid("项目至少要留一个看板")
		}
		ids, err := q.IssueIDsOnBoard(ctx, &id)
		if err != nil {
			return err
		}
		for _, issueID := range ids {
			r, err := q.GetIssue(ctx, issueID)
			if err != nil {
				return err
			}
			l, err := q.ListForStatus(ctx, db.ListForStatusParams{BoardID: target.ID, Status: &r.Issue.Status})
			if errors.Is(err, sql.ErrNoRows) {
				l, err = q.FirstList(ctx, target.ID)
			}
			if err != nil {
				return httpx.Invalid("看板“" + target.Name + "”里没有列表，放不下这些卡片")
			}
			sort, err := bottomOfList(ctx, q, l.ID, issueID)
			if err != nil {
				return err
			}
			if _, err := m.placeCard(ctx, q, r.Issue, l, sort); err != nil {
				return err
			}
		}
		return q.DeleteBoard(ctx, id)
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
		m.d.Bus.Publish("issue.updated", map[string]any{"projectId": projectID})
	}
	return err
}

func (m *Module) copyBoard(ctx context.Context, id int64, name string) (out api.Board, err error) {
	defer func() { m.d.Audit.Record(ctx, "board.copy", "", map[string]any{"id": id}, err) }()
	err = m.tx(ctx, func(q *db.Queries) error {
		b, err := q.GetBoard(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if strings.TrimSpace(name) == "" {
			name = b.Name + "（副本）"
		}
		if name, err = cleanName(name, "看板"); err != nil {
			return err
		}
		nb, err := m.createBoardTx(ctx, q, b.ProjectID, name, b.Icon, "empty")
		if err != nil {
			return err
		}
		lists, err := q.ListLists(ctx, db.ListListsParams{BoardID: b.ID})
		if err != nil {
			return err
		}
		for _, l := range lists {
			if _, err := q.CreateList(ctx, db.CreateListParams{BoardID: nb.ID, Name: l.Name, Position: l.Position,
				Status: l.Status, Color: l.Color, WipLimit: l.WipLimit, CreatedAt: m.now()}); err != nil {
				return err
			}
		}
		out, err = m.boardWithLists(ctx, q, nb, false)
		return err
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": out.ProjectId})
	}
	return out, err
}

// ---- lists ----

func (m *Module) createList(ctx context.Context, boardID int64, in api.CreateBoardList) (out api.BoardList, err error) {
	defer func() { m.d.Audit.Record(ctx, "list.create", in.Name, map[string]any{"boardId": boardID}, err) }()
	name, err := cleanName(in.Name, "列表")
	if err != nil {
		return out, err
	}
	var status *string
	if in.Status != nil {
		if !validStatus(string(*in.Status)) {
			return out, httpx.Invalid("状态不正确")
		}
		s := string(*in.Status)
		status = &s
	}
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		b, err := q.GetBoard(ctx, boardID)
		if err != nil {
			return notFound(err)
		}
		projectID = b.ProjectID
		pos, err := q.MaxListPosition(ctx, boardID)
		if err != nil {
			return err
		}
		l, err := q.CreateList(ctx, db.CreateListParams{BoardID: boardID, Name: name, Position: pos + sortGap,
			Status: status, CreatedAt: m.now()})
		if err != nil {
			return err
		}
		out = toList(l, 0)
		return nil
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
	}
	return out, err
}

func (m *Module) updateList(ctx context.Context, id int64, in api.UpdateBoardList, nulls map[string]bool) (out api.BoardList, err error) {
	defer func() { m.d.Audit.Record(ctx, "list.update", "", map[string]any{"id": id}, err) }()
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetList(ctx, id)
		if err != nil {
			return notFound(err)
		}
		projectID = r.ProjectID
		l := listOf(r)
		if in.Name != nil {
			if l.Name, err = cleanName(*in.Name, "列表"); err != nil {
				return err
			}
		}
		if nulls["status"] {
			l.Status = nil
		} else if in.Status != nil {
			s := string(*in.Status)
			if !validStatus(s) {
				return httpx.Invalid("状态不正确")
			}
			l.Status = &s
		}
		if in.Color != nil {
			l.Color = *in.Color
		}
		if in.WipLimit != nil {
			if *in.WipLimit < 0 || *in.WipLimit > 999 {
				return httpx.Invalid("上限在 0 到 999 之间")
			}
			l.WipLimit = int64(*in.WipLimit)
		}
		if in.Collapsed != nil {
			l.Collapsed = archivedFlag(*in.Collapsed)
		}
		if in.Archived != nil {
			if *in.Archived && l.ArchivedAt == nil {
				now := m.now()
				l.ArchivedAt = &now
			} else if !*in.Archived {
				l.ArchivedAt = nil
			}
		}
		if in.AfterId != nil {
			lists, err := q.ListLists(ctx, db.ListListsParams{BoardID: l.BoardID, Archived: 1})
			if err != nil {
				return err
			}
			ids, pos := make([]int64, len(lists)), make([]float64, len(lists))
			for i, x := range lists {
				ids[i], pos[i] = x.ID, x.Position
			}
			p, ok := positionAfter(*in.AfterId, ids, pos, l.ID)
			if !ok {
				for i, x := range lists {
					pos[i] = float64(i+1) * sortGap
					if err := q.SetListPosition(ctx, db.SetListPositionParams{Position: pos[i], ID: x.ID}); err != nil {
						return err
					}
				}
				if p, ok = positionAfter(*in.AfterId, ids, pos, l.ID); !ok {
					return httpx.Invalid("排序位置不正确")
				}
			}
			l.Position = p
		}
		nl, err := q.UpdateList(ctx, db.UpdateListParams{Name: l.Name, Position: l.Position, Status: l.Status,
			Color: l.Color, WipLimit: l.WipLimit, Collapsed: l.Collapsed, ArchivedAt: l.ArchivedAt, ID: l.ID})
		if err != nil {
			return err
		}
		n, err := q.CountOpenInList(ctx, &id)
		if err != nil {
			return err
		}
		out = toList(nl, int(n))
		return nil
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
	}
	return out, err
}

func (m *Module) deleteList(ctx context.Context, id int64) (err error) {
	defer func() { m.d.Audit.Record(ctx, "list.delete", "", map[string]any{"id": id}, err) }()
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetList(ctx, id)
		if err != nil {
			return notFound(err)
		}
		projectID = r.ProjectID
		n, err := q.CountAllInList(ctx, &id)
		if err != nil {
			return err
		}
		if n > 0 {
			return httpx.NewError(409, "conflict", "列表里还有卡片（包括归档的），先移走再删")
		}
		return q.DeleteList(ctx, id)
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
	}
	return err
}

func (m *Module) archiveListCards(ctx context.Context, id int64) (n int64, err error) {
	defer func() { m.d.Audit.Record(ctx, "list.archive_cards", "", map[string]any{"id": id, "count": n}, err) }()
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		r, err := q.GetList(ctx, id)
		if err != nil {
			return notFound(err)
		}
		projectID = r.ProjectID
		now := m.now()
		listID := id
		n, err = q.ArchiveListCards(ctx, db.ArchiveListCardsParams{ArchivedAt: &now, UpdatedAt: now, ListID: &listID})
		return err
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
		m.d.Bus.Publish("issue.updated", map[string]any{"projectId": projectID})
	}
	return n, err
}

func (m *Module) moveListCards(ctx context.Context, fromID, toID int64) (n int, err error) {
	defer func() {
		m.d.Audit.Record(ctx, "list.move_cards", "", map[string]any{"from": fromID, "to": toID, "count": n}, err)
	}()
	if fromID == toID {
		return 0, httpx.Invalid("目标列表和原列表一样")
	}
	var projectID int64
	err = m.tx(ctx, func(q *db.Queries) error {
		from, err := q.GetList(ctx, fromID)
		if err != nil {
			return notFound(err)
		}
		to, err := q.GetList(ctx, toID)
		if err != nil {
			return notFound(err)
		}
		if to.ArchivedAt != nil {
			return httpx.Invalid("目标列表已归档")
		}
		projectID = from.ProjectID
		ids, err := q.ListColumn(ctx, db.ListColumnParams{ListID: &fromID, ID: 0})
		if err != nil {
			return err
		}
		for _, issueID := range ids {
			r, err := q.GetIssue(ctx, issueID)
			if err != nil {
				return err
			}
			sort, err := bottomOfList(ctx, q, toID, issueID)
			if err != nil {
				return err
			}
			if _, err := m.placeCard(ctx, q, r.Issue, listOf(to), sort); err != nil {
				return err
			}
			if err := activity(ctx, q, issueID, "moved", map[string]any{"toList": to.Name, "listId": toID}, m.now()); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err == nil {
		m.d.Bus.Publish("board.changed", map[string]any{"projectId": projectID})
		m.d.Bus.Publish("issue.updated", map[string]any{"projectId": projectID})
	}
	return n, err
}

// ---- cards ----

func (m *Module) setArchived(ctx context.Context, key string, archive bool) (out api.Issue, err error) {
	action := "issue.restore"
	if archive {
		action = "issue.archive"
	}
	defer func() { m.d.Audit.Record(ctx, action, key, nil, err) }()
	var id int64
	err = m.tx(ctx, func(q *db.Queries) error {
		row, err := m.findIssue(ctx, q, key)
		if err != nil {
			return err
		}
		i := row.Issue
		id = i.ID
		now := m.now()
		if archive {
			if i.ArchivedAt != nil {
				return nil
			}
			if err := q.SetIssueArchived(ctx, db.SetIssueArchivedParams{ArchivedAt: &now, UpdatedAt: now, ID: i.ID}); err != nil {
				return err
			}
			return activity(ctx, q, i.ID, "archived", nil, now)
		}
		if i.ArchivedAt == nil {
			return nil
		}
		// Back to the end of its list; if that list is gone or archived, the
		// first list of the board (or of the project).
		var target db.BoardList
		ok := false
		if i.ListID != nil {
			if l, err := q.GetList(ctx, *i.ListID); err == nil && l.ArchivedAt == nil {
				target, ok = listOf(l), true
			}
		}
		if !ok {
			t, err := m.placeFor(ctx, q, i.ProjectID, i.BoardID, nil, i.Status)
			if err != nil {
				return err
			}
			target = t
		}
		if err := q.SetIssueArchived(ctx, db.SetIssueArchivedParams{ArchivedAt: nil, UpdatedAt: now, ID: i.ID}); err != nil {
			return err
		}
		sort, err := bottomOfList(ctx, q, target.ID, i.ID)
		if err != nil {
			return err
		}
		i.ArchivedAt = nil
		if _, err := m.placeCard(ctx, q, i, target, sort); err != nil {
			return err
		}
		return activity(ctx, q, i.ID, "restored", nil, now)
	})
	if err != nil {
		return out, err
	}
	out, err = m.issueByID(ctx, id)
	if err == nil {
		m.d.Bus.Publish("issue.updated", out)
	}
	return out, err
}

func (m *Module) copyIssue(ctx context.Context, key string) (out api.Issue, err error) {
	defer func() { m.d.Audit.Record(ctx, "issue.copy", key, map[string]any{"copy": out.Key}, err) }()
	src, err := m.getIssue(ctx, key)
	if err != nil {
		return out, err
	}
	labelIDs := make([]int64, 0, len(src.Labels))
	for _, l := range src.Labels {
		labelIDs = append(labelIDs, l.Id)
	}
	in := issueInput{Title: src.Title + "（副本）", Description: src.Description, Status: string(src.Status),
		Priority: src.Priority, DueAt: src.DueAt, MilestoneID: src.MilestoneId, LabelIDs: labelIDs, ListID: src.ListId}
	if src.DueRemind != nil {
		in.DueRemind = string(*src.DueRemind)
	}
	out, err = m.insertIssue(ctx, src.ProjectId, in)
	if err != nil {
		return out, err
	}
	err = m.tx(ctx, func(q *db.Queries) error {
		// Right under the original.
		if src.ListId != nil {
			after := src.Key
			sort, err := m.slotIn(ctx, q, *src.ListId, out.Id, &after, nil)
			if err != nil {
				return err
			}
			if err := q.SetSortOrder(ctx, db.SetSortOrderParams{SortOrder: sort, ID: out.Id}); err != nil {
				return err
			}
		}
		if err := q.CopyChecklists(ctx, db.CopyChecklistsParams{ToIssue: out.Id, FromIssue: src.Id}); err != nil {
			return err
		}
		if err := q.CopyChecklistItems(ctx, db.CopyChecklistItemsParams{FromIssue: src.Id, ToIssue: out.Id}); err != nil {
			return err
		}
		return activity(ctx, q, out.Id, "copied", map[string]any{"from": src.Key}, m.now())
	})
	if err != nil {
		return out, err
	}
	if err = m.claimFiles(ctx, "issue", out.Id, out.Description); err != nil {
		return out, err
	}
	out, err = m.issueByID(ctx, out.Id)
	if err == nil {
		m.d.Bus.Publish("issue.created", out)
	}
	return out, err
}

func (m *Module) setMembers(ctx context.Context, key string, members []api.IssueMember) (out api.Issue, err error) {
	defer func() { m.d.Audit.Record(ctx, "issue.members", key, nil, err) }()
	if len(members) > 20 {
		return out, httpx.Invalid("成员最多 20 个")
	}
	var id int64
	err = m.tx(ctx, func(q *db.Queries) error {
		row, err := m.findIssue(ctx, q, key)
		if err != nil {
			return err
		}
		id = row.Issue.ID
		if err := q.ClearIssueMembers(ctx, id); err != nil {
			return err
		}
		names := make([]string, 0, len(members))
		for _, mem := range members {
			switch mem.Kind {
			case "me":
				mem.Id = ""
			case "agent":
				if strings.TrimSpace(mem.Id) == "" {
					return httpx.Invalid("Agent 成员要有 id")
				}
			default:
				return httpx.Invalid("成员类型不正确")
			}
			if err := q.AddIssueMember(ctx, db.AddIssueMemberParams{IssueID: id, MemberKind: string(mem.Kind), MemberID: mem.Id}); err != nil {
				return err
			}
			names = append(names, string(mem.Kind)+":"+mem.Id)
		}
		return activity(ctx, q, id, "members", map[string]any{"members": names}, m.now())
	})
	if err != nil {
		return out, err
	}
	out, err = m.issueByID(ctx, id)
	if err == nil {
		m.d.Bus.Publish("issue.updated", out)
	}
	return out, err
}

func (m *Module) listActivity(ctx context.Context, key string) ([]api.IssueActivity, error) {
	row, err := m.findIssue(ctx, m.q, key)
	if err != nil {
		return nil, err
	}
	rows, err := m.q.ListActivity(ctx, row.Issue.ID)
	if err != nil {
		return nil, err
	}
	out := make([]api.IssueActivity, 0, len(rows))
	for _, r := range rows {
		data := map[string]any{}
		_ = json.Unmarshal([]byte(r.Data), &data)
		out = append(out, api.IssueActivity{Id: r.ID, At: r.At, Actor: r.Actor, Kind: r.Kind, Data: data})
	}
	return out, nil
}

func (m *Module) boardArchive(ctx context.Context, boardID int64) (issues []api.Issue, lists []api.BoardList, err error) {
	if _, err := m.q.GetBoard(ctx, boardID); err != nil {
		return nil, nil, notFound(err)
	}
	rows, err := m.q.ListArchivedIssues(ctx, &boardID)
	if err != nil {
		return nil, nil, err
	}
	list := make([]issueRow, len(rows))
	for i, r := range rows {
		list[i] = issueRow{r.Issue, r.ProjectKey}
	}
	issues, err = toIssues(ctx, m.q, list)
	if err != nil {
		return nil, nil, err
	}
	all, err := m.q.ListLists(ctx, db.ListListsParams{BoardID: boardID, Archived: 1})
	if err != nil {
		return nil, nil, err
	}
	lists = []api.BoardList{}
	for _, l := range all {
		if l.ArchivedAt != nil {
			lists = append(lists, toList(l, 0))
		}
	}
	return issues, lists, nil
}
