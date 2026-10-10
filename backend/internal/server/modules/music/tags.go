package music

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"go.senan.xyz/taglib"
)

// audioMimes are the formats the library accepts, by lower-case extension.
// These are the ones a browser plays without transcoding.
var audioMimes = map[string]string{
	".mp3":  "audio/mpeg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".ogg":  "audio/ogg",
	".opus": "audio/ogg",
	".wav":  "audio/wav",
}

// audioInfo is what a file says about itself.
type audioInfo struct {
	Title, Artist, Album, AlbumArtist string
	TrackNo, DiscNo, Year             int
	DurationMs, Bitrate               int
	Lyrics                            string
	Cover                             []byte
}

// readAudio reads tags, properties, the first embedded image and embedded
// lyrics. A file that cannot be parsed gives an empty result and the error,
// so the caller can still index it by its file name.
func readAudio(path string) (audioInfo, error) {
	var out audioInfo
	tags, err := taglib.ReadTags(path)
	if err != nil {
		return out, err
	}
	first := func(keys ...string) string {
		for _, k := range keys {
			for _, v := range tags[k] {
				if v = cleanTag(v); v != "" {
					return v
				}
			}
		}
		return ""
	}
	out.Title = first(taglib.Title)
	out.Artist = first(taglib.Artist, taglib.Artists)
	out.Album = first(taglib.Album)
	out.AlbumArtist = first(taglib.AlbumArtist)
	out.TrackNo = leadingNumber(first(taglib.TrackNumber))
	out.DiscNo = leadingNumber(first(taglib.DiscNumber))
	out.Year = year(first(taglib.Date, taglib.OriginalDate, taglib.ReleaseDate))
	out.Lyrics = strings.TrimSpace(strings.Join(append(append([]string{}, tags[taglib.Lyrics]...), tags["UNSYNCEDLYRICS"]...), ""))
	if props, perr := taglib.ReadProperties(path); perr == nil {
		out.DurationMs = int(props.Length.Milliseconds())
		out.Bitrate = int(props.BitRate)
	}
	if img, ierr := taglib.ReadImage(path); ierr == nil && len(img) > 0 {
		out.Cover = img
	}
	return out, nil
}

func cleanTag(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == 0 || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return -1
		}
		return r
	}, s))
}

// leadingNumber reads "3", "3/12" and " 03 " as 3. Anything else is 0.
func leadingNumber(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "/\\"); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 || n > 9999 {
		return 0
	}
	return n
}

// year reads the first four digits of a date such as "2004-09-01".
func year(s string) int {
	if len(s) < 4 {
		return 0
	}
	n, err := strconv.Atoi(s[:4])
	if err != nil || n < 1000 || n > 2999 {
		return 0
	}
	return n
}

// titleFromName gives a title, and an artist when the name has the usual
// "artist - title" shape, for files whose tags are empty.
func titleFromName(name string) (title, artist string) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.TrimSpace(base)
	if i := strings.Index(base, " - "); i > 0 && i+3 < len(base) {
		return strings.TrimSpace(base[i+3:]), strings.TrimSpace(base[:i])
	}
	return base, ""
}
