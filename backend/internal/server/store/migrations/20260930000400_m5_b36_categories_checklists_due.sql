-- +goose Up
CREATE TABLE project_categories (
  id INTEGER PRIMARY KEY,
  project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  parent_id INTEGER REFERENCES project_categories(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  position REAL NOT NULL DEFAULT 0
);
CREATE INDEX project_categories_project ON project_categories(project_id, parent_id, position);

ALTER TABLE issues ADD COLUMN category_id INTEGER REFERENCES project_categories(id) ON DELETE SET NULL;
ALTER TABLE issues ADD COLUMN due_at TEXT;
ALTER TABLE issues ADD COLUMN due_remind TEXT NOT NULL DEFAULT 'at_due';
ALTER TABLE issues ADD COLUMN due_notified_at TEXT;
CREATE INDEX issues_due_at ON issues(due_at) WHERE due_at IS NOT NULL;

CREATE TABLE issue_checklists (
  id INTEGER PRIMARY KEY,
  issue_id INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  position REAL NOT NULL DEFAULT 0
);
CREATE TABLE issue_checklist_items (
  id INTEGER PRIMARY KEY,
  checklist_id INTEGER NOT NULL REFERENCES issue_checklists(id) ON DELETE CASCADE,
  text TEXT NOT NULL,
  done INTEGER NOT NULL DEFAULT 0,
  position REAL NOT NULL DEFAULT 0,
  done_at TEXT
);
CREATE INDEX issue_checklist_items_list ON issue_checklist_items(checklist_id, position);

-- +goose Down
