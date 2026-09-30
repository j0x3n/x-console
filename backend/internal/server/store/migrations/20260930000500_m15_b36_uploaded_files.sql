-- +goose Up
CREATE TABLE uploaded_files (
  id INTEGER PRIMARY KEY,
  scope TEXT NOT NULL,
  name TEXT NOT NULL,
  mime TEXT NOT NULL,
  size INTEGER NOT NULL,
  sha256 TEXT NOT NULL,
  owner_kind TEXT,
  owner_id INTEGER,
  created_at DATETIME NOT NULL
);
CREATE INDEX uploaded_files_owner ON uploaded_files(owner_kind,owner_id);
CREATE INDEX uploaded_files_unclaimed ON uploaded_files(created_at) WHERE owner_kind IS NULL;

-- +goose Down
