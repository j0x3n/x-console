package files

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// B215：日历模块也用 Google 授权，只是权限范围不同。下面几个函数只是把
// Drive 用的那套地址和换令牌的代码开放出来，原来的函数不变。

// WithDefaults fills in the real Google addresses and the HTTP client.
func (e GoogleEndpoints) WithDefaults() GoogleEndpoints { return e.withDefaults() }

// GoogleAuthURLFor is the Google consent page asking for the given scopes.
func GoogleAuthURLFor(e GoogleEndpoints, clientID, redirect, state string, scopes ...string) string {
	e = e.withDefaults()
	q := url.Values{"client_id": {clientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {strings.Join(scopes, " ")}, "state": {state}, "access_type": {"offline"}, "prompt": {"consent"}}
	return e.Auth + "?" + q.Encode()
}

// GoogleRefresh trades a refresh token for an access token and tells when
// to fetch a new one. ErrGDriveAuth means the refresh token stopped working.
func GoogleRefresh(ctx context.Context, e GoogleEndpoints, clientID, secret, refresh string) (string, time.Time, error) {
	e = e.withDefaults()
	out, err := postToken(ctx, e, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh},
		"client_id": {clientID}, "client_secret": {secret}})
	if err != nil {
		return "", time.Time{}, err
	}
	return out.AccessToken, time.Now().Add(time.Duration(max(out.ExpiresIn-60, 30)) * time.Second), nil
}
