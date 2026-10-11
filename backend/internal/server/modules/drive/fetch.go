package drive

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// Safe outgoing downloads (B151). drive.upload_from_url fetches an address an
// outside AI client chose, so the server must not be tricked into reaching its
// own network: only http and https on ports 80 and 443, the address is
// resolved once and the connection goes to that address, loopback, private,
// link-local (cloud metadata), shared (CGNAT), multicast and unspecified
// addresses are refused, and every redirect is checked again.

var errBlockedTarget = errors.New("这个地址不能下载")

// fetchGuard decides what the downloader may reach. Tests replace the
// functions to talk to a local server.
type fetchGuard struct {
	allowAddr func(netip.Addr) bool
	allowPort func(string) bool
}

func defaultFetchGuard() fetchGuard {
	return fetchGuard{allowAddr: publicAddr, allowPort: func(p string) bool { return p == "80" || p == "443" }}
}

var blockedNets = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"100.64.0.0/10",   // shared address space (CGNAT)
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // documentation
		"198.18.0.0/15",   // benchmarking
		"198.51.100.0/24", // documentation
		"203.0.113.0/24",  // documentation
		"240.0.0.0/4",     // reserved
		"64:ff9b::/96",    // NAT64
		"2001:db8::/32",   // documentation
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// publicAddr reports whether an address is on the public internet.
func publicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range blockedNets {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// client builds an HTTP client that connects only where the guard allows.
// There is no overall timeout (a large file takes as long as it takes); the
// caller stops a download that stalls.
func (g fetchGuard) client() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	tr := &http.Transport{
		Proxy: nil, // never through a proxy from the environment: it could lead inside
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if !g.allowPort(port) {
				return nil, fmt.Errorf("%w（端口 %s）", errBlockedTarget, port)
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			var last error = errBlockedTarget
			for _, ip := range ips {
				if !g.allowAddr(ip) {
					last = errBlockedTarget
					continue
				}
				c, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return c, nil
				}
				last = err
			}
			return nil, last
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConns:          2,
		IdleConnTimeout:       30 * time.Second,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("跳转太多次")
			}
			return checkURL(req.URL)
		},
	}
}

// checkURL accepts only http and https addresses with a host name.
func checkURL(u *url.URL) error {
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return fmt.Errorf("%w（只支持 http 和 https）", errBlockedTarget)
	}
	if u.User != nil {
		return fmt.Errorf("%w（地址里不能带账号密码）", errBlockedTarget)
	}
	return nil
}
