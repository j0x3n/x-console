package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestSubscriptionCategoriesMigrationConvertsOldRows(t *testing.T) {
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
	if _, err = provider.UpTo(ctx, 20260928000900); err != nil {
		t.Fatal(err)
	}
	for _, row := range []string{
		"(1, '月付', 'server', 'monthly', 0)",
		"(2, '年付', 'domain', 'yearly', 0)",
		"(3, '两周', 'saas', 'custom_days', 14)",
	} {
		_, err = database.ExecContext(ctx, `INSERT INTO subscriptions (id, name, category, cycle, cycle_days, next_renewal, created_at, updated_at)
			SELECT `+row[1:len(row)-1]+`, '2026-10-01', '2026-09-28', '2026-09-28'`)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	want := map[int64]struct {
		count   int
		unit    string
		builtin string
	}{1: {1, "month", "server"}, 2: {1, "year", "domain"}, 3: {14, "day", "saas"}}
	for id, w := range want {
		var count int
		var unit, builtin string
		err = database.QueryRowContext(ctx, `SELECT s.cycle_count, s.cycle_unit, c.builtin FROM subscriptions s
			JOIN subscription_categories c ON c.id = s.category_id WHERE s.id = ?`, id).Scan(&count, &unit, &builtin)
		if err != nil {
			t.Fatalf("subscription %d: %v", id, err)
		}
		if count != w.count || unit != w.unit || builtin != w.builtin {
			t.Errorf("subscription %d: got %d %s %s, want %+v", id, count, unit, builtin, w)
		}
	}
}
