package music_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/agent/conn"
	"github.com/j0x3n/x-console/backend/internal/server/modules/music/api"
	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type sourceTransport func(*http.Request) (*http.Response, error)

func (f sourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func routeEnv(t *testing.T, status int, body string) (*matchEnv, *atomic.Int32) {
	e := setupMatch(t)
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"autoMatch": false, "writeBack": false}, nil)
	e.m.SetSourceURLs(map[string]string{"lrclib": "https://lrclib.net", "netease": "https://music.163.com", "qqmusic": "https://c.y.qq.com", "itunes": "https://itunes.apple.com", "musicbrainz": "https://musicbrainz.org", "coverart": "https://coverartarchive.org", "qqcover": "https://y.gtimg.cn"})
	calls := new(atomic.Int32)
	e.m.SetSourceHTTPClient(&http.Client{Timeout: time.Second, Transport: sourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "lrclib.net" {
			calls.Add(1)
			if status == 0 {
				return nil, errors.New("fake network failure")
			}
			if body != "" {
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(stringsReader(body)), Request: r}, nil
			}
			if status != 200 {
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(stringsReader("")), Request: r}, nil
			}
		}
		clone := r.Clone(r.Context())
		target, _ := url.Parse(e.src.srv.URL)
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})})
	return e, calls
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }

func fetchAgent(t *testing.T, e *matchEnv, kind string, calls *atomic.Int32, status int, body string) string {
	t.Helper()
	return e.Agent(kind, []string{protocol.CapNetFetch}, func(c *conn.Client) {
		c.Handle(protocol.MethodNetFetch, func(ctx context.Context, raw json.RawMessage) (any, error) {
			calls.Add(1)
			var p protocol.NetFetchParams
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, err
			}
			if err := p.Validate(); err != nil {
				t.Errorf("invalid proxy params: %+v %v", p, err)
				return nil, err
			}
			if status == 0 {
				return nil, errors.New("fake agent failure")
			}
			return protocol.NetFetchResult{Status: status, ContentType: "application/json", Body: []byte(body)}, nil
		})
	})
}

func TestProviderFallbackTriggersAndPreservesDirectErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		fallback bool
	}{
		{"network", 0, "", true}, {"403", 403, "", true}, {"451", 451, "", true}, {"429", 429, "", true}, {"500", 500, "", true},
		{"404", 404, "", false}, {"401", 401, "", false}, {"invalid-json", 200, "<html>blocked</html>", true}, {"invalid-shape", 200, "{}", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, direct := routeEnv(t, tc.status, tc.body)
			agent := new(atomic.Int32)
			fetchAgent(t, e, "desktop", agent, 200, "[]")
			_, err := e.m.SourceSearch(context.Background(), "lrclib")
			if (err == nil) != tc.fallback || agent.Load() != int32(b2int(tc.fallback)) || direct.Load() != 1 {
				t.Fatalf("direct=%d agent=%d error=%v", direct.Load(), agent.Load(), err)
			}
		})
	}
	for _, kind := range []string{"none", "server", "legacy-windows"} {
		t.Run(kind, func(t *testing.T) {
			e, _ := routeEnv(t, 403, "")
			calls := new(atomic.Int32)
			if kind == "server" {
				fetchAgent(t, e, kind, calls, 200, "[]")
			}
			if kind == "legacy-windows" {
				e.Agent("desktop", nil, func(c *conn.Client) { c.Hello.Capabilities = nil })
			}
			_, err := e.m.SourceSearch(context.Background(), "lrclib")
			if err == nil || err.Error() != "HTTP 403" || calls.Load() != 0 {
				t.Fatalf("direct error changed: %v", err)
			}
		})
	}
}

func b2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestProviderCacheExpiryAndChecksAlwaysTryDirect(t *testing.T) {
	e, direct := routeEnv(t, 403, "")
	calls := new(atomic.Int32)
	fetchAgent(t, e, "desktop", calls, 200, "[]")
	var offset atomic.Int64
	start := time.Now()
	e.m.SetSourceClock(func() time.Time { return start.Add(time.Duration(offset.Load())) })
	for range 2 {
		if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err != nil {
			t.Fatal(err)
		}
	}
	if direct.Load() != 1 || calls.Load() != 2 {
		t.Fatalf("cache: %d %d", direct.Load(), calls.Load())
	}
	var rows []api.MusicSourceCheck
	e.MustDo(http.MethodPost, "/music/sources/check", nil, &rows)
	row := rows[0]
	if !row.Ok || row.Via != "agent" || row.AgentName == nil || *row.AgentName != "test-desktop" || row.DirectError == nil || *row.DirectError != "HTTP 403" || direct.Load() != 2 {
		t.Fatalf("source check: %+v, direct=%d", row, direct.Load())
	}
	offset.Store(int64(11 * time.Minute))
	if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err != nil {
		t.Fatal(err)
	}
	if direct.Load() != 3 {
		t.Fatal("expired cache skipped direct")
	}
}

func TestFailedAgentDoesNotCacheAndFailureIsVisible(t *testing.T) {
	for _, body := range []string{"[]", "{}"} {
		t.Run(body, func(t *testing.T) {
			e, direct := routeEnv(t, 403, "")
			calls := new(atomic.Int32)
			status := 500
			if body == "{}" {
				status = 200
			}
			fetchAgent(t, e, "desktop", calls, status, body)
			for range 2 {
				if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err == nil {
					t.Fatal("failed source succeeded")
				}
			}
			if direct.Load() != 2 {
				t.Fatal("failed agent cached")
			}
			var rows []api.MusicSourceCheck
			e.MustDo(http.MethodPost, "/music/sources/check", nil, &rows)
			if rows[0].Ok || rows[0].Via != "agent" || rows[0].Error == "" || rows[0].DirectError == nil {
				t.Fatalf("failure missing: %+v", rows[0])
			}
		})
	}
}

func TestLatestWindowsHeartbeatWins(t *testing.T) {
	e, _ := routeEnv(t, 403, "")
	calls := new(atomic.Int32)
	var older *conn.Client
	first := e.Agent("desktop", []string{protocol.CapNetFetch}, func(c *conn.Client) { older = c })
	second := fetchAgent(t, e, "desktop", calls, 200, "[]")
	if got := e.m.SelectedFetchAgent(context.Background()); got != second {
		t.Fatalf("newest connection: %s", got)
	}
	if err := older.Emit(context.Background(), protocol.EventMetrics, protocol.MetricsSample{}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return e.m.SelectedFetchAgent(context.Background()) == first })
}

func TestCoverAndLyricsUseWindowsFallback(t *testing.T) {
	e, _ := routeEnv(t, 403, "")
	e.m.SetSourceHTTPClient(&http.Client{Transport: sourceTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(stringsReader("")), Request: r}, nil
	})})
	calls := new(atomic.Int32)
	e.Agent("desktop", []string{protocol.CapNetFetch}, func(c *conn.Client) {
		c.Handle(protocol.MethodNetFetch, func(ctx context.Context, raw json.RawMessage) (any, error) {
			calls.Add(1)
			var p protocol.NetFetchParams
			json.Unmarshal(raw, &p)
			if err := p.Validate(); err != nil {
				return nil, err
			}
			parsed, _ := url.Parse(p.URL)
			target, _ := url.Parse(e.src.srv.URL)
			parsed.Scheme = target.Scheme
			parsed.Host = target.Host
			req, _ := http.NewRequestWithContext(ctx, "GET", parsed.String(), nil)
			resp, err := e.src.srv.Client().Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			return protocol.NetFetchResult{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, err
		})
	})
	e.src.on("/lyric/fcgi-bin/fcg_query_lyric_new.fcg", jsonReply(map[string]string{"lyric": syncedLRC}))
	text, err := e.m.SourceLyrics(context.Background(), "qqmusic", "test")
	if err != nil || text != syncedLRC {
		t.Fatalf("lyrics: %q %v", text, err)
	}
	cover, err := e.m.SourceCover(context.Background(), "qqmusic", "test")
	if err != nil || len(cover) == 0 || calls.Load() != 2 {
		t.Fatalf("cover: %d bytes, %v calls=%d", len(cover), err, calls.Load())
	}
	// A real match request also gets lyrics and the album through the fake agent.
	_, tr := e.add(t, "proxy-match.mp3", idTags, nil)
	e.src.on("/soso/fcgi-bin/client_search_cp", jsonReply(map[string]any{"data": map[string]any{"song": map[string]any{"list": []map[string]any{{"songmid": "test", "songname": "夜曲", "albumname": "代理专辑", "interval": tr.DurationMs / 1000, "singer": []map[string]string{{"name": "周杰伦"}}}}}}}))
	e.MustDo(http.MethodPut, "/music/settings", map[string]any{"providers": map[string]bool{"lrclib": false, "netease": false, "itunes": false, "musicbrainz": false}}, nil)
	var matched api.MusicTrack
	e.MustDo(http.MethodPost, fmt.Sprintf("/music/tracks/%d/match", tr.Id), map[string]any{"parts": []string{"lyrics"}}, &matched)
	if matched.LyricsSource != "online" || matched.Album != "代理专辑" || matched.MatchState != "matched" {
		t.Fatalf("match through agent: %+v", matched)
	}
}

func TestCachedAgentFailureRestoresDirectFirst(t *testing.T) {
	e, direct := routeEnv(t, 403, "")
	var calls, status atomic.Int32
	status.Store(200)
	e.Agent("desktop", []string{protocol.CapNetFetch}, func(c *conn.Client) {
		c.Handle(protocol.MethodNetFetch, func(context.Context, json.RawMessage) (any, error) {
			calls.Add(1)
			return protocol.NetFetchResult{Status: int(status.Load()), Body: []byte("[]")}, nil
		})
	})
	if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err != nil {
		t.Fatal(err)
	}
	status.Store(500)
	if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err == nil {
		t.Fatal("cached agent failure ignored")
	}
	if direct.Load() != 1 {
		t.Fatal("cached request tried direct")
	}
	if _, err := e.m.SourceSearch(context.Background(), "lrclib"); err == nil {
		t.Fatal("failed request succeeded")
	}
	if direct.Load() != 2 || calls.Load() != 3 {
		t.Fatalf("failure did not clear cache: direct=%d agent=%d", direct.Load(), calls.Load())
	}
}
