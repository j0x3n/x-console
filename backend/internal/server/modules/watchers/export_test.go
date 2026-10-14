package watchers

import (
	"context"
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
