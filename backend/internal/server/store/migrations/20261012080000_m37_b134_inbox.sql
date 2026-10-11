-- +goose Up
-- B134: 随手记收集箱。
-- status: open 没处理，filed 已归档，dismissed 已忽略。
-- suggest_state: off 没生成，pending 生成中，ready 已有，failed 失败。
-- suggestion 是 JSON（kind、title、body、at、url、reason）。
-- filed_kind、filed_ref 记归档时建出的对象。
CREATE TABLE inbox_items (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 text TEXT NOT NULL,
 source TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'filed', 'dismissed')),
 suggest_state TEXT NOT NULL DEFAULT 'off' CHECK (suggest_state IN ('off', 'pending', 'ready', 'failed')),
 suggestion TEXT NOT NULL DEFAULT '',
 filed_kind TEXT NOT NULL DEFAULT '',
 filed_ref TEXT NOT NULL DEFAULT '',
 created_at DATETIME NOT NULL,
 updated_at DATETIME NOT NULL,
 closed_at DATETIME
);
CREATE INDEX inbox_items_status ON inbox_items(status, created_at);

-- +goose Down
DROP TABLE inbox_items;
