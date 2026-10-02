package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/files"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage/api"
)

// status builds the answer of GET /storage.
func (m *Module) status(ctx context.Context) (api.StorageStatus, error) {
	m.mu.Lock()
	backend, cache, last := m.backend, m.cache, m.last
	running := m.move
	m.mu.Unlock()

	out := api.StorageStatus{Backend: backend, LocalPath: m.d.Config.FilesDir(), Migration: last, Usage: []api.ModuleUsage{}}
	if running != nil {
		out.Migration = running.snapshot()
	}
	var err error
	if out.CacheLimitBytes, err = m.cacheLimit(ctx); err != nil {
		return out, err
	}
	s, secret, err := m.s3Settings(ctx)
	if err != nil {
		return out, err
	}
	if s.Endpoint != "" || s.Bucket != "" || secret != "" {
		out.S3 = &api.StorageS3{Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket, Prefix: s.Prefix,
			AccessKeyId: s.AccessKeyID, HasSecret: secret != "", PathStyle: s.PathStyle}
	}
	if cache != nil {
		if _, out.CacheBytes, err = cache.Usage(ctx); err != nil {
			return out, err
		}
	}
	out.Usage, err = m.moduleUsage(ctx)
	return out, err
}

func (m *Module) moduleUsage(ctx context.Context) ([]api.ModuleUsage, error) {
	now := time.Now()
	if list, ok := m.usage.get(now); ok {
		return list, nil
	}
	list, err := countUsage(ctx, m.d.Files.Store())
	if err != nil {
		return nil, err
	}
	m.usage.put(now, list)
	return list, nil
}

// GetStorage is GET /storage.
func (m *Module) GetStorage(w http.ResponseWriter, r *http.Request) {
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// merged applies the fields of an input on top of the saved S3 setup.
func (m *Module) merged(ctx context.Context, in api.StorageS3Input) (s3Settings, string, error) {
	s, secret, err := m.s3Settings(ctx)
	if err != nil {
		return s, "", err
	}
	if in.Endpoint != nil {
		s.Endpoint = *in.Endpoint
	}
	if in.Region != nil {
		s.Region = *in.Region
	}
	if in.Bucket != nil {
		s.Bucket = *in.Bucket
	}
	if in.Prefix != nil {
		s.Prefix = *in.Prefix
	}
	if in.AccessKeyId != nil {
		s.AccessKeyID = *in.AccessKeyId
	}
	if in.PathStyle != nil {
		s.PathStyle = *in.PathStyle
	}
	if in.SecretAccessKey != nil && *in.SecretAccessKey != "" {
		secret = *in.SecretAccessKey
	}
	return s, secret, nil
}

// PutStorageS3 is PUT /storage/s3. It needs elevation because a new endpoint
// receives the site's files and the secret key.
func (m *Module) PutStorageS3(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireElevated(ctx); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var in api.StorageS3Input
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	err := m.saveS3(ctx, in)
	m.d.Audit.Record(ctx, "storage.s3.configure", "", map[string]any{"secretChanged": in.SecretAccessKey != nil && *in.SecretAccessKey != ""}, err)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.respondStatus(w, r)
}

func (m *Module) respondStatus(w http.ResponseWriter, r *http.Request) {
	out, err := m.status(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (m *Module) saveS3(ctx context.Context, in api.StorageS3Input) error {
	s, secret, err := m.merged(ctx, in)
	if err != nil {
		return err
	}
	if s.Endpoint != "" || s.Bucket != "" {
		if err := files.ValidateS3(s.config(secret)); err != nil {
			return httpx.Invalid(err.Error())
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.move != nil || m.cleaning {
		return httpx.NewError(http.StatusConflict, "conflict", "正在搬迁文件，完成后再改 S3 设置")
	}
	active := m.backend == api.S3
	if active {
		// The site is using this bucket right now: prove the new setup works
		// before it replaces the old one.
		if !s.complete(secret) {
			return httpx.Invalid("正在使用 S3，配置要填完整")
		}
		remote, err := files.NewS3(s.config(secret))
		if err != nil {
			return httpx.Invalid(err.Error())
		}
		if err := remote.Check(ctx); err != nil {
			return httpx.Invalid("连接测试没通过：" + err.Error())
		}
	}
	if err := m.d.Settings.Set(ctx, keyS3, s); err != nil {
		return err
	}
	if in.SecretAccessKey != nil && *in.SecretAccessKey != "" {
		if err := m.d.Settings.SetSecret(ctx, keyS3Secret, secret); err != nil {
			return err
		}
	}
	if active {
		// A different bucket may hold different files: forget the cached copies.
		if err := m.clearCache(); err != nil {
			return err
		}
		raw, cache, err := m.build(ctx, api.S3)
		if err != nil {
			return err
		}
		m.raw, m.cache = raw, cache
		m.d.Files.Swap(m.storeFor(raw, cache))
		m.usage.clear()
	}
	return nil
}

// TestStorageS3 is POST /storage/s3/test. The body, when there is one, is
// tried instead of the saved setup; fields it leaves out come from the saved one.
func (m *Module) TestStorageS3(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in api.StorageS3Input
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err == nil && len(bytes.TrimSpace(raw)) > 0 {
		err = json.Unmarshal(raw, &in)
		if err != nil {
			err = httpx.Invalid("请求体格式不正确")
		}
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	s, secret, err := m.merged(ctx, in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !s.complete(secret) {
		httpx.JSON(w, http.StatusOK, api.TestResult{Ok: false, Message: "请先填写完整的 S3 设置"})
		return
	}
	remote, err := files.NewS3(s.config(secret))
	if err != nil {
		httpx.JSON(w, http.StatusOK, api.TestResult{Ok: false, Message: err.Error()})
		return
	}
	if err := remote.Check(ctx); err != nil {
		httpx.JSON(w, http.StatusOK, api.TestResult{Ok: false, Message: err.Error()})
		return
	}
	httpx.JSON(w, http.StatusOK, api.TestResult{Ok: true, Message: "连接正常"})
}

// PutStorageCache is PUT /storage/cache.
func (m *Module) PutStorageCache(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in api.PutStorageCacheJSONBody
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if in.LimitBytes < 0 {
		httpx.Fail(w, r, httpx.Invalid("缓存上限不能是负数"))
		return
	}
	if err := m.d.Settings.Set(ctx, keyCacheLimit, in.LimitBytes); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	m.mu.Lock()
	cache := m.cache
	m.mu.Unlock()
	if cache != nil {
		if err := cache.SetLimit(ctx, in.LimitBytes); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	m.respondStatus(w, r)
}
