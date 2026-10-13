package knowledge

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestFileKind(t *testing.T) {
	cases := map[string]string{"a.md": "text", "B.TXT": "text", "x.html": "html", "r.docx": "docx", "p.pdf": "pdf", "song.mp3": "", "pic.png": "", "old.doc": ""}
	for name, want := range cases {
		if got := fileKind(name, 1000); got != want {
			t.Errorf("%s: %q", name, got)
		}
	}
	if fileKind("big.pdf", maxDocFile+1) != "" {
		t.Error("too large PDF is indexed")
	}
}

func TestFileText(t *testing.T) {
	gbk, _ := simplifiedchinese.GBK.NewEncoder().String("证书续不上")
	if got, err := fileText("text", strings.NewReader(gbk)); err != nil || got != "证书续不上" {
		t.Errorf("gbk: %q %v", got, err)
	}
	if got, err := fileText("text", strings.NewReader("\xef\xbb\xbf  你好 \n")); err != nil || got != "你好" {
		t.Errorf("bom: %q %v", got, err)
	}
	page := `<html><head><style>p{}</style><script>var x = "不要";</script></head><body><h1>标题</h1><p>正文 &amp; 链接</p></body></html>`
	if got, err := fileText("html", strings.NewReader(page)); err != nil || strings.Contains(got, "不要") || !strings.Contains(got, "标题") || !strings.Contains(got, "正文 & 链接") {
		t.Errorf("html: %q %v", got, err)
	}
	doc := string(MakeDocx("第一段", "第二段"))
	if got, err := fileText("docx", strings.NewReader(doc)); err != nil || got != "第一段\n第二段" {
		t.Errorf("docx: %q %v", got, err)
	}
	if _, err := fileText("docx", strings.NewReader("not a zip")); err == nil {
		t.Error("broken docx")
	}
	pdf := string(MakePDF("Caddy certificate renewal failed"))
	if got, err := fileText("pdf", strings.NewReader(pdf)); err != nil || !strings.Contains(got, "certificate") {
		t.Errorf("pdf: %q %v", got, err)
	}
	if _, err := fileText("pdf", strings.NewReader("%PDF-1.4 broken")); err == nil {
		t.Error("broken pdf")
	}
	if _, err := fileText("text", strings.NewReader("   ")); err == nil {
		t.Error("empty text")
	}
	if _, err := fileText("text", strings.NewReader(strings.Repeat("\x01\x02", 50))); err == nil {
		t.Error("garbled text")
	}
	long := strings.Repeat("字", maxFileText+100)
	if got, _ := fileText("text", strings.NewReader(long)); len([]rune(got)) != maxFileText {
		t.Errorf("limit: %d", len([]rune(got)))
	}
}
