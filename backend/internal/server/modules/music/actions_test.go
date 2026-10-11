package music_test

import (
	"context"
	"encoding/json"
	"testing"

	"go.senan.xyz/taglib"
)

func TestMusicActions(t *testing.T) {
	env, m := setup(t)
	act := func(name string, in any) (map[string]any, error) {
		raw, _ := json.Marshal(in)
		res, err := env.App.Deps.Actions.Run(context.Background(), name, raw)
		if err != nil {
			return nil, err
		}
		b, _ := json.Marshal(res)
		var out map[string]any
		_ = json.Unmarshal(b, &out)
		return out, nil
	}
	root := mkdir(t, env, 0, "音乐")
	useFolders(t, env, root)
	var ids []int64
	for _, title := range []string{"Alpha", "Beta", "Gamma"} {
		upload(t, env, root, title+".mp3", song(t, "mp3", map[string][]string{taglib.Title: {title}, taglib.Artist: {"Ann"}, taglib.Album: {"One"}}, nil))
	}
	reconcile(t, m)
	// A song outside the chosen folders is not in the library.
	other := mkdir(t, env, 0, "其他")
	upload(t, env, other, "Secret.mp3", song(t, "mp3", map[string][]string{taglib.Title: {"Secret"}, taglib.Artist: {"Ann"}}, nil))
	reconcile(t, m)

	found, err := act("music.search", map[string]any{"q": "Alpha"})
	if err != nil {
		t.Fatal(err)
	}
	items, _ := found["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["title"] != "Alpha" {
		t.Fatalf("search: %+v", found)
	}
	all, _ := act("music.search", map[string]any{"artist": "Ann", "limit": 2})
	if got := len(all["items"].([]any)); got != 2 || all["total"] != float64(3) {
		t.Fatalf("limit: %+v", all)
	}
	for _, it := range all["items"].([]any) {
		ids = append(ids, int64(it.(map[string]any)["id"].(float64)))
	}

	list, err := act("music.playlist_create", map[string]any{"name": "AI 选的", "trackIds": ids[:1]})
	if err != nil {
		t.Fatal(err)
	}
	pid := int64(list["id"].(float64))
	added, err := act("music.playlist_add", map[string]any{"playlistId": pid, "trackIds": ids})
	if err != nil {
		t.Fatal(err)
	}
	if tr, _ := added["tracks"].([]any); len(tr) != 2 {
		t.Fatalf("duplicates should be skipped: %+v", added)
	}
	if _, err := act("music.playlist_add", map[string]any{"playlistId": 9999, "trackIds": ids}); err == nil {
		t.Fatal("unknown playlist should fail")
	}
	if _, err := act("music.playlist_create", map[string]any{"name": "  "}); err == nil {
		t.Fatal("empty name should fail")
	}

	if res, err := act("music.scan", map[string]any{}); err != nil || res["running"] != true {
		t.Fatalf("scan: %+v %v", res, err)
	}
	if res, err := act("music.match", map[string]any{"retryFailed": true}); err != nil || res["running"] != true {
		t.Fatalf("match: %+v %v", res, err)
	}
	if _, err := act("music.match", map[string]any{"trackId": 9999}); err == nil {
		t.Fatal("unknown track should fail")
	}
	if _, err := act("music.pending", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err := act("music.search", map[string]any{"bogus": 1}); err == nil {
		t.Fatal("unknown field should fail")
	}
}
