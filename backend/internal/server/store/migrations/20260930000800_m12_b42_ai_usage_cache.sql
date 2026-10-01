-- B42 AI 用量：缓存命中、缓存写入、思考 token、来源、关联对象、成功或失败。
-- input_tokens 统一记全部输入，包括命中缓存和写入缓存的部分。

-- +goose Up
ALTER TABLE ai_usage ADD COLUMN cached_input_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_usage ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_usage ADD COLUMN reasoning_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE ai_usage ADD COLUMN source TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_usage ADD COLUMN ref TEXT NOT NULL DEFAULT '';
ALTER TABLE ai_usage ADD COLUMN status TEXT NOT NULL DEFAULT 'ok';
ALTER TABLE ai_usage ADD COLUMN error TEXT NOT NULL DEFAULT '';
-- 费用是否按原价估算了缓存部分（模型没有缓存价时为 1）
ALTER TABLE ai_usage ADD COLUMN cost_estimated INTEGER NOT NULL DEFAULT 0;
CREATE INDEX ai_usage_source ON ai_usage(source, created_at);

-- +goose Down
DROP INDEX ai_usage_source;
ALTER TABLE ai_usage DROP COLUMN cost_estimated;
ALTER TABLE ai_usage DROP COLUMN error;
ALTER TABLE ai_usage DROP COLUMN status;
ALTER TABLE ai_usage DROP COLUMN ref;
ALTER TABLE ai_usage DROP COLUMN source;
ALTER TABLE ai_usage DROP COLUMN reasoning_tokens;
ALTER TABLE ai_usage DROP COLUMN cache_write_tokens;
ALTER TABLE ai_usage DROP COLUMN cached_input_tokens;
