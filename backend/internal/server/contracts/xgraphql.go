package contracts

import (
	"context"
	"errors"
)

// XGraphQLKey is where the readlater module provides XGraphQL (B207). It
// holds the user's X login cookie (B143); other modules call X through it
// and never see the cookie.
const XGraphQLKey = "readlater.xgraphql"

// ErrXAuth means X refused the cookie, or none is saved. The readlater
// module marks the cookie as expired and tells the user once a day.
var ErrXAuth = errors.New("X 的登录 Cookie 没填或者过期了，去 设置 → 稍后读 重新填")

// XGraphQL calls the web interface of X with the saved cookie.
type XGraphQL interface {
	// Ready reports whether a cookie is saved and not known to be expired.
	Ready(ctx context.Context) bool
	// Query sends GET /i/api/graphql/<queryID>/<operation> with these
	// variables and features (JSON objects) and returns the answer body.
	Query(ctx context.Context, queryID, operation, variables, features string) ([]byte, error)
}
