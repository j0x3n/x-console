package protocol

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	CapNetFetch              = "net.fetch"
	MethodNetFetch           = "net.fetch"
	NetFetchMaxBytes         = 8 << 20
	NetFetchDefaultTimeoutMs = 8000
	NetFetchMaxTimeoutMs     = 15000
	// Exact hosts and suffixes. A suffix requires at least one subdomain label.
	NetFetchHosts = "music.163.com,c.y.qq.com,u.y.qq.com,y.qq.com,y.gtimg.cn,lrclib.net,itunes.apple.com,musicbrainz.org,coverartarchive.org,.mzstatic.com,.music.126.net,.gtimg.cn,.archive.org"
)

type NetFetchParams struct {
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers,omitempty"`
	MaxBytes  int               `json:"maxBytes,omitempty"`
	TimeoutMs int               `json:"timeoutMs,omitempty"`
}

type NetFetchResult struct {
	Status      int    `json:"status"`
	ContentType string `json:"contentType"`
	Body        []byte `json:"body"` // JSON encodes bytes as base64.
}

func ValidateNetFetchURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return &Error{Code: CodeBadParams, Message: "只允许不带凭据的 HTTPS 音乐来源地址"}
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range strings.Split(NetFetchHosts, ",") {
		if host == allowed || strings.HasPrefix(allowed, ".") && strings.HasSuffix(host, allowed) {
			return nil
		}
	}
	return &Error{Code: CodeForbiddenTarget, Message: "主机不在音乐来源白名单里"}
}

func NetFetchHeaderAllowed(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "User-Agent", "Referer", "Accept", "Cookie":
		return true
	}
	return false
}

func (p *NetFetchParams) Validate() error {
	if err := ValidateNetFetchURL(p.URL); err != nil {
		return err
	}
	if p.MaxBytes < 0 || p.MaxBytes > NetFetchMaxBytes || p.TimeoutMs < 0 || p.TimeoutMs > NetFetchMaxTimeoutMs {
		return &Error{Code: CodeBadParams, Message: "响应大小或超时超过允许范围"}
	}
	if p.MaxBytes == 0 {
		p.MaxBytes = NetFetchMaxBytes
	}
	if p.TimeoutMs == 0 {
		p.TimeoutMs = NetFetchDefaultTimeoutMs
	}
	for name, value := range p.Headers {
		if !NetFetchHeaderAllowed(name) || strings.ContainsAny(value, "\r\n\x00") {
			return &Error{Code: CodeBadParams, Message: fmt.Sprintf("不允许的请求头：%s", name)}
		}
	}
	return nil
}

// NetFetchIPAllowed checks the resolved IP actually being dialed.
func NetFetchIPAllowed(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	// Also exclude carrier-grade NAT, whose addresses can route to local services.
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return !cgnat.Contains(ip)
}
