package app

import (
	"github.com/j0x3n/x-console/backend/internal/server/agenthub"
	"github.com/j0x3n/x-console/backend/internal/server/ws"
)

type maintenanceConnections struct {
	browser *ws.Handler
	agents  *agenthub.Hub
}

func (c maintenanceConnections) BrowserConnections() int { return c.browser.ConnectionCount() }
func (c maintenanceConnections) AgentConnections() int   { return c.agents.ConnectionCount() }
