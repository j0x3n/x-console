package browser

import (
	_ "embed"
	"regexp"
	"strings"
)

// stealth.js makes headless Chrome look like the Chrome people use (B226).
//
//go:embed stealth.js
var stealthJS string

// Names that a normal Chrome on Linux with an Intel graphics card reports for
// WebGL. The software renderer of headless Chrome says SwiftShader instead.
const (
	webglVendor   = "Google Inc. (Intel)"
	webglRenderer = "ANGLE (Intel, Mesa Intel(R) UHD Graphics 630 (CFL GT2), OpenGL 4.6)"
	// chromeBarHeight is what window.outerHeight adds to the page height for
	// the tabs and the address bar.
	chromeBarHeight = 85
)

// Screen size that the pages see, a bit larger than the window.
const (
	screenWidth  = 1920
	screenHeight = 1080
)

// languagesFor returns navigator.languages for a locale such as "zh-CN":
// the locale and then its language.
func languagesFor(locale string) []string {
	if locale == "" {
		locale = "zh-CN"
	}
	lang := strings.SplitN(locale, "-", 2)[0]
	if lang == locale {
		return []string{locale}
	}
	return []string{locale, lang}
}

// stealthSource returns the script to run before every new document.
func stealthSource(locale string) string {
	cfg := map[string]any{
		"languages":       languagesFor(locale),
		"webglVendor":     webglVendor,
		"webglRenderer":   webglRenderer,
		"chromeBarHeight": chromeBarHeight,
	}
	return stealthJS + "(" + mustJSON(cfg) + ");"
}

var chromeVersion = regexp.MustCompile(`(?:Headless)?Chrome/(\d+)\.(\d+)\.(\d+)\.(\d+)`)

// userAgent returns the user agent without the word HeadlessChrome and the
// matching userAgentMetadata for Network.setUserAgentOverride. Without the
// metadata, the Sec-CH-UA header and navigator.userAgentData still say
// HeadlessChrome. It returns ok false when the version cannot be read.
func userAgent(ua, product string) (string, map[string]any, bool) {
	ua = strings.ReplaceAll(ua, "HeadlessChrome", "Chrome")
	m := chromeVersion.FindStringSubmatch(ua)
	if m == nil {
		m = chromeVersion.FindStringSubmatch(product)
	}
	if m == nil {
		return ua, nil, false
	}
	major := m[1]
	full := strings.Join(m[1:], ".")
	// The product has the real build number, the user agent has 0 for the
	// last three parts.
	if p := chromeVersion.FindStringSubmatch(product); p != nil {
		full = strings.Join(p[1:], ".")
	}
	// Chrome adds one entry with a made-up name so that sites do not depend
	// on the list.
	brands := func(version, notBrand string) []map[string]string {
		return []map[string]string{
			{"brand": "Not.A/Brand", "version": notBrand},
			{"brand": "Chromium", "version": version},
			{"brand": "Google Chrome", "version": version},
		}
	}
	platform, arch := "Linux", "x86"
	switch {
	case strings.Contains(ua, "Windows"):
		platform = "Windows"
	case strings.Contains(ua, "Macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "CrOS"):
		platform = "Chrome OS"
	}
	if strings.Contains(ua, "aarch64") || strings.Contains(ua, "arm64") {
		arch = "arm"
	}
	return ua, map[string]any{
		"brands":          brands(major, "24"),
		"fullVersionList": brands(full, "24.0.0.0"),
		"fullVersion":     full,
		"platform":        platform,
		"platformVersion": "",
		"architecture":    arch,
		"bitness":         "64",
		"model":           "",
		"mobile":          false,
		"wow64":           false,
	}, true
}
