package drive

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/auth"
	"github.com/j0x3n/x-console/backend/internal/server/httpx"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/api"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive/db"
	"github.com/j0x3n/x-console/backend/internal/server/settings"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const configKey = "drive.s3.config"
const secretKey = "drive.s3.secret"

func (m *Module) config(ctx context.Context) (api.S3Config, error) {
	c := api.S3Config{}
	err := m.d.Settings.Get(ctx, configKey, &c)
	if errors.Is(err, settings.ErrNotSet) {
		err = nil
	}
	if err != nil {
		return c, err
	}
	c.HasSecret, err = m.d.Settings.Has(ctx, secretKey)
	return c, err
}
func overlay(c *api.S3Config, in api.S3ConfigInput) {
	if in.Endpoint != nil {
		c.Endpoint = *in.Endpoint
	}
	if in.Region != nil {
		c.Region = *in.Region
	}
	if in.Bucket != nil {
		c.Bucket = *in.Bucket
	}
	if in.Prefix != nil {
		c.Prefix = *in.Prefix
	}
	if in.AccessKeyId != nil {
		c.AccessKeyId = *in.AccessKeyId
	}
	if in.PathStyle != nil {
		c.PathStyle = *in.PathStyle
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	if in.IncludeHidden != nil {
		c.IncludeHidden = *in.IncludeHidden
	}
}
func validateS3(c api.S3Config) error {
	if !c.Enabled && c.Endpoint == "" && c.Bucket == "" {
		return nil
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return httpx.Invalid("S3 Endpoint 必须是 http 或 https 地址，不能带路径")
	}
	if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/\\ ") {
		return httpx.Invalid("S3 Bucket 不正确")
	}
	if strings.Contains(c.Prefix, "..") || strings.HasPrefix(c.Prefix, "/") || strings.Contains(c.Prefix, "\\") {
		return httpx.Invalid("S3 Prefix 不正确")
	}
	return nil
}
func (m *Module) s3Client(ctx context.Context, c api.S3Config, secret string) (*minio.Client, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	dial := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	lookup := minio.BucketLookupAuto
	if c.PathStyle {
		lookup = minio.BucketLookupPath
	}
	return minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKeyId, secret, ""), Secure: u.Scheme == "https", Region: c.Region, BucketLookup: lookup, MaxRetries: 2, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dial, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second}})
}
func (m *Module) secret(ctx context.Context) (string, error) {
	var s string
	err := m.d.Settings.Get(ctx, secretKey, &s)
	if errors.Is(err, settings.ErrNotSet) {
		return "", nil
	}
	return s, err
}
func (m *Module) GetDriveS3Config(w http.ResponseWriter, r *http.Request) {
	c, err := m.config(r.Context())
	if fail(w, r, err) {
		return
	}
	httpx.JSON(w, 200, c)
}
func (m *Module) PutDriveS3Config(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if fail(w, r, auth.RequireElevated(ctx)) {
		return
	}
	var in api.S3ConfigInput
	if fail(w, r, httpx.Decode(r, &in)) {
		return
	}
	c, err := m.config(ctx)
	if fail(w, r, err) {
		return
	}
	old := c
	overlay(&c, in)
	if fail(w, r, validateS3(c)) {
		return
	}
	if c.Enabled && (c.AccessKeyId == "" || (!c.HasSecret && (in.SecretAccessKey == nil || *in.SecretAccessKey == ""))) {
		httpx.Fail(w, r, httpx.Invalid("请填写 S3 密钥"))
		return
	}
	if in.SecretAccessKey != nil && *in.SecretAccessKey == "" {
		httpx.Fail(w, r, httpx.Invalid("密钥不能为空"))
		return
	}
	m.runMu.Lock()
	err = func() error {
		if in.SecretAccessKey != nil {
			if e := m.d.Settings.SetSecret(ctx, secretKey, *in.SecretAccessKey); e != nil {
				return e
			}
			c.HasSecret = true
		}
		if e := m.d.Settings.Set(ctx, configKey, c); e != nil {
			return e
		}
		if c.Endpoint != old.Endpoint || c.Bucket != old.Bucket || c.Prefix != old.Prefix || c.AccessKeyId != old.AccessKeyId || c.PathStyle != old.PathStyle || in.SecretAccessKey != nil {
			_, e := m.d.DB.ExecContext(ctx, "UPDATE drive_items SET s3_synced_at=NULL,s3_key=NULL,s3_hash=NULL,s3_error=NULL")
			return e
		}
		return nil
	}()
	m.runMu.Unlock()
	if fail(w, r, err) {
		return
	}
	m.audit(ctx, "drive.s3.configure", 0, nil)
	m.triggerSync()
	httpx.JSON(w, 200, c)
}
func (m *Module) TestDriveS3(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c, err := m.config(ctx)
	if fail(w, r, err) {
		return
	}
	secret, err := m.secret(ctx)
	if fail(w, r, err) {
		return
	}
	if r.ContentLength != 0 {
		var in api.S3ConfigInput
		if fail(w, r, httpx.Decode(r, &in)) {
			return
		}
		overlay(&c, in)
		if in.SecretAccessKey != nil {
			secret = *in.SecretAccessKey
		}
	}
	if fail(w, r, validateS3(c)) {
		return
	}
	if c.Endpoint == "" || c.Bucket == "" || c.AccessKeyId == "" || secret == "" {
		httpx.Fail(w, r, httpx.Invalid("请先填写完整的 S3 配置"))
		return
	}
	client, err := m.s3Client(ctx, c, secret)
	if fail(w, r, err) {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	key := path.Join(c.Prefix, ".x-console-test-"+time.Now().UTC().Format("20060102150405.000000000"))
	_, err = client.PutObject(ctx, c.Bucket, key, bytes.NewReader([]byte("ok")), 2, minio.PutObjectOptions{ContentType: "text/plain"})
	if err == nil {
		err = client.RemoveObject(ctx, c.Bucket, key, minio.RemoveObjectOptions{})
	}
	if err != nil {
		httpx.JSON(w, 200, map[string]any{"ok": false, "message": err.Error()})
		return
	}
	httpx.JSON(w, 200, map[string]any{"ok": true, "message": "连接成功"})
}

func (m *Module) GetDriveS3Status(w http.ResponseWriter, r *http.Request) {
	c, err := m.config(r.Context())
	if fail(w, r, err) {
		return
	}
	m.syncMu.Lock()
	status := m.syncStatus
	m.syncMu.Unlock()
	if !c.Enabled {
		status.State = "off"
		status.Pending = 0
		status.Synced = 0
		status.Failed = 0
	}
	if c.Enabled {
		var pending, synced, failed int
		err = m.d.DB.QueryRowContext(r.Context(), `SELECT count(*) FILTER (WHERE s3_synced_at IS NULL AND (s3_error IS NULL OR s3_error='')),count(*) FILTER (WHERE s3_synced_at IS NOT NULL),count(*) FILTER (WHERE s3_error IS NOT NULL AND s3_error<>'') FROM drive_items WHERE is_dir=0 AND trashed_at IS NULL AND (hidden=0 OR ?)`, c.IncludeHidden).Scan(&pending, &synced, &failed)
		if fail(w, r, err) {
			return
		}
		status.Pending = pending
		status.Synced = synced
		status.Failed = failed
		if status.State == "off" {
			status.State = "idle"
		}
		if failed > 0 && status.State != "syncing" {
			status.State = "failed"
		}
	}
	httpx.JSON(w, 200, status)
}
func (m *Module) SyncDriveS3(w http.ResponseWriter, r *http.Request) {
	c, err := m.config(r.Context())
	if fail(w, r, err) {
		return
	}
	if !c.Enabled {
		httpx.Fail(w, r, httpx.ErrIntegrationMissing)
		return
	}
	m.triggerSync()
	w.WriteHeader(http.StatusAccepted)
}

// fullSyncEvery is how often syncAll checks every file against the bucket.
const fullSyncEvery = 24 * time.Hour

func (m *Module) triggerSync() {
	select {
	case m.syncReq <- struct{}{}:
	default:
	}
}
func (m *Module) syncAll(ctx context.Context) error {
	m.runMu.Lock()
	defer m.runMu.Unlock()
	c, err := m.config(ctx)
	if err != nil || !c.Enabled {
		m.syncMu.Lock()
		m.syncStatus.State = "off"
		m.syncMu.Unlock()
		return err
	}
	secret, err := m.secret(ctx)
	if err != nil {
		return err
	}
	client, err := m.s3Client(ctx, c, secret)
	if err != nil {
		return err
	}
	m.syncMu.Lock()
	m.syncStatus.State = "syncing"
	m.syncMu.Unlock()
	m.d.Bus.Publish("drive.sync_status", map[string]any{"state": "syncing"})
	defer func() {
		now := time.Now().UTC()
		m.syncMu.Lock()
		m.syncStatus.LastRunAt = &now
		state := m.syncStatus.State
		m.syncMu.Unlock()
		m.d.Bus.Publish("drive.sync_status", map[string]any{"state": state})
	}()
	// Only files that changed since their last upload, or failed, or are
	// hidden and must leave the bucket. Renames and moves bump updated_at of
	// the whole subtree, so a changed object key is caught too. Once a day
	// every file is checked against the bucket as well.
	full := time.Since(m.lastFullSync) >= fullSyncEvery
	query := "SELECT id FROM drive_items WHERE is_dir=0 AND trashed_at IS NULL"
	if !full {
		changed := "(s3_synced_at IS NULL OR s3_synced_at<updated_at OR s3_hash IS NOT sha256 OR COALESCE(s3_error,'')<>'')"
		if c.IncludeHidden {
			query += " AND " + changed
		} else {
			query += " AND ((hidden=0 AND " + changed + ") OR (hidden=1 AND s3_key IS NOT NULL))"
		}
	}
	rows, err := m.d.DB.QueryContext(ctx, query+" ORDER BY id")
	if err != nil {
		m.syncMu.Lock()
		m.syncStatus.State = "failed"
		m.syncMu.Unlock()
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		m.syncMu.Lock()
		m.syncStatus.State = "failed"
		m.syncMu.Unlock()
		return err
	}
	deleteErr := m.syncDeletes(ctx, client, c)
	m.syncMu.Lock()
	if deleteErr == nil {
		m.syncStatus.State = "idle"
		m.syncStatus.LastError = nil
	} else {
		msg := deleteErr.Error()
		m.syncStatus.State = "failed"
		m.syncStatus.LastError = &msg
	}
	m.syncMu.Unlock()
	err = deleteErr
	for _, id := range ids {
		item, e := m.row(ctx, id)
		if e != nil {
			err = e
			break
		}
		if item.Hidden != 0 && !c.IncludeHidden {
			if item.S3Key != nil {
				e = client.RemoveObject(ctx, c.Bucket, *item.S3Key, minio.RemoveObjectOptions{})
				if e == nil {
					_, e = m.d.DB.ExecContext(ctx, "UPDATE drive_items SET s3_key=NULL,s3_synced_at=NULL,s3_error=NULL WHERE id=?", id)
				}
			}
		} else {
			e = m.syncItem(ctx, client, c, item, full)
		}
		if e != nil {
			msg := e.Error()
			_, _ = m.d.DB.ExecContext(ctx, "UPDATE drive_items SET s3_error=? WHERE id=?", msg, id)
			m.syncMu.Lock()
			m.syncStatus.State = "failed"
			m.syncStatus.LastError = &msg
			m.syncMu.Unlock()
			err = e
		}
	}
	if err == nil && full {
		m.lastFullSync = time.Now()
	}
	return err
}
func (m *Module) objectKey(ctx context.Context, c api.S3Config, item db.DriveItem) (string, error) {
	ancestors, err := m.path(ctx, item)
	if err != nil {
		return "", err
	}
	parts := []string{}
	if c.Prefix != "" {
		parts = append(parts, c.Prefix)
	}
	if item.Hidden != 0 {
		parts = append(parts, ".hidden")
	}
	for _, part := range ancestors {
		parts = append(parts, part.Name)
	}
	parts = append(parts, item.Name)
	return path.Join(parts...), nil
}

// syncItem uploads one file. An up-to-date file is trusted without asking the
// bucket, unless verify is set.
func (m *Module) syncItem(ctx context.Context, client *minio.Client, c api.S3Config, item db.DriveItem, verify bool) error {
	key, err := m.objectKey(ctx, c, item)
	if err != nil {
		return err
	}
	if item.S3Key != nil && *item.S3Key == key && item.S3Hash != nil && *item.S3Hash == item.Sha256 && item.S3SyncedAt != nil && !item.S3SyncedAt.Before(item.UpdatedAt) {
		if !verify {
			return nil
		}
		if _, e := client.StatObject(ctx, c.Bucket, key, minio.StatObjectOptions{}); e == nil {
			return nil
		}
	}
	var info minio.UploadInfo
	copied := item.S3Key != nil && *item.S3Key != "" && *item.S3Key != key && item.S3Hash != nil && *item.S3Hash == item.Sha256
	if copied {
		info, err = client.CopyObject(ctx, minio.CopyDestOptions{Bucket: c.Bucket, Object: key}, minio.CopySrcOptions{Bucket: c.Bucket, Object: *item.S3Key})
		if err != nil {
			copied = false
		}
	}
	if !copied {
		f, _, e := m.store.Get(ctx, blobKey(item.Sha256))
		if e != nil {
			return e
		}
		info, err = client.PutObject(ctx, c.Bucket, key, f, item.Size, minio.PutObjectOptions{ContentType: item.Mime})
		f.Close()
	}
	if err == nil && item.S3Key != nil && *item.S3Key != "" && *item.S3Key != key {
		err = client.RemoveObject(ctx, c.Bucket, *item.S3Key, minio.RemoveObjectOptions{})
	}
	if err != nil {
		return err
	}
	// Record the version that was uploaded, not the current time: if the
	// file was renamed or changed meanwhile, updated_at is newer and the next
	// run picks it up again.
	_, err = m.d.DB.ExecContext(ctx, "UPDATE drive_items SET s3_key=?,s3_hash=?,s3_etag=?,s3_synced_at=?,s3_error=NULL WHERE id=?", key, item.Sha256, info.ETag, item.UpdatedAt, item.ID)
	return err
}
func (m *Module) syncDeletes(ctx context.Context, client *minio.Client, c api.S3Config) error {
	rows, err := m.d.DB.QueryContext(ctx, "SELECT key FROM drive_s3_deletions ORDER BY created_at")
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			break
		}
		keys = append(keys, key)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	var first error
	for _, key := range keys {
		var refs int
		if err = m.d.DB.QueryRowContext(ctx, "SELECT count(*) FROM drive_items WHERE s3_key=?", key).Scan(&refs); err != nil {
			return err
		}
		if refs == 0 {
			if err = client.RemoveObject(ctx, c.Bucket, key, minio.RemoveObjectOptions{}); err != nil {
				if first == nil {
					first = err
				}
				continue
			}
		}
		if _, err = m.d.DB.ExecContext(ctx, "DELETE FROM drive_s3_deletions WHERE key=?", key); err != nil {
			return err
		}
	}
	return first
}
