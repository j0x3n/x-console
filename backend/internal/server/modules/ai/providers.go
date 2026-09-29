package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai/api"
)

type providerRow struct {
	api.AiProvider
	key sql.NullString
}

const providerColumns = `SELECT p.id,p.name,p.base_url,p.api_key_enc,p.models_refreshed_at,p.last_error,p.created_at,
 (SELECT count(*) FROM ai_provider_models WHERE provider_id=p.id) FROM ai_providers p`

func scanProvider(s interface{ Scan(...any) error }) (providerRow, error) {
	var p providerRow
	var refreshed sql.NullTime
	var lastError sql.NullString
	err := s.Scan(&p.Id, &p.Name, &p.BaseUrl, &p.key, &refreshed, &lastError, &p.CreatedAt, &p.ModelCount)
	if err != nil {
		return p, err
	}
	p.HasApiKey = p.key.Valid && p.key.String != ""
	if refreshed.Valid {
		p.ModelsRefreshedAt = &refreshed.Time
	}
	if lastError.Valid {
		p.LastError = &lastError.String
	}
	return p, nil
}

func (m *Module) provider(ctx context.Context, id int64) (providerRow, error) {
	p, err := scanProvider(m.d.DB.QueryRowContext(ctx, providerColumns+` WHERE p.id=?`, id))
	return p, notFound(err)
}

func providerName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if n := len([]rune(value)); n < 1 || n > 40 {
		return "", httpx.Invalid("供应商名称长度应为 1 到 40 字")
	}
	return value, nil
}

func providerURL(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", httpx.Invalid("Base URL 应为 http 或 https 地址")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (m *Module) ListAiProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := m.d.DB.QueryContext(r.Context(), providerColumns+` ORDER BY p.created_at,p.id`)
	if m.fail(w, r, err) {
		return
	}
	defer rows.Close()
	out := []api.AiProvider{}
	for rows.Next() {
		p, err := scanProvider(rows)
		if m.fail(w, r, err) {
			return
		}
		out = append(out, p.AiProvider)
	}
	if m.fail(w, r, rows.Err()) {
		return
	}
	httpx.JSON(w, 200, out)
}

func (m *Module) CreateAiProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	var body api.AiProviderInput
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	name, err := providerName(body.Name)
	if m.fail(w, r, err) {
		return
	}
	baseURL, err := providerURL(body.BaseUrl)
	if m.fail(w, r, err) {
		return
	}
	var sealed *string
	if body.ApiKey != nil && *body.ApiKey != "" {
		value, err := m.d.Secrets.Seal(*body.ApiKey)
		if m.fail(w, r, err) {
			return
		}
		sealed = &value
	}
	result, err := m.d.DB.ExecContext(ctx, `INSERT INTO ai_providers(name,base_url,api_key_enc,created_at) VALUES(?,?,?,?)`, name, baseURL, sealed, time.Now().UTC())
	if m.fail(w, r, err) {
		return
	}
	id, err := result.LastInsertId()
	if m.fail(w, r, err) {
		return
	}
	p, err := m.provider(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(ctx, "ai.provider.create", strconv.FormatInt(id, 10), map[string]any{"name": name, "baseUrl": baseURL}, nil)
	m.d.Bus.Publish("ai.provider_changed", p.AiProvider)
	go func() {
		background, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = m.refreshModels(background, id)
	}()
	httpx.JSON(w, 201, p.AiProvider)
}

func (m *Module) UpdateAiProvider(w http.ResponseWriter, r *http.Request, id api.ProviderId) {
	ctx := r.Context()
	if m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	p, err := m.provider(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	var body api.AiProviderPatch
	if m.fail(w, r, httpx.Decode(r, &body)) {
		return
	}
	name, baseURL := p.Name, p.BaseUrl
	if body.Name != nil {
		name, err = providerName(*body.Name)
	}
	if m.fail(w, r, err) {
		return
	}
	if body.BaseUrl != nil {
		baseURL, err = providerURL(*body.BaseUrl)
	}
	if m.fail(w, r, err) {
		return
	}
	var sealed any = p.key
	if body.ApiKey != nil {
		sealed = nil
		if *body.ApiKey != "" {
			sealed, err = m.d.Secrets.Seal(*body.ApiKey)
			if m.fail(w, r, err) {
				return
			}
		}
	}
	_, err = m.d.DB.ExecContext(ctx, `UPDATE ai_providers SET name=?,base_url=?,api_key_enc=? WHERE id=?`, name, baseURL, sealed, id)
	if m.fail(w, r, err) {
		return
	}
	p, err = m.provider(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	m.d.Audit.Record(ctx, "ai.provider.update", strconv.FormatInt(id, 10), map[string]any{"name": name, "baseUrl": baseURL}, nil)
	m.d.Bus.Publish("ai.provider_changed", p.AiProvider)
	httpx.JSON(w, 200, p.AiProvider)
}

func (m *Module) DeleteAiProvider(w http.ResponseWriter, r *http.Request, id api.ProviderId) {
	ctx := r.Context()
	if m.fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	p, err := m.provider(ctx, id)
	if m.fail(w, r, err) {
		return
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if m.fail(w, r, err) {
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM ai_providers WHERE id=?`, id); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	for _, key := range []string{"ai.fast_model", "ai.agent_model"} {
		if _, err = tx.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND json_extract(value,'$.providerId')=?`, key, id); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	if m.fail(w, r, tx.Commit()) {
		return
	}
	m.d.Audit.Record(ctx, "ai.provider.delete", strconv.FormatInt(id, 10), map[string]any{"name": p.Name, "baseUrl": p.BaseUrl}, nil)
	m.d.Bus.Publish("ai.provider_changed", map[string]any{"id": id})
	httpx.NoContent(w)
}

func (m *Module) TestAiProvider(w http.ResponseWriter, r *http.Request, id api.ProviderId) {
	p, err := m.provider(r.Context(), id)
	if m.fail(w, r, err) {
		return
	}
	start := time.Now()
	models, err := m.fetchModels(r.Context(), p)
	result := api.AiProviderTest{Ok: err == nil}
	if err == nil {
		count, ms := len(models), int(time.Since(start).Milliseconds())
		result.Message = fmt.Sprintf("找到 %d 个模型", count)
		result.ModelCount, result.LatencyMs = &count, &ms
	} else {
		result.Message = err.Error()
	}
	var last any
	if err != nil {
		last = result.Message
	}
	if m.fail(w, r, setProviderError(r.Context(), m.d.DB, id, last)) {
		return
	}
	httpx.JSON(w, 200, result)
}

func setProviderError(ctx context.Context, db *sql.DB, id int64, value any) error {
	_, err := db.ExecContext(ctx, `UPDATE ai_providers SET last_error=? WHERE id=?`, value, id)
	return err
}

func (m *Module) fetchModels(ctx context.Context, p providerRow) ([]string, error) {
	key := "local-model"
	if p.key.Valid && p.key.String != "" {
		var err error
		key, err = m.d.Secrets.Open(p.key.String)
		if err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseUrl+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, errors.New("API Key 不对")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("接口返回 %d", resp.StatusCode)
	}
	var data struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&data); err != nil {
		return nil, fmt.Errorf("模型列表解析失败: %w", err)
	}
	models := make([]string, 0, len(data.Data))
	seen := map[string]bool{}
	for _, model := range data.Data {
		if model.ID != "" && !seen[model.ID] {
			seen[model.ID] = true
			models = append(models, model.ID)
		}
	}
	return models, nil
}

func (m *Module) refreshModels(ctx context.Context, id int64) ([]string, error) {
	p, err := m.provider(ctx, id)
	if err != nil {
		return nil, err
	}
	models, err := m.fetchModels(ctx, p)
	if err != nil {
		_ = setProviderError(ctx, m.d.DB, id, err.Error())
		return nil, err
	}
	tx, err := m.d.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM ai_provider_models WHERE provider_id=?`, id); err != nil {
		return nil, err
	}
	for _, model := range models {
		if _, err = tx.ExecContext(ctx, `INSERT INTO ai_provider_models(provider_id,model_id) VALUES(?,?)`, id, model); err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE ai_providers SET models_refreshed_at=?,last_error=NULL WHERE id=?`, time.Now().UTC(), id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	m.d.Bus.Publish("ai.provider_changed", map[string]any{"id": id})
	return models, nil
}
