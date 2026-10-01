package notes_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/modules/notes/api"
	"github.com/j0x3n/x-console/backend/internal/server/testutil"
)

func TestNoteColorValidationAndPersistence(t *testing.T) {
	env := testutil.New(t)
	for _, kind := range []api.NoteKind{api.NoteKindNote, api.NoteKindMemo} {
		for _, color := range []api.NoteColor{"", "red", "orange", "yellow", "green", "teal", "blue", "purple", "pink", "brown", "gray"} {
			n := createNote(t, env, api.CreateNote{Title: str("颜色"), Kind: &kind, Color: &color})
			if n.Color == nil || *n.Color != color {
				t.Fatalf("create %s: %+v", color, n.Color)
			}
			var updated api.Note
			env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]string{"color": "blue"}, &updated)
			if updated.Color == nil || *updated.Color != "blue" {
				t.Fatalf("patch: %+v", updated.Color)
			}
			env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]string{"color": ""}, nil)
			var read api.Note
			env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &read)
			if read.Color == nil || *read.Color != "" {
				t.Fatalf("reset: %+v", read.Color)
			}
		}
	}
	for _, color := range []string{"#ffffff", "RED", " red ", "cyan", "<script>", "default"} {
		if code, _ := env.Do(http.MethodPost, "/notes", map[string]string{"color": color}, nil); code != 400 {
			t.Fatalf("invalid create %q: %d", color, code)
		}
		n := createNote(t, env, api.CreateNote{Color: ptr(api.NoteColor("red"))})
		if code, _ := env.Do(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]string{"color": color}, nil); code != 400 {
			t.Fatalf("invalid patch %q: %d", color, code)
		}
		var got api.Note
		env.MustDo(http.MethodGet, fmt.Sprintf("/notes/%d", n.Id), nil, &got)
		if got.Color == nil || *got.Color != "red" {
			t.Fatal("invalid color changed note")
		}
	}
}

func TestPublicNoteColor(t *testing.T) {
	env := testutil.New(t)
	n := createNote(t, env, api.CreateNote{Body: str("公开颜色"), Color: ptr(api.NoteColor("purple"))})
	share := noteShare(t, env, n.Id, "")
	for _, color := range []string{"purple", ""} {
		env.MustDo(http.MethodPatch, fmt.Sprintf("/notes/%d", n.Id), map[string]string{"color": color}, nil)
		resp, raw := publicNoteRequest(t, env, http.MethodGet, "/public/notes/"+share.Token, nil, nil)
		var out api.PublicNote
		if err := json.Unmarshal(raw, &out); err != nil || resp.StatusCode != 200 || out.Color == nil || string(*out.Color) != color {
			t.Fatalf("public color: %d %s", resp.StatusCode, raw)
		}
	}
}
