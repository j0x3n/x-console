package watchers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/j0x3n/x-console/backend/internal/server/browser"
	"github.com/j0x3n/x-console/backend/internal/server/contracts"
)

// Hooks for the integration tests in package watchers_test, which cannot live
// in this package because testutil imports app, which imports watchers.

func SetBrowser(m *Module, b *browser.Service) { m.browser = b }

func SetChat(m *Module, c contracts.ToolChat) { m.chatOverride = c }

func SetLLM(m *Module, l contracts.LLM) { m.llmOverride = l }

func SetWait(m *Module, d time.Duration) { m.waitFor = d }

func Tick(m *Module, ctx context.Context) error { return m.tick(ctx) }

func Cleanup(m *Module, ctx context.Context) error { return m.cleanup(ctx) }

// WaitRuns waits until no run is in progress.
func WaitRuns(m *Module) { m.bg.Wait() }

func Live(m *Module, id int64) *browser.Session { return m.browser.Live(sessionKey(id)) }

func BaseDomain(host string) string { return baseDomain(host) }

func SetX(m *Module, x contracts.XGraphQL) { m.xOverride = x }

// ParseXTarget exposes the target parser: kind, value, canonical address.
func ParseXTarget(in string) (string, string, string, error) {
	t, u, err := parseXTarget(in)
	return t.Kind, t.Value, u, err
}

// DetectKind exposes the kind detection: kind and the address to store.
func DetectKind(in string) (string, string, error) { return detectKind(in) }

// JudgeUser exposes the message the change judge gets.
func JudgeUser(goal, old, value string) string { return judgeUser(goal, old, value) }

// AgentSystem and AgentFirstMessage expose the exploration prompts.
func AgentSystem() string { return agentSystem }

func AgentFirstMessage(url, goal string, hasLogin bool, page string) string {
	return agentFirstMessage(url, goal, hasLogin, page)
}

// CollectPosts returns the ids and handles of the posts in an X answer.
func CollectPosts(body []byte) ([]string, error) {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []string
	for _, p := range collectPosts(doc) {
		out = append(out, p.ID+"@"+p.Handle)
	}
	return out, nil
}

// StartEgress starts the egress proxy on addr (use "127.0.0.1:0") until ctx
// ends, and points the proxy address given to the browser at it.
func StartEgress(m *Module, ctx context.Context, addr string) (string, error) {
	got, err := m.egress.start(ctx, addr)
	if err != nil {
		return "", err
	}
	m.egress.publicURL = "http://" + got.String()
	return got.String(), nil
}

// EgressProxyURL is the address the browser of watcher id is given.
func EgressProxyURL(m *Module, id int64) (string, error) { return m.egress.proxyURL(id) }
