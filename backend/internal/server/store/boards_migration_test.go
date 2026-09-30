package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

// B46：旧数据迁移后，每个项目一个看板、6 个列表，Issue 按状态放进对应列表。
func TestBoardsMigrationPlacesExistingIssues(t *testing.T) {
	ctx := context.Background()
	database, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)&_time_format=sqlite")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	provider, err := goose.NewProvider(goose.DialectSQLite3, database, mustSub(migrations, "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.UpTo(ctx, 20260930000800); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO projects (id, key, name, created_at, updated_at) VALUES
		(1, 'XC', 'X Console', '2026-09-01', '2026-09-01'), (2, 'BL', '博客', '2026-09-01', '2026-09-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO issues (project_id, number, title, status, created_at, updated_at) VALUES
		(1, 1, 'a', 'todo', '2026-09-01', '2026-09-01'),
		(1, 2, 'b', 'done', '2026-09-01', '2026-09-01'),
		(1, 3, 'c', 'in_review', '2026-09-01', '2026-09-01'),
		(2, 1, 'd', 'backlog', '2026-09-01', '2026-09-01')`); err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var boards, lists int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM project_boards`).Scan(&boards); err != nil || boards != 2 {
		t.Fatalf("boards: %d %v", boards, err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM board_lists`).Scan(&lists); err != nil || lists != 12 {
		t.Fatalf("lists: %d %v", lists, err)
	}
	var wrong int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM issues i
		LEFT JOIN board_lists l ON l.id = i.list_id
		LEFT JOIN project_boards b ON b.id = i.board_id
		WHERE l.id IS NULL OR b.id IS NULL OR l.board_id <> b.id OR b.project_id <> i.project_id OR l.status <> i.status`).Scan(&wrong); err != nil || wrong != 0 {
		t.Fatalf("misplaced issues: %d %v", wrong, err)
	}
	var name string
	if err := database.QueryRowContext(ctx, `SELECT l.name FROM issues i JOIN board_lists l ON l.id = i.list_id WHERE i.title = 'c'`).Scan(&name); err != nil || name != "待审核" {
		t.Fatalf("list name: %q %v", name, err)
	}
}
