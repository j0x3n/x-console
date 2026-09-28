package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestWritableCalendarMigrationPreservesEvents(t *testing.T) {
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
	if _, err = provider.UpTo(ctx, 20260928000700); err != nil {
		t.Fatal(err)
	}
	_, err = database.ExecContext(ctx, `INSERT INTO calendars (id, name, kind, url, created_at)
		VALUES (42, '已有日历', 'ics', 'https://example.invalid/events.ics', '2026-09-28')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.ExecContext(ctx, `INSERT INTO calendar_events
		(id, calendar_id, uid, title, starts_at, ends_at) VALUES
		(99, 42, 'existing', '已有日程', '2026-10-20 10:00:00', '2026-10-20 11:00:00')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var calendarID int64
	var title, href, etag string
	if err = database.QueryRowContext(ctx, `SELECT calendar_id, title, href, etag FROM calendar_events WHERE id = 99`).Scan(
		&calendarID, &title, &href, &etag); err != nil {
		t.Fatal(err)
	}
	if calendarID != 42 || title != "已有日程" || href != "" || etag != "" {
		t.Fatalf("migrated row = %d %q %q %q", calendarID, title, href, etag)
	}
	if _, err = database.ExecContext(ctx, `INSERT INTO calendars (name, kind, url, created_at)
		VALUES ('本地', 'local', '', '2026-09-28')`); err != nil {
		t.Fatal(err)
	}
	var violation int
	if err = database.QueryRowContext(ctx, `SELECT count(*) FROM pragma_foreign_key_check`).Scan(&violation); err != nil {
		t.Fatal(err)
	}
	if violation != 0 {
		t.Fatalf("foreign key violations: %d", violation)
	}
}
