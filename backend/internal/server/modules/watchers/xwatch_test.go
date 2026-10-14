package watchers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers"
	"github.com/j0x3n/x-console/backend/internal/server/modules/watchers/api"
)

// fakeX answers like the web interface of X, with posts nested the way the
// timelines nest them.
type fakeX struct {
	mu    sync.Mutex
	ready bool
	posts []map[string]any // newest last
	calls []string
	err   error
}

func post(id, handle, text string) map[string]any {
	return map[string]any{"__typename": "Tweet", "rest_id": id,
		"core":   map[string]any{"user_results": map[string]any{"result": map[string]any{"core": map[string]any{"screen_name": handle, "name": handle}}}},
		"legacy": map[string]any{"full_text": text, "created_at": "Wed Oct 14 08:00:00 +0000 2026"}}
}

func (f *fakeX) add(p map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.posts = append(f.posts, p)
}

func (f *fakeX) Ready(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready
}

func (f *fakeX) Query(_ context.Context, queryID, operation, variables, _ string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, queryID+"/"+operation+" "+variables)
	if f.err != nil {
		return nil, f.err
	}
	if operation == "UserByScreenName" {
		return []byte(`{"data":{"user":{"result":{"rest_id":"42"}}}}`), nil
	}
	var entries []any
	for i := len(f.posts) - 1; i >= 0; i-- {
		entries = append(entries, map[string]any{"content": map[string]any{"itemContent": map[string]any{"tweet_results": map[string]any{"result": f.posts[i]}}}})
	}
	doc := map[string]any{"data": map[string]any{"timeline": map[string]any{"instructions": []any{map[string]any{"entries": entries}}}}}
	return json.Marshal(doc)
}

func TestXTargets(t *testing.T) {
	for in, want := range map[string]string{
		"@alice":                                    "user alice https://x.com/alice",
		"https://twitter.com/alice":                 "user alice https://x.com/alice",
		"https://x.com/alice/status/123?s=20":       "replies 123 https://x.com/i/status/123",
		"https://x.com/search?q=%E9%99%8D%E6%81%AF": "search 降息 https://x.com/search?f=live&q=%E9%99%8D%E6%81%AF",
		"美联储 降息":                                    "search 美联储 降息 https://x.com/search?f=live&q=%E7%BE%8E%E8%81%94%E5%82%A8+%E9%99%8D%E6%81%AF",
	} {
		kind, value, u, err := watchers.ParseXTarget(in)
		if got := kind + " " + value + " " + u; err != nil || got != want {
			t.Errorf("%s: %q %v, want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://example.com/alice", "https://x.com/home", ""} {
		if _, _, _, err := watchers.ParseXTarget(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
}

func TestCollectPostsFindsNestedPostsNewestFirst(t *testing.T) {
	wrapped := map[string]any{"__typename": "TweetWithVisibilityResults", "tweet": post("200", "bob", "被包了一层")}
	repost := post("150", "alice", "RT @bob: x")
	repost["legacy"].(map[string]any)["retweeted_status_result"] = map[string]any{}
	doc := map[string]any{"data": map[string]any{"a": []any{
		map[string]any{"tweet_results": map[string]any{"result": post("99", "alice", "旧的")}},
		map[string]any{"deep": map[string]any{"items": []any{map[string]any{"tweet_results": map[string]any{"result": wrapped}}}}},
		map[string]any{"tweet_results": map[string]any{"result": post("1000", "alice", "最新的")}},
		map[string]any{"tweet_results": map[string]any{"result": post("99", "alice", "重复的")}},
	}}}
	raw, _ := json.Marshal(doc)
	got, err := watchers.CollectPosts(raw)
	if err != nil || strings.Join(got, ",") != "1000@alice,200@bob,99@alice" {
		t.Fatalf("got %v %v", got, err)
	}
}

func TestXWatcherNotifiesNewPostsAndFilters(t *testing.T) {
	f := setup(t, false)
	x := &fakeX{ready: true}
	watchers.SetX(f.m, x)
	x.add(post("100", "alice", "第一条"))
	x.add(post("101", "alice", "第二条"))
	var w api.Watcher
	f.env.MustDo(http.MethodPost, "/watchers", map[string]any{"kind": "x", "name": "Alice", "url": "@alice", "goal": ""}, &w)
	if w.Kind != api.X || w.Url != "https://x.com/alice" || w.IntervalMinutes != 30 || w.AllowedHosts[0] != "x.com" {
		t.Fatalf("created %+v", w)
	}
	watchers.WaitRuns(f.m)
	got := f.get(t, w.Id)
	if got.Status != api.WatcherStatusOk || !strings.Contains(got.CurrentValue, "第二条") || len(got.Changes) != 0 || got.HasRecipe {
		t.Fatalf("first run: %s %q %d changes", got.Status, got.CurrentValue, len(got.Changes))
	}
	if f.notified(t, "watcher.x") != 0 {
		t.Fatal("the first run notified old posts")
	}

	// Two new posts, one of them a repost of somebody else: only the own one counts.
	x.add(post("102", "alice", "新的一条 https://t.co/a"))
	rp := post("103", "alice", "RT @bob: 别人的")
	rp["legacy"].(map[string]any)["retweeted_status_result"] = map[string]any{}
	x.add(rp)
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if len(got.Changes) != 1 || !strings.Contains(got.Changes[0].NewValue, "新的一条") || strings.Contains(got.Changes[0].NewValue, "别人的") ||
		!strings.Contains(got.Changes[0].NewValue, "https://x.com/alice/status/102") {
		t.Fatalf("changes %+v", got.Changes)
	}
	if f.notified(t, "watcher.x") != 1 {
		t.Fatal("no notification for the new post")
	}
	// The user id is looked up once and remembered.
	lookups := 0
	for _, c := range x.calls {
		if strings.Contains(c, "UserByScreenName") {
			lookups++
		}
	}
	if lookups != 1 {
		t.Fatalf("user looked up %d times: %v", lookups, x.calls)
	}
	// The save button puts the post into read later.
	action := fmt.Sprintf("watcher.save:%d", got.Changes[0].Id)
	if err := f.env.App.Deps.Notify.HandleAction(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	var saved struct{ Items []struct{ URL string } }
	f.env.MustDo(http.MethodGet, "/readlater", nil, &saved)
	if len(saved.Items) != 1 || saved.Items[0].URL != "https://x.com/alice/status/102" {
		t.Fatalf("read later %+v", saved.Items)
	}

	// With a filter sentence, AI drops the posts that do not match.
	f.env.MustDo(http.MethodPatch, fmt.Sprintf("/watchers/%d", w.Id), map[string]any{"goal": "只要和降息有关的"}, nil)
	x.add(post("104", "alice", "今天天气不错"))
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if len(got.Changes) != 2 || got.Changes[0].Important || f.notified(t, "watcher.x") != 1 {
		t.Fatalf("filtered: %+v", got.Changes[0])
	}

	// X refuses the cookie: the watcher waits for a new cookie.
	x.mu.Lock()
	x.err = contracts.ErrXAuth
	x.mu.Unlock()
	f.env.MustDo(http.MethodPost, fmt.Sprintf("/watchers/%d/run", w.Id), map[string]string{"kind": "check"}, nil)
	watchers.WaitRuns(f.m)
	got = f.get(t, w.Id)
	if got.Status != api.WatcherStatusLogin || !strings.Contains(got.StatusDetail, "Cookie") {
		t.Fatalf("expired cookie: %s %s", got.Status, got.StatusDetail)
	}

	// Settings keep the query ids the user overrides.
	var set api.WatchersSettings
	f.env.MustDo(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 1, "dangerWords": []string{},
		"xQueryIds": map[string]string{"UserTweets": "NEWid_123456", "Nope": "x"}}, &set)
	if (*set.XQueryIds)["UserTweets"] != "NEWid_123456" || (*set.XQueryIds)["SearchTimeline"] == "" || (*set.XQueryIds)["Nope"] != "" {
		t.Fatalf("query ids %+v", *set.XQueryIds)
	}
	if status, _ := f.env.Do(http.MethodPut, "/watchers/settings", map[string]any{"budgetUsd": 1, "dangerWords": []string{},
		"xQueryIds": map[string]string{"UserTweets": "bad id!"}}, nil); status != http.StatusBadRequest {
		t.Fatalf("bad query id: %d", status)
	}
	if status, _ := f.env.Do(http.MethodPost, "/watchers", map[string]any{"kind": "x", "name": "y", "url": "@bob", "goal": "", "password": "p"}, nil); status != http.StatusBadRequest {
		f.env.Elevate()
		if status, _ := f.env.Do(http.MethodPost, "/watchers", map[string]any{"kind": "x", "name": "y", "url": "@bob", "goal": "", "password": "p"}, nil); status != http.StatusBadRequest {
			t.Fatalf("x with a password: %d", status)
		}
	}
}
