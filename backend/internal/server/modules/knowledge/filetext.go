package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"golang.org/x/net/html"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// B199: text from drive files. Only the start of a file is indexed, as with
// read-later pages.

const (
	// maxFileText is how many characters of a file are kept. The chunker keeps
	// at most 25 passages of about 800 characters anyway.
	maxFileText = 20000
	// maxTextFile is how much of a plain text file is read.
	maxTextFile = 1 << 20
	// maxDocFile is the largest PDF or Word file that is opened.
	maxDocFile = 50 << 20
	// fileTextVersion changes when the extraction changes, so that files are
	// read again.
	fileTextVersion = "1"
)

var textExts = map[string]bool{
	".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".json": true,
	".yaml": true, ".yml": true, ".log": true, ".rst": true, ".org": true, ".tex": true,
	".ini": true, ".toml": true, ".xml": true,
}

// fileKind is "text", "html", "docx", "pdf", or "" for files that are not indexed.
func fileKind(name string, size int64) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case textExts[ext]:
		return "text"
	case ext == ".html" || ext == ".htm":
		return "html"
	case ext == ".docx" && size <= maxDocFile:
		return "docx"
	case ext == ".pdf" && size <= maxDocFile:
		return "pdf"
	}
	return ""
}

var errNoText = errors.New("文件里没有能读出来的文字")

// fileText reads the text of a file of the given kind.
func fileText(kind string, r io.Reader) (string, error) {
	var text string
	switch kind {
	case "text", "html":
		data, err := io.ReadAll(io.LimitReader(r, maxTextFile))
		if err != nil {
			return "", err
		}
		text = decodeText(data)
		if kind == "html" {
			text = htmlText(text)
		}
	case "docx", "pdf":
		data, err := io.ReadAll(io.LimitReader(r, maxDocFile+1))
		if err != nil {
			return "", err
		}
		if kind == "docx" {
			text, err = docxText(data)
		} else {
			text, err = pdfText(data)
		}
		if err != nil {
			return "", err
		}
	default:
		return "", errNoText
	}
	text = strings.TrimSpace(limitRunes(text, maxFileText))
	if text == "" {
		return "", errNoText
	}
	if garbled(text) {
		return "", errors.New("文件里的文字是乱码，可能是扫描件或用了特殊字体")
	}
	return text, nil
}

// decodeText takes UTF-8, and GBK when the bytes are not UTF-8.
func decodeText(data []byte) string {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if utf8.Valid(data) {
		return string(data)
	}
	if out, err := simplifiedchinese.GBK.NewDecoder().Bytes(data); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(data), "")
}

func limitRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// garbled says whether most of the text is not letters, digits or punctuation,
// which is what PDFs with unmapped fonts give.
func garbled(s string) bool {
	total, bad := 0, 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.Is(unicode.Co, r) {
			bad++
		}
	}
	return total > 0 && bad*5 > total
}

var blockTags = map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "section": true, "article": true, "pre": true, "blockquote": true}

// htmlText keeps the visible text of a page, one block per line.
func htmlText(src string) string {
	z := html.NewTokenizer(strings.NewReader(src))
	var b strings.Builder
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return b.String()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch tag := string(name); {
			case tag == "script" || tag == "style" || tag == "noscript":
				skip++
			case blockTags[tag]:
				b.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			switch tag := string(name); {
			case tag == "script" || tag == "style" || tag == "noscript":
				skip = max(0, skip-1)
			case blockTags[tag]:
				b.WriteByte('\n')
			}
		case html.TextToken:
			if skip == 0 {
				b.WriteString(html.UnescapeString(string(z.Text())))
			}
		}
	}
}

// docxText reads word/document.xml: the text runs, one paragraph per line.
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("Word 文件打不开")
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		dec := xml.NewDecoder(io.LimitReader(rc, 64<<20))
		var b strings.Builder
		inText := false
		for b.Len() < maxFileText*4 {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", errors.New("Word 文件内容不完整")
			}
			switch t := tok.(type) {
			case xml.StartElement:
				switch t.Name.Local {
				case "t":
					inText = true
				case "tab":
					b.WriteByte('\t')
				case "br":
					b.WriteByte('\n')
				}
			case xml.EndElement:
				switch t.Name.Local {
				case "t":
					inText = false
				case "p":
					b.WriteByte('\n')
				}
			case xml.CharData:
				if inText {
					b.Write(t)
				}
			}
		}
		return b.String(), nil
	}
	return "", errors.New("Word 文件里没有正文")
}

// pdfText reads page by page until there is enough text.
func pdfText(data []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", fmt.Errorf("PDF 读取失败：%v", r)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("PDF 打不开，可能加了密码或已损坏")
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage() && utf8.RuneCountInString(b.String()) < maxFileText; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		s, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(s)
		b.WriteByte('\n')
	}
	return b.String(), nil
}
