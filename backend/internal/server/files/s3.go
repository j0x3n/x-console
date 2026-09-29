package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Config is what it takes to reach a bucket.
type S3Config struct {
	Endpoint        string // http://host:port or https://host, no path
	Region          string
	Bucket          string
	Prefix          string // keys live under this prefix in the bucket
	AccessKeyID     string
	SecretAccessKey string
	PathStyle       bool // bucket in the path, for MinIO and most self-hosted servers
}

// ValidateS3 checks the fields that do not need a connection.
func ValidateS3(c S3Config) error {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return errors.New("S3 地址必须是 http 或 https 开头，不能带路径")
	}
	if c.Bucket == "" || strings.ContainsAny(c.Bucket, "/\\ ") {
		return errors.New("S3 桶名不正确")
	}
	if c.Prefix != "" {
		if _, err := cleanPrefix(c.Prefix); err != nil {
			return errors.New("S3 前缀不正确")
		}
	}
	return nil
}

// S3 keeps files in an S3 bucket. Each key is an object named Prefix/key.
type S3 struct {
	Client *minio.Client
	Bucket string
	Prefix string // "" or "dir/" with a trailing slash
}

var _ Store = (*S3)(nil)

// NewS3 builds a client for the bucket. It does not connect yet; call Check.
func NewS3(c S3Config) (*S3, error) {
	if err := ValidateS3(c); err != nil {
		return nil, err
	}
	u, _ := url.Parse(c.Endpoint)
	lookup := minio.BucketLookupAuto
	if c.PathStyle {
		lookup = minio.BucketLookupPath
	}
	dial := (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	client, err := minio.New(u.Host, &minio.Options{
		Creds: credentials.NewStaticV4(c.AccessKeyID, c.SecretAccessKey, ""), Secure: u.Scheme == "https",
		Region: c.Region, BucketLookup: lookup, MaxRetries: 2,
		Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: dial, TLSHandshakeTimeout: 10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second, MaxIdleConnsPerHost: 8},
	})
	if err != nil {
		return nil, err
	}
	prefix := strings.Trim(c.Prefix, "/")
	if prefix != "" {
		prefix += "/"
	}
	return &S3{Client: client, Bucket: c.Bucket, Prefix: prefix}, nil
}

// Check writes, reads and deletes a small file, and returns a short Chinese
// message for the settings page that says what is wrong.
func (s *S3) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	exists, err := s.Client.BucketExists(ctx, s.Bucket)
	if err != nil {
		return explainS3(err)
	}
	if !exists {
		return errors.New("桶不存在")
	}
	const probe = ".x-console-test"
	const body = "x-console"
	if err := s.Put(ctx, probe, strings.NewReader(body), int64(len(body))); err != nil {
		return explainS3(err)
	}
	rc, _, err := s.Get(ctx, probe)
	if err != nil {
		return explainS3(err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil || string(got) != body {
		return errors.New("写进去的内容读回来不一样")
	}
	if err := s.Delete(ctx, probe); err != nil {
		return explainS3(err)
	}
	return nil
}

// explainS3 turns an S3 error into a short message a person can act on.
func explainS3(err error) error {
	var resp minio.ErrorResponse
	if errors.As(err, &resp) {
		switch resp.Code {
		case "NoSuchBucket":
			return errors.New("桶不存在")
		case "InvalidAccessKeyId":
			return errors.New("访问密钥 ID 不对")
		case "SignatureDoesNotMatch":
			return errors.New("密钥不对")
		case "AccessDenied":
			return errors.New("没有权限，检查密钥的权限和桶名")
		case "AuthorizationHeaderMalformed", "InvalidRegion":
			return errors.New("区域不对")
		}
		if resp.Message != "" {
			return fmt.Errorf("%s：%s", resp.Code, resp.Message)
		}
		return errors.New(resp.Code)
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return errors.New("连接超时")
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return errors.New("解析不了这个地址")
	}
	return err
}

func (s *S3) object(key string) (string, error) {
	if err := CheckKey(key); err != nil {
		return "", err
	}
	return s.Prefix + key, nil
}

func isNoSuchKey(err error) bool {
	var resp minio.ErrorResponse
	return errors.As(err, &resp) && (resp.Code == "NoSuchKey" || resp.StatusCode == http.StatusNotFound && resp.Code != "NoSuchBucket")
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	obj, err := s.object(key)
	if err != nil {
		return err
	}
	_, err = s.Client.PutObject(ctx, s.Bucket, obj, r, size, minio.PutObjectOptions{})
	return err
}

func (s *S3) info(key string, st minio.ObjectInfo) Info {
	return Info{Key: key, Size: st.Size, ModTime: st.LastModified}
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	return s.GetRange(ctx, key, 0, -1)
}

func (s *S3) GetRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, Info, error) {
	obj, err := s.object(key)
	if err != nil {
		return nil, Info{}, err
	}
	st, err := s.Client.StatObject(ctx, s.Bucket, obj, minio.StatObjectOptions{})
	if err != nil {
		if isNoSuchKey(err) {
			return nil, Info{}, ErrNotFound
		}
		return nil, Info{}, err
	}
	info := s.info(key, st)
	if offset < 0 {
		offset = 0
	}
	if offset >= st.Size || length == 0 {
		return io.NopCloser(strings.NewReader("")), info, nil
	}
	var opts minio.GetObjectOptions
	switch {
	case length > 0:
		err = opts.SetRange(offset, min(offset+length, st.Size)-1)
	case offset > 0:
		err = opts.SetRange(offset, 0)
	}
	if err != nil {
		return nil, Info{}, err
	}
	o, err := s.Client.GetObject(ctx, s.Bucket, obj, opts)
	if err != nil {
		return nil, Info{}, err
	}
	return o, info, nil
}

func (s *S3) Stat(ctx context.Context, key string) (Info, error) {
	obj, err := s.object(key)
	if err != nil {
		return Info{}, err
	}
	st, err := s.Client.StatObject(ctx, s.Bucket, obj, minio.StatObjectOptions{})
	if err != nil {
		if isNoSuchKey(err) {
			return Info{}, ErrNotFound
		}
		return Info{}, err
	}
	return s.info(key, st), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	obj, err := s.object(key)
	if err != nil {
		return err
	}
	err = s.Client.RemoveObject(ctx, s.Bucket, obj, minio.RemoveObjectOptions{})
	if err != nil && isNoSuchKey(err) {
		return nil
	}
	return err
}

func (s *S3) List(ctx context.Context, prefix string) iter.Seq2[Info, error] {
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return failed(err)
	}
	full := s.Prefix
	if prefix != "" {
		full += prefix + "/"
	}
	return func(yield func(Info, error) bool) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		for o := range s.Client.ListObjects(ctx, s.Bucket, minio.ListObjectsOptions{Prefix: full, Recursive: true}) {
			if o.Err != nil {
				yield(Info{}, o.Err)
				return
			}
			key := strings.TrimPrefix(o.Key, s.Prefix)
			if strings.HasSuffix(key, "/") { // folder marker made by other tools
				continue
			}
			if !yield(Info{Key: key, Size: o.Size, ModTime: o.LastModified}, nil) {
				return
			}
		}
	}
}

func (s *S3) Copy(ctx context.Context, from, to string) error {
	src, err := s.object(from)
	if err != nil {
		return err
	}
	dst, err := s.object(to)
	if err != nil {
		return err
	}
	_, err = s.Client.CopyObject(ctx, minio.CopyDestOptions{Bucket: s.Bucket, Object: dst}, minio.CopySrcOptions{Bucket: s.Bucket, Object: src})
	if err != nil && isNoSuchKey(err) {
		return ErrNotFound
	}
	return err
}
