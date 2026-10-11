package music

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sources of lyrics and covers (B148). Each one is a small client with a base
// URL that tests replace with a local server. Failures are returned as errors
// and the caller skips that source: one source being down never stops a match.
//
// NetEase and QQ Music have no public API. The calls below are the ones their
// web pages use; they can change or stop working at any time.

const userAgent = "X-Console/1 (+https://github.com/j0x3n/x-console)"

// Source names, as stored and shown.
const (
	SourceLRCLIB      = "lrclib"
	SourceNetease     = "netease"
	SourceQQ          = "qqmusic"
	SourceITunes      = "itunes"
	SourceMusicBrainz = "musicbrainz"
)

// AllSources lists the sources in the order they are tried.
var AllSources = []string{SourceLRCLIB, SourceNetease, SourceQQ, SourceITunes, SourceMusicBrainz}

// query is what is known about the song being matched.
type query struct {
	Title, Artist, Album string
	DurationMs           int
}

func (q query) keywords() string { return strings.TrimSpace(q.Title + " " + q.Artist) }

// candidate is one search hit.
type candidate struct {
	Source, SourceID     string
	Title, Artist, Album string
	DurationMs           int // 0 when the source does not say
	Lyrics, Cover        bool
	// Inline lyrics, for sources that return them with the search hit.
	LyricsText string
	// Source specific data needed to fetch the cover.
	coverRef string
}

type provider interface {
	Name() string
	Search(ctx context.Context, q query) ([]candidate, error)
	// FetchLyrics returns LRC or plain text. Only called when c.Lyrics.
	FetchLyrics(ctx context.Context, c candidate) (string, error)
	// FetchCover returns the image bytes. Only called when c.Cover.
	FetchCover(ctx context.Context, c candidate) ([]byte, error)
}

// providerSet holds the clients and the base URLs they use.
type providerSet struct {
	client *http.Client
	urls   map[string]string

	mbMu       sync.Mutex
	mbLast     time.Time
	mbInterval time.Duration
}

func newProviderSet() *providerSet {
	return &providerSet{
		client: &http.Client{Timeout: 8 * time.Second},
		urls: map[string]string{
			SourceLRCLIB:      "https://lrclib.net",
			SourceNetease:     "https://music.163.com",
			SourceQQ:          "https://c.y.qq.com",
			SourceITunes:      "https://itunes.apple.com",
			SourceMusicBrainz: "https://musicbrainz.org",
			"coverart":        "https://coverartarchive.org",
			"qqcover":         "https://y.gtimg.cn",
		},
		mbInterval: 1100 * time.Millisecond,
	}
}

func (s *providerSet) get(name string) provider {
	switch name {
	case SourceLRCLIB:
		return lrclibProvider{s}
	case SourceNetease:
		return neteaseProvider{s}
	case SourceQQ:
		return qqProvider{s}
	case SourceITunes:
		return itunesProvider{s}
	case SourceMusicBrainz:
		return musicbrainzProvider{s}
	}
	return nil
}

const maxBody = 8 << 20

// do sends a GET and returns the body of a 200 response.
func (s *providerSet) do(ctx context.Context, rawURL string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

func (s *providerSet) getJSON(ctx context.Context, rawURL string, headers map[string]string, out any) error {
	body, err := s.do(ctx, rawURL, headers)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// ---- LRCLIB ----

type lrclibProvider struct{ s *providerSet }

func (lrclibProvider) Name() string { return SourceLRCLIB }

func (p lrclibProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	v := url.Values{"track_name": {q.Title}}
	if q.Artist != "" {
		v.Set("artist_name", q.Artist)
	}
	var hits []struct {
		ID           int64   `json:"id"`
		TrackName    string  `json:"trackName"`
		ArtistName   string  `json:"artistName"`
		AlbumName    string  `json:"albumName"`
		Duration     float64 `json:"duration"`
		Instrumental bool    `json:"instrumental"`
		Plain        string  `json:"plainLyrics"`
		Synced       string  `json:"syncedLyrics"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceLRCLIB]+"/api/search?"+v.Encode(), nil, &hits); err != nil {
		return nil, err
	}
	var out []candidate
	for _, h := range hits {
		text := h.Synced
		if text == "" {
			text = h.Plain
		}
		if text == "" || h.Instrumental {
			continue
		}
		out = append(out, candidate{Source: SourceLRCLIB, SourceID: strconv.FormatInt(h.ID, 10), Title: h.TrackName, Artist: h.ArtistName,
			Album: h.AlbumName, DurationMs: int(h.Duration * 1000), Lyrics: true, LyricsText: text})
	}
	return out, nil
}

func (lrclibProvider) FetchLyrics(_ context.Context, c candidate) (string, error) {
	if c.LyricsText == "" {
		return "", errors.New("no lyrics in the search hit")
	}
	return c.LyricsText, nil
}

func (lrclibProvider) FetchCover(context.Context, candidate) ([]byte, error) {
	return nil, errors.New("lrclib has no covers")
}

// ---- NetEase Cloud Music ----

type neteaseProvider struct{ s *providerSet }

func (neteaseProvider) Name() string { return SourceNetease }

func (p neteaseProvider) headers() map[string]string {
	return map[string]string{"Referer": "https://music.163.com/", "Cookie": "appver=2.0.2"}
}

func (p neteaseProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	v := url.Values{"s": {q.keywords()}, "type": {"1"}, "offset": {"0"}, "total": {"true"}, "limit": {"10"}}
	var res struct {
		Result struct {
			Songs []struct {
				ID       int64  `json:"id"`
				Name     string `json:"name"`
				Duration int    `json:"duration"`
				Artists  []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name string `json:"name"`
				} `json:"album"`
			} `json:"songs"`
		} `json:"result"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceNetease]+"/api/search/get/web?"+v.Encode(), p.headers(), &res); err != nil {
		return nil, err
	}
	var out []candidate
	for _, s := range res.Result.Songs {
		names := make([]string, 0, len(s.Artists))
		for _, a := range s.Artists {
			names = append(names, a.Name)
		}
		out = append(out, candidate{Source: SourceNetease, SourceID: strconv.FormatInt(s.ID, 10), Title: s.Name, Artist: strings.Join(names, "/"),
			Album: s.Album.Name, DurationMs: s.Duration, Lyrics: true, Cover: true})
	}
	return out, nil
}

func (p neteaseProvider) FetchLyrics(ctx context.Context, c candidate) (string, error) {
	var res struct {
		Lrc struct {
			Lyric string `json:"lyric"`
		} `json:"lrc"`
	}
	u := p.s.urls[SourceNetease] + "/api/song/lyric?lv=1&kv=1&tv=-1&id=" + url.QueryEscape(c.SourceID)
	if err := p.s.getJSON(ctx, u, p.headers(), &res); err != nil {
		return "", err
	}
	if strings.TrimSpace(res.Lrc.Lyric) == "" {
		return "", errors.New("no lyrics")
	}
	return res.Lrc.Lyric, nil
}

func (p neteaseProvider) FetchCover(ctx context.Context, c candidate) ([]byte, error) {
	var res struct {
		Songs []struct {
			Album struct {
				PicURL string `json:"picUrl"`
			} `json:"album"`
		} `json:"songs"`
	}
	id := url.QueryEscape(c.SourceID)
	u := p.s.urls[SourceNetease] + "/api/song/detail/?id=" + id + "&ids=%5B" + id + "%5D"
	if err := p.s.getJSON(ctx, u, p.headers(), &res); err != nil {
		return nil, err
	}
	if len(res.Songs) == 0 || res.Songs[0].Album.PicURL == "" {
		return nil, errors.New("no cover")
	}
	pic := res.Songs[0].Album.PicURL
	if strings.HasPrefix(pic, "http://") && !strings.HasPrefix(p.s.urls[SourceNetease], "http://127.") {
		pic = "https://" + strings.TrimPrefix(pic, "http://")
	}
	return p.s.do(ctx, pic+"?param=1200y1200", nil)
}

// ---- QQ Music ----

type qqProvider struct{ s *providerSet }

func (qqProvider) Name() string { return SourceQQ }

func (qqProvider) headers() map[string]string {
	return map[string]string{"Referer": "https://y.qq.com/"}
}

func (p qqProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	v := url.Values{"w": {q.keywords()}, "format": {"json"}, "p": {"1"}, "n": {"10"}, "cr": {"1"}, "t": {"0"}}
	var res struct {
		Data struct {
			Song struct {
				List []struct {
					SongMID   string `json:"songmid"`
					SongName  string `json:"songname"`
					AlbumName string `json:"albumname"`
					AlbumMID  string `json:"albummid"`
					Interval  int    `json:"interval"`
					Singer    []struct {
						Name string `json:"name"`
					} `json:"singer"`
				} `json:"list"`
			} `json:"song"`
		} `json:"data"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceQQ]+"/soso/fcgi-bin/client_search_cp?"+v.Encode(), p.headers(), &res); err != nil {
		return nil, err
	}
	var out []candidate
	for _, s := range res.Data.Song.List {
		names := make([]string, 0, len(s.Singer))
		for _, a := range s.Singer {
			names = append(names, a.Name)
		}
		out = append(out, candidate{Source: SourceQQ, SourceID: s.SongMID, Title: s.SongName, Artist: strings.Join(names, "/"),
			Album: s.AlbumName, DurationMs: s.Interval * 1000, Lyrics: true, Cover: s.AlbumMID != "", coverRef: s.AlbumMID})
	}
	return out, nil
}

var htmlEntity = regexp.MustCompile(`&#?\w+;`)

func (p qqProvider) FetchLyrics(ctx context.Context, c candidate) (string, error) {
	v := url.Values{"songmid": {c.SourceID}, "format": {"json"}, "nobase64": {"1"}}
	var res struct {
		Lyric string `json:"lyric"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceQQ]+"/lyric/fcgi-bin/fcg_query_lyric_new.fcg?"+v.Encode(), p.headers(), &res); err != nil {
		return "", err
	}
	// The service escapes characters such as ":" as HTML entities.
	text := htmlEntity.ReplaceAllStringFunc(res.Lyric, html.UnescapeString)
	if strings.TrimSpace(text) == "" {
		return "", errors.New("no lyrics")
	}
	return text, nil
}

func (p qqProvider) FetchCover(ctx context.Context, c candidate) ([]byte, error) {
	if c.coverRef == "" {
		return nil, errors.New("no cover")
	}
	return p.s.do(ctx, p.s.urls["qqcover"]+"/music/photo_new/T002R800x800M000"+url.PathEscape(c.coverRef)+".jpg", nil)
}

// ---- iTunes Search ----

type itunesProvider struct{ s *providerSet }

func (itunesProvider) Name() string { return SourceITunes }

func (p itunesProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	v := url.Values{"term": {q.keywords()}, "entity": {"song"}, "limit": {"10"}}
	var res struct {
		Results []struct {
			TrackID    int64  `json:"trackId"`
			TrackName  string `json:"trackName"`
			ArtistName string `json:"artistName"`
			Collection string `json:"collectionName"`
			Millis     int    `json:"trackTimeMillis"`
			Artwork    string `json:"artworkUrl100"`
		} `json:"results"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceITunes]+"/search?"+v.Encode(), nil, &res); err != nil {
		return nil, err
	}
	var out []candidate
	for _, r := range res.Results {
		if r.Artwork == "" {
			continue
		}
		out = append(out, candidate{Source: SourceITunes, SourceID: strconv.FormatInt(r.TrackID, 10), Title: r.TrackName, Artist: r.ArtistName,
			Album: r.Collection, DurationMs: r.Millis, Cover: true, coverRef: r.Artwork})
	}
	return out, nil
}

func (itunesProvider) FetchLyrics(context.Context, candidate) (string, error) {
	return "", errors.New("itunes has no lyrics")
}

func (p itunesProvider) FetchCover(ctx context.Context, c candidate) ([]byte, error) {
	big := strings.Replace(c.coverRef, "100x100bb", "1000x1000bb", 1)
	return p.s.do(ctx, big, nil)
}

// ---- MusicBrainz and the Cover Art Archive ----

type musicbrainzProvider struct{ s *providerSet }

func (musicbrainzProvider) Name() string { return SourceMusicBrainz }

// wait keeps MusicBrainz to one request a second, as its rules ask.
func (p musicbrainzProvider) wait(ctx context.Context) error {
	p.s.mbMu.Lock()
	defer p.s.mbMu.Unlock()
	if d := p.s.mbInterval - time.Since(p.s.mbLast); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	p.s.mbLast = time.Now()
	return nil
}

func (p musicbrainzProvider) Search(ctx context.Context, q query) ([]candidate, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	esc := func(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }
	expr := `recording:"` + esc(q.Title) + `"`
	if q.Artist != "" {
		expr += ` AND artist:"` + esc(q.Artist) + `"`
	}
	v := url.Values{"query": {expr}, "fmt": {"json"}, "limit": {"5"}}
	var res struct {
		Recordings []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Length       int    `json:"length"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Releases []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"releases"`
		} `json:"recordings"`
	}
	if err := p.s.getJSON(ctx, p.s.urls[SourceMusicBrainz]+"/ws/2/recording?"+v.Encode(), nil, &res); err != nil {
		return nil, err
	}
	var out []candidate
	for _, r := range res.Recordings {
		if len(r.Releases) == 0 {
			continue
		}
		names := make([]string, 0, len(r.ArtistCredit))
		for _, a := range r.ArtistCredit {
			names = append(names, a.Name)
		}
		out = append(out, candidate{Source: SourceMusicBrainz, SourceID: r.ID, Title: r.Title, Artist: strings.Join(names, "/"),
			Album: r.Releases[0].Title, DurationMs: r.Length, Cover: true, coverRef: r.Releases[0].ID})
	}
	return out, nil
}

func (musicbrainzProvider) FetchLyrics(context.Context, candidate) (string, error) {
	return "", errors.New("musicbrainz has no lyrics")
}

func (p musicbrainzProvider) FetchCover(ctx context.Context, c candidate) ([]byte, error) {
	return p.s.do(ctx, p.s.urls["coverart"]+"/release/"+url.PathEscape(c.coverRef)+"/front-1200", nil)
}
