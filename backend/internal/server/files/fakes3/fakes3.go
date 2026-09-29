// Package fakes3 is a small in-memory S3 server for tests. It speaks the
// parts of the protocol that minio-go needs to store, read, list, copy and
// delete objects in one bucket with path-style addresses. It does not check
// signatures.
package fakes3

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server is a running fake S3.
type Server struct {
	*httptest.Server
	Bucket string

	// Fail, when set, is asked before every object request. A non-zero result
	// is sent back as the HTTP status instead of doing the work.
	Fail func(method, key string) int

	mu      sync.Mutex
	objects map[string]object
	Puts    int // number of successful PUT requests that stored data
	Gets    int // number of GET requests that read an object
}

type object struct {
	data    []byte
	modTime time.Time
}

// New starts a fake S3 with one bucket and stops it when the test ends.
func New(t testing.TB, bucket string) *Server {
	s := &Server{Bucket: bucket, objects: map[string]object{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Objects returns a copy of the stored objects by key.
func (s *Server) Objects() map[string][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string][]byte, len(s.objects))
	for k, o := range s.objects {
		out[k] = bytes.Clone(o.data)
	}
	return out
}

// GetCount is the number of object reads so far.
func (s *Server) GetCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Gets
}

// Set stores an object directly.
func (s *Server) Set(key string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = object{data: bytes.Clone(data), modTime: time.Now().UTC()}
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	bucket, key, _ := strings.Cut(path, "/")
	if bucket != s.Bucket {
		writeError(w, r, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist")
		return
	}
	if key == "" {
		s.serveBucket(w, r)
		return
	}
	if s.Fail != nil {
		if status := s.Fail(r.Method, key); status != 0 {
			writeError(w, r, status, "InternalError", "injected failure")
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case http.MethodPut:
		if src := r.Header.Get("X-Amz-Copy-Source"); src != "" {
			s.copyObject(w, r, key, src)
			return
		}
		data, err := readBody(r)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "IncompleteBody", err.Error())
			return
		}
		s.objects[key] = object{data: data, modTime: time.Now().UTC()}
		s.Puts++
		w.Header().Set("ETag", etag(data))
		w.WriteHeader(http.StatusOK)
	case http.MethodGet, http.MethodHead:
		if r.Method == http.MethodGet {
			s.Gets++
		}
		o, ok := s.objects[key]
		if !ok {
			writeError(w, r, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
			return
		}
		serveObject(w, r, o)
	case http.MethodDelete:
		delete(s.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", r.Method)
	}
}

func (s *Server) serveBucket(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && q.Has("location"):
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
	case r.Method == http.MethodGet:
		s.list(w, r)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", r.Method)
	}
}

type listContent struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int    `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type listResult struct {
	XMLName               xml.Name      `xml:"ListBucketResult"`
	Xmlns                 string        `xml:"xmlns,attr"`
	Name                  string        `xml:"Name"`
	Prefix                string        `xml:"Prefix"`
	KeyCount              int           `xml:"KeyCount"`
	MaxKeys               int           `xml:"MaxKeys"`
	IsTruncated           bool          `xml:"IsTruncated"`
	NextContinuationToken string        `xml:"NextContinuationToken,omitempty"`
	EncodingType          string        `xml:"EncodingType,omitempty"`
	Contents              []listContent `xml:"Contents"`
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prefix := q.Get("prefix")
	after := q.Get("continuation-token")
	if after == "" {
		after = q.Get("start-after")
	}
	if after == "" {
		after = q.Get("marker")
	}
	limit := 1000
	if n, err := strconv.Atoi(q.Get("max-keys")); err == nil && n > 0 && n < limit {
		limit = n
	}
	s.mu.Lock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		if strings.HasPrefix(k, prefix) && k > after {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	res := listResult{Xmlns: "http://s3.amazonaws.com/doc/2006-03-01/", Name: s.Bucket, Prefix: prefix, MaxKeys: limit,
		EncodingType: q.Get("encoding-type")}
	if len(keys) > limit {
		keys = keys[:limit]
		res.IsTruncated = true
		res.NextContinuationToken = keys[len(keys)-1]
	}
	for _, k := range keys {
		o := s.objects[k]
		name := k
		if res.EncodingType == "url" {
			name = url.QueryEscape(k)
		}
		res.Contents = append(res.Contents, listContent{Key: name, LastModified: o.modTime.Format("2006-01-02T15:04:05.000Z"),
			ETag: etag(o.data), Size: len(o.data), StorageClass: "STANDARD"})
	}
	s.mu.Unlock()
	res.KeyCount = len(res.Contents)
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(res)
}

func (s *Server) copyObject(w http.ResponseWriter, r *http.Request, key, src string) {
	src, _ = url.PathUnescape(strings.TrimPrefix(src, "/"))
	src = strings.TrimPrefix(src, s.Bucket+"/")
	o, ok := s.objects[src]
	if !ok {
		writeError(w, r, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	s.objects[key] = object{data: bytes.Clone(o.data), modTime: time.Now().UTC()}
	s.Puts++
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<CopyObjectResult><ETag>%s</ETag><LastModified>%s</LastModified></CopyObjectResult>`,
		etag(o.data), time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
}

func serveObject(w http.ResponseWriter, r *http.Request, o object) {
	h := w.Header()
	h.Set("ETag", etag(o.data))
	h.Set("Last-Modified", o.modTime.Format(http.TimeFormat))
	h.Set("Accept-Ranges", "bytes")
	data := o.data
	status := http.StatusOK
	if rng := r.Header.Get("Range"); rng != "" && r.Method == http.MethodGet {
		start, end, ok := parseRange(rng, int64(len(data)))
		if !ok {
			h.Set("Content-Range", fmt.Sprintf("bytes */%d", len(data)))
			writeError(w, r, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "The requested range is not satisfiable")
			return
		}
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
		data = data[start : end+1]
		status = http.StatusPartialContent
	}
	h.Set("Content-Length", strconv.Itoa(len(data)))
	h.Set("Content-Type", "application/octet-stream")
	w.WriteHeader(status)
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}

// parseRange reads "bytes=a-b", "bytes=a-" and "bytes=-n".
func parseRange(header string, size int64) (start, end int64, ok bool) {
	spec, found := strings.CutPrefix(header, "bytes=")
	if !found || size == 0 {
		return 0, 0, false
	}
	from, to, _ := strings.Cut(spec, "-")
	switch {
	case from == "":
		n, err := strconv.ParseInt(to, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		return max(size-n, 0), size - 1, true
	default:
		a, err := strconv.ParseInt(from, 10, 64)
		if err != nil || a >= size {
			return 0, 0, false
		}
		b := size - 1
		if to != "" {
			if b2, err := strconv.ParseInt(to, 10, 64); err == nil {
				b = min(b2, size-1)
			}
		}
		return a, b, a <= b
	}
}

// readBody reads a PUT body, undoing the "aws-chunked" framing minio-go uses
// on plain HTTP connections.
func readBody(r *http.Request) ([]byte, error) {
	if !strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-") {
		return io.ReadAll(r.Body)
	}
	br := bufio.NewReader(r.Body)
	var out bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeHex, _, _ := strings.Cut(strings.TrimSpace(line), ";")
		n, err := strconv.ParseInt(sizeHex, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("bad chunk header %q", line)
		}
		if n == 0 {
			return out.Bytes(), nil
		}
		if _, err := io.CopyN(&out, br, n); err != nil {
			return nil, err
		}
		if _, err := br.Discard(2); err != nil { // trailing \r\n
			return nil, err
		}
	}
}

func etag(data []byte) string {
	sum := md5.Sum(data)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message><Resource>%s</Resource></Error>`,
		code, xmlEscape(message), xmlEscape(r.URL.Path))
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
