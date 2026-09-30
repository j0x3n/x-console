-- +goose Up
CREATE TABLE ai_providers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    api_key_enc TEXT,
    models_refreshed_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL
);
CREATE TABLE ai_provider_models (
    provider_id INTEGER NOT NULL REFERENCES ai_providers(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    PRIMARY KEY (provider_id, model_id)
);
CREATE TABLE ai_model_specs (
    provider_id INTEGER NOT NULL REFERENCES ai_providers(id) ON DELETE CASCADE,
    model_id TEXT NOT NULL,
    context_window INTEGER,
    tool_call INTEGER,
    reasoning INTEGER,
    PRIMARY KEY (provider_id, model_id)
);
CREATE TABLE ai_usage (
    id INTEGER PRIMARY KEY,
    provider_id INTEGER,
    provider_name TEXT NOT NULL,
    model TEXT NOT NULL,
    purpose TEXT NOT NULL CHECK(purpose IN ('fast','agent')),
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    cost REAL,
    created_at DATETIME NOT NULL
);
CREATE INDEX ai_usage_created ON ai_usage(created_at);

-- +goose Down
DROP TABLE ai_usage;
DROP TABLE ai_model_specs;
DROP TABLE ai_provider_models;
DROP TABLE ai_providers;
