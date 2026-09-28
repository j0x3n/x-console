-- +goose Up
CREATE TABLE calendars_writable (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('ics', 'caldav', 'local')),
    url TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    secret TEXT NOT NULL DEFAULT '',
    color TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    last_synced_at DATETIME,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL
);
INSERT INTO calendars_writable SELECT * FROM calendars;
CREATE TABLE calendar_events_writable (
    id INTEGER PRIMARY KEY,
    calendar_id INTEGER NOT NULL REFERENCES calendars_writable (id) ON DELETE CASCADE,
    uid TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    starts_at DATETIME NOT NULL,
    ends_at DATETIME NOT NULL,
    all_day INTEGER NOT NULL DEFAULT 0,
    tzid TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    rrule TEXT NOT NULL DEFAULT '',
    rdates TEXT NOT NULL DEFAULT '[]',
    exdates TEXT NOT NULL DEFAULT '[]',
    recurrence_id DATETIME,
    href TEXT NOT NULL DEFAULT '',
    etag TEXT NOT NULL DEFAULT ''
);
INSERT INTO calendar_events_writable (id, calendar_id, uid, title, starts_at, ends_at, all_day,
    tzid, location, description, rrule, rdates, exdates, recurrence_id)
SELECT id, calendar_id, uid, title, starts_at, ends_at, all_day,
    tzid, location, description, rrule, rdates, exdates, recurrence_id FROM calendar_events;
DROP TABLE calendar_events;
DROP TABLE calendars;
ALTER TABLE calendars_writable RENAME TO calendars;
ALTER TABLE calendar_events_writable RENAME TO calendar_events;
CREATE INDEX calendar_events_calendar ON calendar_events (calendar_id);
CREATE INDEX calendar_events_start ON calendar_events (starts_at);

-- +goose Down
-- Removing these fields would lose writeback state, so rollbacks are handled by a new forward migration.
SELECT 1;
