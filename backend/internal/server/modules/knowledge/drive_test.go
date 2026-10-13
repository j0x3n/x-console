package knowledge_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge"
	"github.com/j0x3n/x-console/backend/internal/server/modules/knowledge/api"
)

func (f *fixture) mkdir(t *testing.T, parent int64, name string) int64 {
	t.Helper()
	var out struct{ Id int64 }
	f.env.MustDo(http.MethodPost, "/drive/folders", map[string]any{"parentId": parent, "name": name}, &out)
	return out.Id
}

func (f *fixture) upload(t *testing.T, parent int64, name string, data []byte) int64 {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(data)
	w.Close()
	req, err := http.NewRequest(http.MethodPost, f.env.URL(fmt.Sprintf("/drive/upload?parent=%d", parent)), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := f.env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload %s: %d %s", name, resp.StatusCode, raw)
	}
	var out struct{ Items []struct{ Id int64 } }
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Items) != 1 {
		t.Fatalf("upload result: %s", raw)
	}
	return out.Items[0].Id
}

func (f *fixture) useFolders(t *testing.T, ids ...int64) api.KnowledgeSettings {
	t.Helper()
	folders := []api.KnowledgeDriveFolder{}
	for _, id := range ids {
		folders = append(folders, api.KnowledgeDriveFolder{Id: id})
	}
	var got api.KnowledgeSettings
	f.env.MustDo(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Sources: []string{"notes"}, DriveFolders: &folders}, &got)
	return got
}

// hide makes the given modules hidden and locked; none restores them.
func (f *fixture) hide(t *testing.T, modules ...string) {
	t.Helper()
	h := hiddenSet{map[string]bool{}}
	for _, m := range modules {
		h.set[m] = true
	}
	module.Provide[contracts.HiddenModules](f.env.App.Deps.Registry, contracts.HiddenModulesKey, h)
}

func (f *fixture) sentText(sub string) bool {
	f.ai.mu.Lock()
	defer f.ai.mu.Unlock()
	for _, in := range f.ai.inputs {
		for _, s := range in {
			if strings.Contains(s, sub) {
				return true
			}
		}
	}
	return false
}

// B199: files in the chosen drive folders are indexed, others are not.
func TestDriveFolders(t *testing.T) {
	f := setup(t)
	docs := f.mkdir(t, 0, "资料")
	sub := f.mkdir(t, docs, "运维")
	other := f.mkdir(t, 0, "照片")
	f.upload(t, sub, "caddy.md", []byte("# Caddy\n证书续不上，因为 80 端口被占用"))
	f.upload(t, docs, "plan.docx", knowledge.MakeDocx("旅行计划", "去看咖啡庄园"))
	f.upload(t, docs, "manual.pdf", knowledge.MakePDF("database backup manual"))
	f.upload(t, docs, "scan.pdf", []byte("%PDF-1.4 not really"))
	f.upload(t, docs, "photo.png", []byte("\x89PNG"))
	f.upload(t, other, "secret.txt", []byte("数据库密码"))

	// Folders are checked when saved.
	bad := []api.KnowledgeDriveFolder{{Id: 9999}}
	if code, _ := f.env.Do(http.MethodPut, "/knowledge/settings", api.KnowledgeSettings{Enabled: true, Sources: []string{}, DriveFolders: &bad}, nil); code != http.StatusBadRequest && code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown folder: %d", code)
	}
	got := f.useFolders(t, docs, docs)
	if got.DriveFolders == nil || len(*got.DriveFolders) != 1 || (*got.DriveFolders)[0].Path == nil || *(*got.DriveFolders)[0].Path != "/资料" {
		t.Fatalf("saved folders: %+v", got.DriveFolders)
	}

	f.sync()
	st := f.status(t)
	if st.LastError != nil || st.Docs != 3 || st.Unreadable == nil || *st.Unreadable != 1 {
		t.Fatalf("status: %+v", st)
	}
	if !f.sentText("database backup manual") || !f.sentText("旅行计划") || f.sentText("数据库密码") {
		t.Fatal("wrong files sent")
	}
	hits := f.search(t, "证书续不上")
	if len(hits) == 0 || hits[0].Title != "caddy.md" || hits[0].Source != "drive" || !strings.HasPrefix(hits[0].Link, "/drive?folder=") {
		t.Fatalf("search: %+v", hits)
	}

	// Unchanged files are not read or sent again; the unreadable one is not retried.
	f.ai.reset()
	f.sync()
	if n := f.ai.embedded(); n != 0 {
		t.Fatalf("second sync embedded %d", n)
	}
	if st := f.status(t); st.Unreadable == nil || *st.Unreadable != 1 {
		t.Fatalf("unreadable after second sync: %+v", st)
	}

	// While the drive is hidden and locked, its passages are not returned.
	f.hide(t, "drive")
	if hits := f.search(t, "证书续不上"); len(hits) != 0 {
		t.Errorf("hidden drive searched: %s", titles(hits))
	}
	f.hide(t)

	// No folders: the drive leaves the index.
	f.useFolders(t)
	f.sync()
	if st := f.status(t); st.Docs != 0 {
		t.Fatalf("after removing folders: %+v", st)
	}
}
