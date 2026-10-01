package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	"golang.org/x/net/html"
)

const snippetRunes = 160

// textPart is where a message's readable text starts.
type textPart struct {
	path     []int
	encoding string
	charset  string
	html     bool
}

func (p textPart) key() string { return fmt.Sprint(p.path) }

// findTextPart picks the first text/plain part that is not an attachment,
// or else the first text/html one.
func findTextPart(bs imap.BodyStructure) (textPart, bool) {
	var plain, htm *textPart
	bs.Walk(func(path []int, part imap.BodyStructure) bool {
		single, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		if d := single.Disposition(); d != nil && strings.EqualFold(d.Value, "attachment") {
			return true
		}
		tp := textPart{path: slices.Clone(path), encoding: strings.ToLower(single.Encoding), charset: single.Params["charset"]}
		switch single.MediaType() {
		case "text/plain":
			if plain == nil {
				plain = &tp
			}
		case "text/html":
			if htm == nil {
				tp.html = true
				htm = &tp
			}
		}
		return true
	})
	if plain != nil {
		return *plain, true
	}
	if htm != nil {
		return *htm, true
	}
	return textPart{}, false
}

// hasAttachments reports parts that are real attachments, not inline images.
func hasAttachments(bs imap.BodyStructure) bool {
	found := false
	bs.Walk(func(_ []int, part imap.BodyStructure) bool {
		single, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return !found
		}
		d := single.Disposition()
		switch {
		case d != nil && strings.EqualFold(d.Value, "attachment"):
			found = true
		case single.Filename() != "" && (d == nil || !strings.EqualFold(d.Value, "inline")) && single.Type != "text":
			found = true
		}
		return !found
	})
	return found
}

// fetchSnippets reads the first 2 KB of each message's text part. Messages
// whose text sits in the same part are asked in one FETCH.
func (m *Module) fetchSnippets(c *imapclient.Client, msgs []*imapclient.FetchMessageBuffer) map[imap.UID]string {
	out := map[imap.UID]string{}
	type group struct {
		part textPart
		uids imap.UIDSet
	}
	groups := map[string]*group{}
	parts := map[imap.UID]textPart{}
	for _, b := range msgs {
		if b.BodyStructure == nil {
			continue
		}
		tp, ok := findTextPart(b.BodyStructure)
		if !ok {
			continue
		}
		parts[b.UID] = tp
		g := groups[tp.key()]
		if g == nil {
			g = &group{part: tp}
			groups[tp.key()] = g
		}
		g.uids.AddNum(b.UID)
	}
	for _, g := range groups {
		section := &imap.FetchItemBodySection{Part: g.part.path, Peek: true, Partial: &imap.SectionPartial{Offset: 0, Size: snippetBytes}}
		res, err := c.Fetch(g.uids, &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
		if err != nil {
			m.d.Log.Warn("mail: snippet fetch", "err", err)
			continue
		}
		for _, b := range res {
			tp := parts[b.UID]
			for _, s := range b.BodySection {
				out[b.UID] = snippetText(s.Bytes, tp.encoding, tp.charset, tp.html)
			}
		}
	}
	return out
}

// snippetText decodes the start of a part. The bytes may be cut anywhere.
func snippetText(raw []byte, encoding, cs string, isHTML bool) string {
	var b []byte
	switch encoding {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, raw)
		clean = clean[:len(clean)/4*4]
		b = make([]byte, base64.StdEncoding.DecodedLen(len(clean)))
		n, _ := base64.StdEncoding.Decode(b, clean)
		b = b[:n]
	case "quoted-printable":
		b, _ = io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
	default:
		b = raw
	}
	s := decodeCharset(b, cs)
	if isHTML {
		s = htmlToText(s)
	}
	return cutRunes(strings.Join(strings.Fields(s), " "), snippetRunes)
}

func decodeCharset(b []byte, cs string) string {
	switch strings.ToLower(strings.TrimSpace(cs)) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
	default:
		if r, err := charset.Reader(cs, bytes.NewReader(b)); err == nil {
			if out, err := io.ReadAll(r); err == nil || len(out) > 0 {
				b = out
			}
		}
	}
	// 截断时最后一个字可能只有半个，丢掉
	return strings.ToValidUTF8(string(b), "")
}

// htmlToText keeps the visible text of an HTML document, one line per block.
func htmlToText(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var sb strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.TrimSpace(collapseLines(sb.String()))
		case html.TextToken:
			if skip == 0 {
				sb.Write(z.Text())
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "head", "title":
				skip++
			case "br", "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "table", "blockquote":
				sb.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "head", "title":
				if skip > 0 {
					skip--
				}
			case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "table", "blockquote":
				sb.WriteByte('\n')
			}
		}
	}
}

// collapseLines trims every line and keeps at most one empty line in a row.
func collapseLines(s string) string {
	var out []string
	blank := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
