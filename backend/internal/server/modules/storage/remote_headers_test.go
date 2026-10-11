package storage

import (
	"net/http/httptest"
	"testing"
)

func TestRemotePreviewSandbox(t *testing.T) {
	for _, mimeType := range []string{"text/html", "image/svg+xml", "image/svg+xml; charset=utf-8", "application/xhtml+xml"} {
		t.Run(mimeType, func(t *testing.T) {
			w := httptest.NewRecorder()
			remoteHeaders(w, "preview", mimeType, true)
			if w.Header().Get("Content-Security-Policy") != "sandbox" {
				t.Fatal("active preview must be sandboxed")
			}
		})
	}
	for _, mimeType := range []string{"application/pdf", "image/png", "text/plain; charset=utf-8", "video/mp4"} {
		w := httptest.NewRecorder()
		remoteHeaders(w, "preview", mimeType, true)
		if w.Header().Get("Content-Security-Policy") != "" {
			t.Fatalf("sandbox prevents native preview: %s", mimeType)
		}
	}
}
