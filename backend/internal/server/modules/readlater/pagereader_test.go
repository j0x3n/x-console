package readlater_test

import (
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/readlater"
)

// B211: other modules read public pages through readlater. Local and internal
// addresses are refused, and nothing lands in the read-later list.
func TestPageReader(t *testing.T) {
	env, m, s := setup(t)
	reader, ok := module.Lookup[contracts.PageReader](env.App.Deps.Registry, contracts.PageReaderKey)
	if !ok {
		t.Fatal("PageReader not provided")
	}

	page, err := reader.Read(t.Context(), s.URL+"/article")
	if err != nil || !strings.Contains(page.Title, "鲸鱼协议详解") || !strings.Contains(page.Text, "心跳间隔不能短于往返时间的两倍") {
		t.Fatalf("read: %+v %v", page, err)
	}
	text, err := reader.Read(t.Context(), s.URL+"/notes.txt")
	if err != nil || !strings.Contains(text.Text, "第二行") {
		t.Fatalf("plain text: %+v %v", text, err)
	}

	var n int
	if err := env.App.Deps.DB.QueryRow(`SELECT count(*) FROM read_items`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("read items after reading: %d %v", n, err)
	}

	// The default client refuses the test server, which is on 127.0.0.1.
	readlater.UseStrictClient(m)
	for _, target := range []string{s.URL + "/article", "http://127.0.0.1:1/", "http://169.254.169.254/latest/meta-data/"} {
		if _, err := reader.Read(t.Context(), target); err == nil || !strings.Contains(err.Error(), "不能访问") {
			t.Fatalf("%s: want a refusal, got %q", target, err.Error())
		}
	}
	// the IPv6 loopback may fail to connect before the check, so only an error is required
	for _, target := range []string{"http://[::1]/", "file:///etc/passwd", "ftp://example.com/x", "example.com/x"} {
		if _, err := reader.Read(t.Context(), target); err == nil {
			t.Fatalf("%s went through", target)
		}
	}
}
