package mail

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net/textproto"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
	mailmessage "github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

const snippetRunes = 160

// Read the original headers instead of ENVELOPE: some servers return an
// invalid quoted string there and break the whole IMAP connection.
func summaryHeaderSection() *imap.FetchItemBodySection {
	return &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"Subject", "From", "Date", "Message-ID"},
		Peek:         true,
	}
}

func fullHeaderSection() *imap.FetchItemBodySection {
	return &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, Peek: true}
}

// go-imap encodes HEADER.FIELDS names as quoted strings. On a tagged BAD,
// try the simpler HEADER request for compatibility with strict servers.
// Both forms use PEEK and avoid the malformed ENVELOPE returned by QQ.
func fetchSummaries(c *imapclient.Client, set imap.NumSet) ([]*imapclient.FetchMessageBuffer, error) {
	opts := &imap.FetchOptions{
		UID: true, Flags: true, InternalDate: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
		BodySection: []*imap.FetchItemBodySection{summaryHeaderSection()},
	}
	msgs, err := c.Fetch(set, opts).Collect()
	if err == nil && !summariesNeedFullHeaders(msgs) {
		return msgs, nil
	}
	var status *imap.Error
	if err != nil && (!errors.As(err, &status) || status.Type != imap.StatusResponseTypeBad) {
		return nil, fmt.Errorf("读取邮件摘要（FETCH HEADER.FIELDS）: %w", err)
	}
	opts.BodySection = []*imap.FetchItemBodySection{fullHeaderSection()}
	msgs, err = c.Fetch(set, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("读取邮件摘要（重试 FETCH HEADER）: %w", err)
	}
	// Some servers answer OK but return empty or unrelated filtered headers.
	// If even HEADER is empty, obtain the original RFC 5322 message with PEEK.
	for _, b := range msgs {
		raw, ok := summaryHeaderBytes(b)
		if ok {
			h := summaryHeader(raw)
			if h.Get("Subject") != "" || h.Get("From") != "" {
				continue
			}
		}
		section := bodySection()
		full, err := c.Fetch(imap.UIDSetNum(b.UID), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
		if err != nil {
			return nil, fmt.Errorf("补取完整邮件的邮件头: %w", err)
		}
		found := false
		for _, message := range full {
			if message.UID != b.UID {
				continue
			}
			raw := messageBodyBytes(message)
			if raw == nil {
				continue
			}
			if end := bytes.Index(raw, []byte("\r\n\r\n")); end >= 0 {
				raw = raw[:end+4]
			} else if end := bytes.Index(raw, []byte("\n\n")); end >= 0 {
				raw = raw[:end+2]
			}
			b.BodySection = []imapclient.FetchBodySectionBuffer{{Section: fullHeaderSection(), Bytes: bytes.Clone(raw)}}
			found = true
			break
		}
		if !found {
			return nil, errors.New("补取完整邮件失败：没有返回原始邮件")
		}
	}
	return checkedSummaryHeaders(msgs)
}

func summariesNeedFullHeaders(msgs []*imapclient.FetchMessageBuffer) bool {
	for _, b := range msgs {
		raw, ok := summaryHeaderBytes(b)
		if !ok {
			return true
		}
		h := summaryHeader(raw)
		if h.Get("Subject") == "" || h.Get("From") == "" {
			return true
		}
	}
	return false
}

// Offset-zero FETCH responses may contain the entire body without echoing a
// partial marker. Never confuse a text part or header with the original mail.
func messageBodyBytes(b *imapclient.FetchMessageBuffer) []byte {
	for _, body := range b.BodySection {
		s := body.Section
		if s != nil && s.Specifier == "" && len(s.Part) == 0 && len(s.HeaderFields) == 0 && len(s.HeaderFieldsNot) == 0 && (s.Partial == nil || s.Partial.Offset == 0) && body.Bytes != nil {
			return body.Bytes
		}
	}
	return nil
}

func checkedSummaryHeaders(msgs []*imapclient.FetchMessageBuffer) ([]*imapclient.FetchMessageBuffer, error) {
	for _, b := range msgs {
		if _, ok := summaryHeaderBytes(b); !ok {
			return nil, errors.New("读取邮件摘要失败：FETCH 没有返回完整邮件头")
		}
	}
	return msgs, nil
}

// Header field lists describe a set. Servers may reorder or change the case
// of its names. Never accept a message body, nested header or partial header.
func summaryHeaderBytes(b *imapclient.FetchMessageBuffer) ([]byte, bool) {
	for _, body := range b.BodySection {
		s := body.Section
		if s == nil || s.Specifier != imap.PartSpecifierHeader || len(s.Part) != 0 || s.Partial != nil || len(s.HeaderFieldsNot) != 0 || body.Bytes == nil {
			continue
		}
		complete := true
		if len(s.HeaderFields) != 0 {
			for _, want := range summaryHeaderSection().HeaderFields {
				if !slices.ContainsFunc(s.HeaderFields, func(field string) bool { return strings.EqualFold(field, want) }) {
					complete = false
					break
				}
			}
		}
		if complete {
			return body.Bytes, true
		}
	}
	return nil, false
}

func addressName(name string) string {
	if decoded, err := wordDecoder.DecodeHeader(name); err == nil {
		name = decoded
	}
	return cleanLine(name)
}

func summaryHeader(raw []byte) mailmessage.Header {
	// Keep any fields parsed before a malformed header, without blocking sync.
	h, _ := textproto.NewReader(bufio.NewReader(io.LimitReader(bytes.NewReader(raw), 64*1024))).ReadMIMEHeader()
	return mailmessage.HeaderFromMap(h)
}

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
	return cutRunes(cleanText(s), snippetRunes)
}

var (
	// looksHTML: a tag or a comment, so text/plain parts that carry HTML
	// (some newsletters do) are cleaned too.
	looksHTML = regexp.MustCompile(`(?i)<(!--|!\[|/?[a-z][a-z0-9]*[\s/>])`)
	// htmlLeftovers are pieces of Outlook conditional comments that survive
	// when a snippet is cut, e.g. "<!--[if !mso]><!-->" or "<![endif]-->".
	htmlLeftovers = regexp.MustCompile(`(?i)<!--\[if[^\]]*\]>(<!-->)?|<!\[endif\]-->|<!\[endif\]>|<!-->|<!--|-->|\[if !?mso[^\]]*\]>?|\[endif\]`)
)

// cleanText turns a snippet into one line of plain text (B92): HTML and
// escaped HTML become their visible text, comment leftovers and entities go.
func cleanText(s string) string {
	if strings.Contains(s, "&lt;") {
		s = html.UnescapeString(s)
	}
	if looksHTML.MatchString(s) {
		s = htmlToText(s)
	}
	s = htmlLeftovers.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.Join(strings.Fields(s), " ")
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
