// Package browsertest finds a Chrome for tests of code that uses the browser
// package.
package browsertest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
)

// Path returns a Chrome executable, or "" when there is none. It looks at
// XC_TEST_BROWSER, XC_SHOTS_BROWSER (set in CI), the usual install places and
// the Playwright browsers folder.
func Path() string {
	for _, env := range []string{"XC_TEST_BROWSER", "XC_SHOTS_BROWSER"} {
		if p := os.Getenv(env); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	for _, p := range []string{"/usr/bin/google-chrome", "/usr/bin/chromium", "/usr/bin/chromium-browser"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	base := os.Getenv("PLAYWRIGHT_BROWSERS_PATH")
	if base == "" {
		base = "/opt/pw-browsers"
	}
	if list, _ := filepath.Glob(filepath.Join(base, "chromium-*", "chrome-linux", "chrome")); len(list) > 0 {
		return list[len(list)-1]
	}
	return ""
}

// Service returns a browser service that starts a local Chrome, and skips the
// test when there is none.
func Service(t testing.TB) *browser.Service {
	t.Helper()
	p := Path()
	if p == "" {
		t.Skip("没有找到 Chrome，设置 XC_TEST_BROWSER")
	}
	s := browser.New(p, nil)
	t.Cleanup(s.Close)
	return s
}
