-- +goose Up
CREATE TABLE monitor_icons (
    monitor_id INTEGER  PRIMARY KEY REFERENCES monitors (id) ON DELETE CASCADE,
    mime       TEXT     NOT NULL,
    data       BLOB     NOT NULL,
    fetched_at DATETIME NOT NULL
);

-- +goose Down
DROP TABLE monitor_icons;
