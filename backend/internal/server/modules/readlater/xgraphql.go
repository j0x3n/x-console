package readlater

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// B207: other modules (the X watchers) call X with the cookie saved here.

type xGraphQL struct{ m *Module }

var _ contracts.XGraphQL = xGraphQL{}

func (x xGraphQL) Ready(ctx context.Context) bool {
	if _, ok := x.m.xCookie(ctx); !ok {
		return false
	}
	return x.m.xConfig(ctx).Status != "expired"
}

func validGraphQLName(s string) bool {
	return s != "" && len(s) <= 64 && strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-") == ""
}

func (x xGraphQL) Query(ctx context.Context, queryID, operation, variables, features string) ([]byte, error) {
	m := x.m
	c, ok := m.xCookie(ctx)
	if !ok {
		return nil, contracts.ErrXAuth
	}
	if !validGraphQLName(queryID) || !validGraphQLName(operation) {
		return nil, errors.New("queryId 或接口名里有不能用的字符")
	}
	q := url.Values{"variables": {variables}}
	if features != "" {
		q.Set("features", features)
	}
	target := m.xURL("graphql") + "/" + queryID + "/" + operation + "?" + q.Encode()
	if err := m.cookieHostOK(target); err != nil {
		return nil, err
	}
	head := http.Header{}
	head.Set("Authorization", "Bearer "+webBearer)
	head.Set("X-Csrf-Token", c.CT0)
	head.Set("Cookie", "auth_token="+c.AuthToken+"; ct0="+c.CT0)
	head.Set("X-Twitter-Auth-Type", "OAuth2Session")
	head.Set("X-Twitter-Active-User", "yes")
	head.Set("X-Twitter-Client-Language", "en")
	body, err := m.xGet(ctx, target, head)
	if errors.Is(err, errXAuth) {
		m.xExpired(ctx, m.xConfig(ctx))
		return nil, contracts.ErrXAuth
	}
	return body, err
}
