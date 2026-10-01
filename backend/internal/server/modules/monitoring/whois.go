package monitoring

import (
	"context"
	"io"
	"net"
	"strings"
	"time"
)

const (
	WhoisIANAKey     = "monitoring.whois_iana"
	defaultWhoisIANA = "whois.iana.org:43"
	whoisTimeout     = 10 * time.Second
	whoisMaxBody     = 64 << 10
	whoisCacheTTL    = 24 * time.Hour
)

type whoisCacheEntry struct {
	host string
	ok   bool
	at   time.Time
}

func (m *Module) whoisIANA(ctx context.Context) string {
	var addr string
	if err := m.d.Settings.Get(ctx, WhoisIANAKey, &addr); err != nil || strings.TrimSpace(addr) == "" {
		return defaultWhoisIANA
	}
	return strings.TrimSpace(addr)
}

func (m *Module) lookupWhois(ctx context.Context, domain string) (time.Time, string, bool) {
	server, ok := m.whoisServer(ctx, domainTLD(domain))
	if !ok {
		return time.Time{}, "", false
	}
	body, err := queryWhois(ctx, whoisAddr(server), domain)
	if err != nil {
		return time.Time{}, "", false
	}
	if next := whoisReferral(body); next != "" && !sameWhoisHost(next, server) {
		// The registrar's answer wins, but some registrars leave the date
		// out: then the registry's answer is used.
		if body2, err := queryWhois(ctx, whoisAddr(next), domain); err == nil {
			if exp, reg, ok := parseWhois(body2); ok {
				return exp, reg, true
			}
		}
	}
	return parseWhois(body)
}

func (m *Module) whoisServer(ctx context.Context, tld string) (string, bool) {
	if tld == "" {
		return "", false
	}
	m.whoisMu.Lock()
	if m.whoisServers != nil {
		if c, ok := m.whoisServers[tld]; ok && time.Since(c.at) < whoisCacheTTL {
			m.whoisMu.Unlock()
			return c.host, c.ok
		}
	}
	m.whoisMu.Unlock()

	body, err := queryWhois(ctx, m.whoisIANA(ctx), tld)
	if err != nil {
		return "", false
	}
	host := ianaWhoisHost(body)
	m.whoisMu.Lock()
	if m.whoisServers == nil {
		m.whoisServers = map[string]whoisCacheEntry{}
	}
	m.whoisServers[tld] = whoisCacheEntry{host: host, ok: host != "", at: time.Now()}
	m.whoisMu.Unlock()
	return host, host != ""
}

func queryWhois(ctx context.Context, addr, query string) (string, error) {
	addr = whoisAddr(addr)
	if addr == "" {
		return "", net.InvalidAddrError("empty whois address")
	}
	conn, err := (&net.Dialer{Timeout: whoisTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(whoisTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if _, err := io.WriteString(conn, query+"\r\n"); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(conn, whoisMaxBody))
	if err != nil && len(data) == 0 {
		return "", err
	}
	return string(data), nil
}

func whoisAddr(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "43")
}

func sameWhoisHost(a, b string) bool {
	return strings.EqualFold(whoisAddr(a), whoisAddr(b))
}

func domainTLD(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	i := strings.LastIndex(domain, ".")
	if i < 0 || i == len(domain)-1 {
		return ""
	}
	return domain[i+1:]
}

func ianaWhoisHost(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if len(line) < 6 || !strings.EqualFold(line[:6], "whois:") {
			continue
		}
		return firstField(line[6:])
	}
	return ""
}

func whoisReferral(body string) string {
	for _, line := range strings.Split(body, "\n") {
		name, value, ok := splitWhoisLine(line)
		if !ok {
			continue
		}
		if strings.EqualFold(name, "whois server") || strings.EqualFold(name, "registrar whois server") {
			return firstField(value)
		}
	}
	return ""
}

func splitWhoisLine(line string) (string, string, bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\r"))
	i := strings.Index(line, ":")
	if i <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func firstField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

var whoisExpiryNames = map[string]struct{}{
	"registry expiry date":                   {},
	"registrar registration expiration date": {},
	"expiry date":                            {},
	"expiration date":                        {},
	"expiration time":                        {},
	"expires on":                             {},
	"expires":                                {},
	"paid-till":                              {},
	"valid until":                            {},
	"renewal date":                           {},
}

var whoisTimeFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006.01.02",
	"2006/01/02",
	"02/01/2006 15:04:05",
	"02/01/2006",
	"02-Jan-2006",
	"January 2 2006",
}

func parseWhois(body string) (time.Time, string, bool) {
	var registrar string
	var expires time.Time
	found := false
	for _, line := range strings.Split(body, "\n") {
		name, value, ok := splitWhoisLine(line)
		if !ok {
			continue
		}
		key := strings.ToLower(name)
		if key == "registrar" && registrar == "" {
			registrar = value
		}
		if _, ok := whoisExpiryNames[key]; ok && !found {
			if t, ok := parseWhoisTime(value); ok {
				expires = t
				found = true
			}
		}
	}
	return expires, registrar, found
}

func parseWhoisTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	fields := strings.Fields(value)
	candidates := []string{value}
	if len(fields) > 0 {
		candidates = append(candidates, fields[0])
	}
	if len(fields) >= 2 {
		candidates = append(candidates, fields[0]+" "+fields[1])
	}
	if len(fields) >= 3 {
		candidates = append(candidates, strings.Join(fields[:3], " "))
	}
	for _, c := range candidates {
		for _, layout := range whoisTimeFormats {
			if t, err := time.Parse(layout, c); err == nil {
				return t.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

func manualExpiryTime(day time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := day.Date()
	return time.Date(y, m, d, 23, 59, 59, 0, loc)
}
