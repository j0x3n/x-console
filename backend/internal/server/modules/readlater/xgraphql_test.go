package readlater_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/j0x3n/x-console/backend/internal/server/contracts"
	"github.com/j0x3n/x-console/backend/internal/server/module"
)

// B207: the X watchers call X through readlater with the saved cookie.
func TestXGraphQLForOtherModules(t *testing.T) {
	env, _, f := setupX(t)
	x, ok := module.Lookup[contracts.XGraphQL](env.App.Deps.Registry, contracts.XGraphQLKey)
	if !ok {
		t.Fatal("XGraphQL not provided")
	}
	if x.Ready(t.Context()) {
		t.Fatal("ready without a cookie")
	}
	if _, err := x.Query(t.Context(), "abcdefgh12", "UserTweets", `{}`, `{}`); !errors.Is(err, contracts.ErrXAuth) {
		t.Fatalf("query without a cookie: %v", err)
	}
	env.Elevate()
	env.MustDo(http.MethodPut, "/readlater/x-auth", map[string]any{"authToken": "tok-ABC123def456", "ct0": "csrf-XYZ789abc012"}, nil)
	if !x.Ready(t.Context()) {
		t.Fatal("not ready with a cookie")
	}
	f.mu.Lock()
	f.gql = `{"data":{"ok":true}}`
	f.mu.Unlock()
	body, err := x.Query(t.Context(), "abcdefgh12", "UserTweets", `{"userId":"1"}`, `{"a":true}`)
	if err != nil || !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("query: %s %v", body, err)
	}
	f.mu.Lock()
	path, cookie, csrf := f.gqlPaths[len(f.gqlPaths)-1], f.cookies[len(f.cookies)-1], f.csrf[len(f.csrf)-1]
	f.mu.Unlock()
	if path != "/graphql/abcdefgh12/UserTweets" || !strings.Contains(cookie, "auth_token=tok-ABC123def456") || csrf != "csrf-XYZ789abc012" {
		t.Fatalf("request: %s %q %q", path, cookie, csrf)
	}
	if _, err := x.Query(t.Context(), "bad/id", "UserTweets", `{}`, ""); err == nil {
		t.Fatal("a query id with a slash went through")
	}

	// X refuses the cookie: the caller gets ErrXAuth and the cookie is marked expired.
	f.mu.Lock()
	f.gqlCode = http.StatusUnauthorized
	f.mu.Unlock()
	if _, err := x.Query(t.Context(), "abcdefgh12", "UserTweets", `{}`, ""); !errors.Is(err, contracts.ErrXAuth) {
		t.Fatalf("refused cookie: %v", err)
	}
	if x.Ready(t.Context()) {
		t.Fatal("still ready after X refused the cookie")
	}
	var st struct{ Status string }
	env.MustDo(http.MethodGet, "/readlater/x-auth", nil, &st)
	if st.Status != "expired" {
		t.Fatalf("x-auth status %q", st.Status)
	}
}
