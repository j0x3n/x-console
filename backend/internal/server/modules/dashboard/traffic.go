package dashboard

import (
 "encoding/json"
 "io"
 "net/http"
 "os"
 "path/filepath"
)

// Traffic is a credential-free local snapshot, never a user-controlled path/URL.
// Mount stays on the normal authenticated Console router.
func (m *Module) traffic(w http.ResponseWriter, r *http.Request) {
 f, err := os.Open(filepath.Join(m.d.Config.DataDir, "naive-traffic.json"))
 if err != nil { http.Error(w, "流量采集尚未接入", http.StatusServiceUnavailable); return }
 defer f.Close()
 raw, err := io.ReadAll(io.LimitReader(f, 1<<20+1))
 if err != nil || len(raw)>1<<20 || !json.Valid(raw) {
  http.Error(w,"流量快照不可用",http.StatusServiceUnavailable);return
 }
 w.Header().Set("Content-Type","application/json")
 w.Header().Set("Cache-Control","no-store")
 w.Write(raw)
}
