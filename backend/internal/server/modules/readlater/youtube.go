package readlater

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var youtubeVideoID = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)

func youtubeID(u *url.URL) (string, bool) {
	host := strings.ToLower(u.Hostname())
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "www.youtube.com", "m.youtube.com", "music.youtube.com", "www.youtube-nocookie.com":
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if u.Path == "/watch" {
			id = u.Query().Get("v")
		} else if len(parts) == 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
			id = parts[1]
		}
	}
	return id, youtubeVideoID.MatchString(id)
}

type youtubePlayer struct {
	VideoDetails struct {
		Title       string `json:"title"`
		Author      string `json:"author"`
		Description string `json:"shortDescription"`
		Thumbnail   struct {
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
		} `json:"thumbnail"`
	} `json:"videoDetails"`
	Captions struct {
		Tracks struct {
			Tracks []struct {
				URL      string `json:"baseUrl"`
				Language string `json:"languageCode"`
			} `json:"captionTracks"`
		} `json:"playerCaptionsTracklistRenderer"`
	} `json:"captions"`
}

// Decode JSON rather than matching braces: descriptions can contain braces and escapes.
func youtubePlayerData(body []byte) youtubePlayer {
	for _, marker := range []string{"ytInitialPlayerResponse =", "ytInitialPlayerResponse=", `"ytInitialPlayerResponse":`} {
		if pos := bytes.Index(body, []byte(marker)); pos >= 0 {
			var out youtubePlayer
			if json.NewDecoder(bytes.NewReader(bytes.TrimSpace(body[pos+len(marker):]))).Decode(&out) == nil {
				return out
			}
		}
	}
	return youtubePlayer{}
}

func (m *Module) youtubeGet(ctx context.Context, raw string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 X-Console-ReadLater/1.0")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	m.mu.Lock()
	client := m.client
	m.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New(fetchError(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("YouTube 内容读取失败，状态码 %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if len(body) > maxBody {
		return nil, errors.New("YouTube 内容超过大小限制")
	}
	return body, err
}

func (m *Module) fetchYouTube(ctx context.Context, id string) (page, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	watch := "https://www.youtube.com/watch?v=" + id
	body, watchErr := m.youtubeGet(ctx, watch+"&hl=zh-CN")
	player := youtubePlayerData(body)
	title, author, description := player.VideoDetails.Title, player.VideoDetails.Author, player.VideoDetails.Description
	image := ""
	if thumbs := player.VideoDetails.Thumbnail.Thumbnails; len(thumbs) > 0 {
		image = thumbs[len(thumbs)-1].URL
	}
	if title == "" {
		data, err := m.youtubeGet(ctx, "https://www.youtube.com/oembed?format=json&url="+url.QueryEscape(watch))
		if err != nil {
			if watchErr != nil {
				return page{}, watchErr
			}
			return page{}, err
		}
		var embed struct {
			Title  string `json:"title"`
			Author string `json:"author_name"`
			Image  string `json:"thumbnail_url"`
		}
		if err = json.Unmarshal(data, &embed); err != nil || embed.Title == "" {
			return page{}, errors.New("YouTube 没有返回视频信息")
		}
		title, author, image = embed.Title, embed.Author, embed.Image
	}
	transcript := ""
	tracks := player.Captions.Tracks.Tracks
	if len(tracks) > 0 {
		track := tracks[0]
		for _, candidate := range tracks {
			if strings.HasPrefix(candidate.Language, "zh") {
				track = candidate
				break
			}
		}
		// The normal fetch client validates the resolved address, including redirects.
		if u, err := url.Parse(track.URL); err == nil && u.Scheme == "https" && (u.Hostname() == "www.youtube.com" || u.Hostname() == "www.youtube-nocookie.com") {
			query := u.Query()
			query.Set("fmt", "json3")
			u.RawQuery = query.Encode()
			if raw, err := m.youtubeGet(ctx, u.String()); err == nil {
				transcript = youtubeCaptions(raw)
			}
		}
	}
	content := strings.TrimSpace(author + "\n\n" + description)
	article := "<p>" + html.EscapeString(author) + "</p>"
	if image != "" {
		article += `<figure><img src="` + html.EscapeString(image) + `" alt="` + html.EscapeString(title) + `"></figure>`
	}
	if description != "" {
		article += "<p>" + strings.ReplaceAll(html.EscapeString(description), "\n", "<br>") + "</p>"
	}
	if transcript != "" {
		content += "\n\n字幕\n" + transcript
		article += "<h2>字幕</h2><p>" + strings.ReplaceAll(html.EscapeString(transcript), "\n", "<br>") + "</p>"
	}
	base, _ := url.Parse(watch)
	return page{Title: clipRunes(title, maxTitle), Site: "YouTube", Excerpt: clipRunes(description, 500), Content: clipRunes(content, maxContent), HTML: article, Base: base, Meta: map[string]any{"youtubeId": id, "author": author, "hasTranscript": transcript != ""}}, nil
}

func youtubeCaptions(raw []byte) string {
	var timed struct {
		Events []struct {
			Segments []struct {
				Text string `json:"utf8"`
			} `json:"segs"`
		} `json:"events"`
	}
	var lines []string
	if json.Unmarshal(raw, &timed) == nil {
		for _, event := range timed.Events {
			var line strings.Builder
			for _, segment := range event.Segments {
				line.WriteString(segment.Text)
			}
			if text := strings.TrimSpace(line.String()); text != "" {
				lines = append(lines, text)
			}
		}
	} else {
		var captions struct {
			Text []string `xml:"text"`
		}
		if xml.Unmarshal(raw, &captions) == nil {
			for _, text := range captions.Text {
				lines = append(lines, html.UnescapeString(text))
			}
		}
	}
	return clipRunes(tidyText(strings.Join(lines, "\n")), maxContent)
}
