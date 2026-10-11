package readlater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type youtubeTransport struct {
	base string
	next http.RoundTripper
}

func (tr youtubeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	u := *r.URL
	base, _ := url.Parse(tr.base)
	u.Scheme, u.Host = base.Scheme, base.Host
	clone.URL = &u
	return tr.next.RoundTrip(clone)
}

func TestYouTubeURLs(t *testing.T) {
	for _, raw := range []string{"https://youtu.be/M7lc1UVf-VE", "https://www.youtube.com/watch?v=M7lc1UVf-VE&t=2", "https://m.youtube.com/shorts/M7lc1UVf-VE", "https://www.youtube.com/embed/M7lc1UVf-VE"} {
		u, _ := url.Parse(raw)
		if id, ok := youtubeID(u); !ok || id != "M7lc1UVf-VE" {
			t.Fatal(raw, id, ok)
		}
	}
	for _, raw := range []string{"https://youtube.com.evil.test/watch?v=M7lc1UVf-VE", "https://youtube.com/watch?v=bad", "https://youtube.com/channel/M7lc1UVf-VE"} {
		u, _ := url.Parse(raw)
		if _, ok := youtubeID(u); ok {
			t.Fatal(raw)
		}
	}
}

func TestYouTubeFetchAndFallback(t *testing.T) {
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "player", true: "oembed"}[fallback], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/watch":
					if fallback {
						w.WriteHeader(403)
						return
					}
					w.Write([]byte(`<script>var ytInitialPlayerResponse = {"videoDetails":{"title":"视频 {标题}","author":"作者","shortDescription":"第一行\n第二行"},"captions":{"playerCaptionsTracklistRenderer":{"captionTracks":[{"baseUrl":"https://www.youtube.com/api/timedtext?v=abc","languageCode":"zh"}]}}};</script>`))
				case "/oembed":
					w.Write([]byte(`{"title":"公开视频","author_name":"频道","thumbnail_url":"https://i.ytimg.com/vi/M7lc1UVf-VE/hqdefault.jpg"}`))
				case "/api/timedtext":
					w.Write([]byte(`<transcript><text>第一段 &amp; 内容</text><text>第二段</text></transcript>`))
				default:
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			client := newFetchClient(true)
			client.Transport = youtubeTransport{srv.URL, client.Transport}
			m := &Module{client: client}
			pg, err := m.fetchPage(context.Background(), "https://youtu.be/M7lc1UVf-VE")
			if err != nil {
				t.Fatal(err)
			}
			if pg.Site != "YouTube" || pg.Title == "" || pg.Content == "" {
				t.Fatalf("%+v", pg)
			}
			if !fallback && !strings.Contains(pg.Content, "第一段 & 内容") {
				t.Fatal(pg.Content)
			}
			if fallback && strings.Contains(pg.Content, "字幕") {
				t.Fatal("invented transcript")
			}
		})
	}
}
