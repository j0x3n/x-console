package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"
)

// Hooks for the integration tests in package knowledge_test, which cannot live
// in this package because testutil imports app, which imports knowledge.

func SetNow(m *Module, now func() time.Time) { m.nowFn = now }

func Sync(m *Module, ctx context.Context) { m.sync(ctx) }

// Status returns the sync state kept in memory.
func LastError(m *Module) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastErr
}

func SetBudget(m *Module, n int) { m.budget = n }

// MakePDF builds a one-page PDF that shows text in Helvetica (B199 tests).
// text must be ASCII without parentheses or backslashes.
func MakePDF(text string) []byte {
	content := "BT /F1 18 Tf 20 100 Td (" + text + ") Tj ET"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return []byte(b.String())
}

// MakeDocx builds a Word file with one paragraph per line.
func MakeDocx(lines ...string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("word/document.xml")
	var body strings.Builder
	for _, l := range lines {
		body.WriteString(`<w:p><w:r><w:t>` + l + `</w:t></w:r></w:p>`)
	}
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>%s</w:body></w:document>`, body.String())
	zw.Close()
	return buf.Bytes()
}
