-- B129 AI 编码投入产出对账：记下编码任务的 PR 有没有合并。
-- 编码任务自己没有这个记录。outcome 是 merged 或 closed（PR 关闭没合并）。
-- task_id 不加外键：对账是只读汇总，任务被删后留着这行也没有影响。

-- +goose Up
CREATE TABLE codingroi_outcomes (
    task_id INTEGER  PRIMARY KEY,
    outcome TEXT     NOT NULL CHECK (outcome IN ('merged', 'closed')),
    at      DATETIME NOT NULL,
    source  TEXT     NOT NULL DEFAULT '', -- webhook 或 github
    pr_url  TEXT     NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE codingroi_outcomes;
