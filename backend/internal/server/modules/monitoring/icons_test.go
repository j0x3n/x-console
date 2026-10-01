package monitoring_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring/api"
)

func tinyPNG(t *testing.T, c color.Color) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func waitIcons(t *testing.T, m *monitoring.Module) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.WaitIcons(ctx); err != nil {
		t.Fatal(err)
	}
}

func getIcon(t *testing.T, env interface{ URL(string) string }, client *http.Client, id int64) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, env.URL(fmt.Sprintf("/monitors/%d/icon", id)), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func TestMonitorIcons(t *testing.T) {
	env, m := setup(t)
	pngRed := tinyPNG(t, color.NRGBA{R: 255, A: 255})
	pngBlue := tinyPNG(t, color.NRGBA{B: 255, A: 255})
	pngGreen := tinyPNG(t, color.NRGBA{G: 255, A: 255})
	var mu sync.Mutex
	paths := map[string]int{}
	var modeBox struct {
		mu   sync.Mutex
		mode string
	}
	modeBox.mode = "link"
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths[r.URL.Path]++
		mu.Unlock()
		modeBox.mu.Lock()
		cur := modeBox.mode
		modeBox.mu.Unlock()
		switch cur {
		case "link":
			if r.URL.Path == "/i.png" {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngRed)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/i.png"></head></html>`))
		case "favicon":
			if r.URL.Path == "/favicon.ico" {
				w.Header().Set("Content-Type", "image/x-icon")
				_, _ = w.Write(pngBlue)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><title>x</title></head></html>`))
		case "huge":
			if r.URL.Path == "/i.png" {
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(bytes.Repeat([]byte{0}, 64*1024+1))
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="icon" href="/i.png">`))
		case "svg":
			if r.URL.Path == "/i.svg" {
				w.Header().Set("Content-Type", "image/svg+xml")
				_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="icon" href="/i.svg">`))
		case "magic":
			if r.URL.Path == "/i.bin" {
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(pngGreen)
				return
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<link rel="icon" href="/i.bin">`))
		case "apple":
			switch r.URL.Path {
			case "/small.png":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngBlue)
			case "/apple.png":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngRed)
			default:
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<link rel="icon" sizes="32x32" href="/small.png"><link rel="apple-touch-icon" href="/apple.png">`))
			}
		case "sizes":
			switch r.URL.Path {
			case "/small.png":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngBlue)
			case "/big.png":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngGreen)
			default:
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<link rel="icon" href="/small.png" sizes="16x16"><link rel="icon" href="/big.png" sizes="128x128">`))
			}
		case "r3":
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, "/b", http.StatusFound)
			case "/b":
				http.Redirect(w, r, "/c", http.StatusFound)
			case "/c":
				http.Redirect(w, r, "/d", http.StatusFound)
			case "/d":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngRed)
			default:
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<link rel="icon" href="/a">`))
			}
		case "r4":
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, "/b", http.StatusFound)
			case "/b":
				http.Redirect(w, r, "/c", http.StatusFound)
			case "/c":
				http.Redirect(w, r, "/d", http.StatusFound)
			case "/d":
				http.Redirect(w, r, "/e", http.StatusFound)
			case "/e":
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(pngRed)
			default:
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<link rel="icon" href="/a">`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer site.Close()

	setMode := func(name string) {
		waitIcons(t, m)
		modeBox.mu.Lock()
		modeBox.mode = name
		modeBox.mu.Unlock()
	}
	add := func(name, target string) api.Monitor {
		t.Helper()
		var mon api.Monitor
		env.MustDo(http.MethodPost, "/monitors", api.MonitorInput{Kind: "http", Name: name, Target: target}, &mon)
		if mon.IconAt != nil {
			t.Fatalf("create should not wait for the icon: %+v", mon)
		}
		waitIcons(t, m)
		return mon
	}

	link := add("link", site.URL)
	status, hdr, body := getIcon(t, env, env.Client, link.Id)
	if status != 200 || hdr.Get("Content-Type") != "image/png" || !bytes.Equal(body, pngRed) {
		t.Fatalf("link icon: %d %s %d bytes", status, hdr.Get("Content-Type"), len(body))
	}
	if hdr.Get("Cache-Control") != "private, max-age=604800" || hdr.Get("X-Content-Type-Options") != "nosniff" || hdr.Get("Content-Security-Policy") != "" {
		t.Fatalf("png headers: %v", hdr)
	}
	var got api.Monitor
	env.MustDo(http.MethodGet, fmt.Sprintf("/monitors/%d", link.Id), nil, &got)
	if got.IconAt == nil {
		t.Fatal("missing iconAt")
	}
	var list []api.Monitor
	env.MustDo(http.MethodGet, "/monitors?kind=http", nil, &list)
	var listed bool
	for _, item := range list {
		if item.Id == link.Id && item.IconAt != nil {
			listed = true
		}
	}
	if !listed {
		t.Fatalf("list missing iconAt: %+v", list)
	}

	mu.Lock()
	pngHits := paths["/i.png"]
	mu.Unlock()
	if _, err := m.CheckNow(context.Background(), link.Id, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitIcons(t, m)
	mu.Lock()
	if paths["/i.png"] != pngHits {
		t.Fatalf("fresh icon was fetched again: %d -> %d", pngHits, paths["/i.png"])
	}
	mu.Unlock()
	old := time.Now().Add(-8 * 24 * time.Hour).UTC()
	if _, err := env.App.Deps.DB.Exec(`UPDATE monitor_icons SET fetched_at = ? WHERE monitor_id = ?`, old, link.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CheckNow(context.Background(), link.Id, time.Now()); err != nil {
		t.Fatal(err)
	}
	waitIcons(t, m)
	mu.Lock()
	if paths["/i.png"] <= pngHits {
		t.Fatalf("stale icon was not fetched: %d", paths["/i.png"])
	}
	mu.Unlock()

	setMode("favicon")
	fav := add("favicon", site.URL)
	status, hdr, body = getIcon(t, env, env.Client, fav.Id)
	if status != 200 || hdr.Get("Content-Type") != "image/x-icon" || !bytes.Equal(body, pngBlue) {
		t.Fatalf("favicon: %d %s %d", status, hdr.Get("Content-Type"), len(body))
	}

	setMode("huge")
	huge := add("huge", site.URL)
	status, _, _ = getIcon(t, env, env.Client, huge.Id)
	if status != 404 {
		t.Fatalf("huge icon stored: %d", status)
	}

	setMode("svg")
	svg := add("svg", site.URL)
	status, hdr, body = getIcon(t, env, env.Client, svg.Id)
	if status != 200 || hdr.Get("Content-Type") != "image/svg+xml" || !bytes.Contains(body, []byte("<svg")) {
		t.Fatalf("svg: %d %s %q", status, hdr.Get("Content-Type"), body)
	}
	if hdr.Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'; sandbox" {
		t.Fatalf("svg csp: %q", hdr.Get("Content-Security-Policy"))
	}

	setMode("magic")
	magic := add("magic", site.URL)
	status, hdr, body = getIcon(t, env, env.Client, magic.Id)
	if status != 200 || hdr.Get("Content-Type") != "image/png" || !bytes.Equal(body, pngGreen) {
		t.Fatalf("sniff: %d %s", status, hdr.Get("Content-Type"))
	}

	setMode("apple")
	apple := add("apple", site.URL)
	_, _, body = getIcon(t, env, env.Client, apple.Id)
	if !bytes.Equal(body, pngRed) {
		t.Fatal("apple-touch-icon was not preferred")
	}

	setMode("sizes")
	sized := add("sizes", site.URL)
	_, _, body = getIcon(t, env, env.Client, sized.Id)
	if !bytes.Equal(body, pngGreen) {
		t.Fatal("largest icon was not preferred")
	}

	setMode("r3")
	okRedir := add("r3", site.URL)
	status, _, body = getIcon(t, env, env.Client, okRedir.Id)
	if status != 200 || !bytes.Equal(body, pngRed) {
		t.Fatalf("3 redirects: %d %d bytes", status, len(body))
	}

	setMode("r4")
	badRedir := add("r4", site.URL)
	status, _, _ = getIcon(t, env, env.Client, badRedir.Id)
	if status != 404 {
		t.Fatalf("4 redirects stored: %d", status)
	}

	env.MustDo(http.MethodDelete, fmt.Sprintf("/monitors/%d", link.Id), nil, nil)
	status, _, _ = getIcon(t, env, env.Client, link.Id)
	if status != 404 {
		t.Fatalf("icon after delete: %d", status)
	}
	var left int
	if err := env.App.Deps.DB.QueryRow(`SELECT COUNT(*) FROM monitor_icons WHERE monitor_id = ?`, link.Id).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("icon row left: %d", left)
	}
}
