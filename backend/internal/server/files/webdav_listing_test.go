package files_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/files"
)

func TestWebDAVListingRejectsUnreadableEntry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/blobs/a</d:href><d:propstat><d:status>HTTP/1.1 403 Forbidden</d:status><d:prop/></d:propstat></d:response></d:multistatus>`))
	}))
	defer srv.Close()
	s, err := files.NewWebDAV(files.WebDAVConfig{URL: srv.URL + "/dav/"})
	if err != nil {
		t.Fatal(err)
	}
	var got error
	for _, err := range s.List(context.Background(), "") {
		if err != nil {
			got = err
			break
		}
	}
	if got == nil || !strings.Contains(got.Error(), "属性读取失败") {
		t.Fatalf("listing failure %v", got)
	}
}

func TestWebDAVListingRejectsDisappearingChild(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/dav/blobs/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<d:multistatus xmlns:d="DAV:"><d:response><d:href>/dav/</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response><d:response><d:href>/dav/blobs/</d:href><d:propstat><d:status>HTTP/1.1 200 OK</d:status><d:prop><d:resourcetype><d:collection/></d:resourcetype></d:prop></d:propstat></d:response></d:multistatus>`))
	}))
	defer srv.Close()
	s, err := files.NewWebDAV(files.WebDAVConfig{URL: srv.URL + "/dav/"})
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	for _, err := range s.List(context.Background(), "") {
		if err != nil {
			failed = true
			break
		}
	}
	if !failed {
		t.Fatal("disappearing directory was silently omitted")
	}
}
