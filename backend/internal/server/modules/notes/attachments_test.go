package notes_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func uploadFile(t *testing.T, env *testutil.Env, noteID int64, name string, content []byte) (int, api.Attachment) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, env.URL(fmt.Sprintf("/notes/%d/attachments", noteID)), &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Requested-With", "x-console")
	resp, err := env.Client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out api.Attachment
	if resp.StatusCode == http.StatusCreated {
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, out
}

func TestAttachmentsPersistAndDeleteWithNote(t *testing.T) {
	env := testutil.New(t)
	note := createNote(t, env, api.CreateNote{Title: str("图片")})
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	status, image := uploadFile(t, env, note.Id, "photo.png", png)
	if status != 201 || image.NoteId != note.Id || image.Mime != "image/png" || image.Size != int64(len(png)) {
		t.Fatalf("upload: %d %+v", status, image)
	}
	_, text := uploadFile(t, env, note.Id, "report.txt", []byte("hello world"))
	var listed []api.Attachment
	env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d/attachments", note.Id), nil, &listed)
	if len(listed) != 2 || listed[0].Id != text.Id || listed[1].Id != image.Id {
		t.Fatalf("list: %+v", listed)
	}
	body := "![photo](" + image.Url + ")"
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", note.Id), api.UpdateNote{Body: &body}, nil)
	page := search(t, env, "")
	if len(page.Items) != 1 || page.Items[0].Thumbnail == nil || *page.Items[0].Thumbnail != image.Url {
		t.Fatalf("thumbnail: %+v", page.Items)
	}
	resp, err := env.Client.Get(env.Server.URL + image.Url)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 200 || !bytes.Equal(got, png) || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "inline") || resp.Header.Get("Cache-Control") != "private, max-age=31536000" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("download: %d %q %v", resp.StatusCode, resp.Header.Get("Content-Disposition"), err)
	}
	public, err := http.Get(env.Server.URL + image.Url)
	if err != nil {
		t.Fatal(err)
	}
	public.Body.Close()
	if public.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous download: %d", public.StatusCode)
	}
	file := filepath.Join(env.App.Deps.Config.FilesDir(), "notes", "attachments", fmt.Sprint(image.Id))
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notes/%d", note.Id), nil, nil)
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("file after note deletion: %v", err)
	}
	var remaining int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM note_attachments WHERE note_id = ?`, note.Id).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("attachment rows after note deletion: %d %v", remaining, err)
	}
	if code, _ := env.Do(http.MethodGet, fmt.Sprintf("/notes/attachments/%d", image.Id), nil, nil); code != 404 {
		t.Fatalf("deleted download: %d", code)
	}
}

func TestAttachmentValidationAndIndividualDeletion(t *testing.T) {
	env := testutil.New(t)
	note := createNote(t, env, api.CreateNote{Title: str("附件")})
	if code, _ := uploadFile(t, env, 999, "missing.png", []byte("x")); code != 404 {
		t.Fatalf("missing note: %d", code)
	}
	if code, _ := env.Do(http.MethodGet, "/notes/999/attachments", nil, nil); code != 404 {
		t.Fatalf("list missing: %d", code)
	}
	if code, _ := uploadFile(t, env, note.Id, "large.bin", bytes.Repeat([]byte("a"), (50<<20)+1)); code != 413 {
		t.Fatalf("oversize: %d", code)
	}
	_, image := uploadFile(t, env, note.Id, "fake.svg", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"))
	resp, err := env.Client.Get(env.Server.URL + image.Url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("svg disposition: %s", resp.Header.Get("Content-Disposition"))
	}
	env.MustDo(http.MethodDelete, fmt.Sprintf("/notes/attachments/%d", image.Id), nil, nil)
	if code, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notes/attachments/%d", image.Id), nil, nil); code != 404 {
		t.Fatalf("delete twice: %d", code)
	}
}

func TestFailedDeleteKeepsAttachmentFiles(t *testing.T) {
	env := testutil.New(t)
	note := createNote(t, env, api.CreateNote{Title: str("图片")})
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	_, image := uploadFile(t, env, note.Id, "photo.png", png)
	fetch := func() int {
		resp, err := env.Client.Get(env.Server.URL + image.Url)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// Make the database step fail. The files must stay, or the note shows
	// a broken image.
	if _, err := env.App.Deps.DB.Exec(`CREATE TRIGGER keep_rows BEFORE DELETE ON note_attachments BEGIN SELECT RAISE(ABORT, 'no'); END`); err != nil {
		t.Fatal(err)
	}
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notes/attachments/%d", image.Id), nil, nil); status < 500 {
		t.Fatalf("attachment delete: %d", status)
	}
	if status, _ := env.Do(http.MethodDelete, fmt.Sprintf("/notes/%d", note.Id), nil, nil); status < 500 {
		t.Fatalf("note delete: %d", status)
	}
	if status := fetch(); status != http.StatusOK {
		t.Fatalf("attachment after failed deletes: %d", status)
	}
}

func TestThumbnailsForManyNotes(t *testing.T) {
	env := testutil.New(t)
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 100)...)
	first := createNote(t, env, api.CreateNote{Title: str("甲")})
	_, text := uploadFile(t, env, first.Id, "a.txt", []byte("hello"))
	_, image := uploadFile(t, env, first.Id, "a.png", png)
	// A text file linked as an image is skipped; the real image is used.
	body := "![x](" + text.Url + ") ![y](" + image.Url + ")"
	env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", first.Id), api.UpdateNote{Body: &body}, nil)
	// Another note linking the first note's image does not get it.
	borrowed := "![z](" + image.Url + ")"
	createNote(t, env, api.CreateNote{Title: str("乙"), Body: &borrowed})
	page := search(t, env, "")
	thumbs := map[string]*string{}
	for _, item := range page.Items {
		thumbs[item.Title] = item.Thumbnail
	}
	if thumbs["甲"] == nil || *thumbs["甲"] != image.Url || thumbs["乙"] != nil {
		t.Fatalf("thumbnails: 甲=%v 乙=%v", thumbs["甲"], thumbs["乙"])
	}
}
