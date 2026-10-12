// Package netfetch fetches a bounded GET response from an allowed public music source.
package netfetch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"

	"github.com/j0x3n/x-console/backend/pkg/protocol"
)

type Fetcher struct {
	transport *http.Transport
	allow     func(net.IP) bool
}

func New() *Fetcher { return newFetcher(protocol.NetFetchIPAllowed) }

func newFetcher(allow func(net.IP) bool) *Fetcher {
	f := &Fetcher{allow: allow}
	dialer := &net.Dialer{Timeout: 8 * time.Second, Control: f.control}
	f.transport = &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 8 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConns: 8}
	return f
}

func (f *Fetcher) control(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !f.allow(ip) {
		return &protocol.Error{Code: protocol.CodeForbiddenTarget, Message: "音乐来源必须解析到公网地址"}
	}
	return nil
}

var standard = New()

func Fetch(ctx context.Context, raw json.RawMessage) (any, error) { return standard.Fetch(ctx, raw) }

func (f *Fetcher) Fetch(ctx context.Context, raw json.RawMessage) (any, error) {
	var p protocol.NetFetchParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, &protocol.Error{Code: protocol.CodeBadParams, Message: "请求参数格式不对"}
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutMs)*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: f.transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return &protocol.Error{Code: protocol.CodeBadParams, Message: "重定向超过 5 次"}
		}
		return protocol.ValidateNetFetchURL(r.URL.String())
	}}
	resp, err := client.Do(req)
	if err != nil {
		var pe *protocol.Error
		if errors.As(err, &pe) {
			return nil, pe
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, &protocol.Error{Code: protocol.CodeTimeout, Message: "请求音乐来源超时"}
		}
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(p.MaxBytes)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > p.MaxBytes {
		return nil, &protocol.Error{Code: protocol.CodeBadParams, Message: "音乐来源响应超过大小限制"}
	}
	return protocol.NetFetchResult{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: body}, nil
}
