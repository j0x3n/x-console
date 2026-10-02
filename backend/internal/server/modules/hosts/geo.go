package hosts

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

type geoResolver struct {
	mu               sync.RWMutex
	reader           *maxminddb.Reader
	path, url, month string
	client           *http.Client
	fetching         bool
	closed           bool
	retryAt          time.Time
}

func newGeoResolver(dir string) *geoResolver {
	g := &geoResolver{path: filepath.Join(dir, "geo", "country.mmdb"), client: &http.Client{Timeout: time.Minute}}
	if reader, err := maxminddb.Open(g.path); err == nil {
		if reader.Verify() == nil {
			g.reader = reader
			if info, e := os.Stat(g.path); e == nil {
				g.month = info.ModTime().UTC().Format("2006-01")
			}
		} else {
			reader.Close()
		}
	}
	return g
}

func countryName(code string) string {
	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}
	name := display.SimplifiedChinese.Regions().Name(region)
	if name == "" {
		return code
	}
	return name
}

func publicIP(ip string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
		return netip.Addr{}, false
	}
	for _, prefix := range []string{"100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		p, _ := netip.ParsePrefix(prefix)
		if p.Contains(addr) {
			return netip.Addr{}, false
		}
	}
	return addr, true
}

func (g *geoResolver) country(ctx context.Context, ip string) string {
	addr, ok := publicIP(ip)
	if !ok {
		return ""
	}
	g.update(ctx)
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.reader == nil {
		return ""
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := g.reader.Lookup(net.IP(addr.AsSlice()), &record); err != nil {
		return ""
	}
	code := strings.ToUpper(record.Country.ISOCode)
	if len(code) != 2 {
		return ""
	}
	return code
}

func (g *geoResolver) update(ctx context.Context) {
	g.mu.Lock()
	month := time.Now().UTC().Format("2006-01")
	if g.closed || g.fetching || g.month == month || time.Now().Before(g.retryAt) {
		g.mu.Unlock()
		return
	}
	g.fetching = true
	g.mu.Unlock()
	go func() {
		runCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		err := g.download(runCtx, month)
		g.mu.Lock()
		g.fetching = false
		if err != nil {
			g.retryAt = time.Now().Add(time.Hour)
		}
		g.mu.Unlock()
	}()
}

func (g *geoResolver) download(ctx context.Context, month string) error {
	url := g.url
	if url == "" {
		url = "https://download.db-ip.com/free/dbip-country-lite-" + month + ".mmdb.gz"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("geo download: %d", resp.StatusCode)
	}
	reader, err := gzip.NewReader(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return err
	}
	defer reader.Close()
	if err = os.MkdirAll(filepath.Dir(g.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(g.path), "country-*.mmdb")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, io.LimitReader(reader, (50<<20)+1))
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if n > 50<<20 {
		return errors.New("geo database too large")
	}
	next, err := maxminddb.Open(tmp.Name())
	if err != nil {
		return err
	}
	err = next.Verify()
	_ = next.Close()
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return context.Canceled
	}
	if g.reader != nil {
		_ = g.reader.Close()
		g.reader = nil
	}
	if err = os.Rename(tmp.Name(), g.path); err != nil {
		g.reader, _ = maxminddb.Open(g.path)
		return err
	}
	g.reader, err = maxminddb.Open(g.path)
	if err == nil {
		g.month = month
		g.retryAt = time.Time{}
	}
	return err
}

func (g *geoResolver) close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	if g.reader != nil {
		g.reader.Close()
		g.reader = nil
	}
}
