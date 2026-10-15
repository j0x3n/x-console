-- +goose Up
-- api_style 的 CHECK 要加上 jev。SQLite 改不了 CHECK，所以换表。子表引用这张表，先复制再删。
CREATE TABLE ai_providers_new (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key_enc TEXT,
    models_refreshed_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL,
    api_style TEXT NOT NULL DEFAULT 'chat' CHECK (api_style IN ('chat', 'responses', 'jev'))
);
INSERT INTO ai_providers_new (id, name, base_url, api_key_enc, models_refreshed_at, last_error, created_at, api_style)
SELECT id, name, base_url, api_key_enc, models_refreshed_at, last_error, created_at, api_style FROM ai_providers;
CREATE TABLE ai_provider_models_new (
    provider_id INTEGER NOT NULL REFERENCES ai_providers_new(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    PRIMARY KEY (provider_id, model_id)
);
INSERT INTO ai_provider_models_new (provider_id, model_id)
SELECT provider_id, model_id FROM ai_provider_models;
CREATE TABLE ai_model_specs_new (
    provider_id INTEGER NOT NULL REFERENCES ai_providers_new(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    context_window INTEGER,
    tool_call INTEGER,
    reasoning INTEGER,
    PRIMARY KEY (provider_id, model_id)
);
INSERT INTO ai_model_specs_new (provider_id, model_id, context_window, tool_call, reasoning)
SELECT provider_id, model_id, context_window, tool_call, reasoning FROM ai_model_specs;
DROP TABLE ai_model_specs;
DROP TABLE ai_provider_models;
DROP TABLE ai_providers;
ALTER TABLE ai_providers_new RENAME TO ai_providers;
ALTER TABLE ai_provider_models_new RENAME TO ai_provider_models;
ALTER TABLE ai_model_specs_new RENAME TO ai_model_specs;

-- +goose Down
-- 只加不删。
SELECT 1;
