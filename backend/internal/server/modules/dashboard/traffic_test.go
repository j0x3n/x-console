package dashboard

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/config"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

func TestTrafficSnapshot(t *testing.T) {
	dir := t.TempDir()
	m := &Module{d: &module.Deps{Config: config.Config{DataDir: dir}}}
	for _, tc := range []struct {
		data string
		want int
	}{{"", 503}, {"invalid", 503}, {`{"time":1,"accounts":[]}`, 200}} {
		if tc.data != "" {
			if err := os.WriteFile(filepath.Join(dir, "naive-traffic.json"), []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
		}
		w := httptest.NewRecorder()
		m.traffic(w, httptest.NewRequest("GET", "/dashboard/traffic", nil))
		if w.Code != tc.want {
			t.Fatalf("status %d want %d", w.Code, tc.want)
		}
	}
}
