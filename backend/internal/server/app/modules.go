package app

import (
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/ai"
	"github.com/j0x3n/x-console/backend/internal/server/modules/aiagents"
	"github.com/j0x3n/x-console/backend/internal/server/modules/automations"
	"github.com/j0x3n/x-console/backend/internal/server/modules/backup"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard"
	"github.com/j0x3n/x-console/backend/internal/server/modules/documents"
	"github.com/j0x3n/x-console/backend/internal/server/modules/drive"
	uploadfiles "github.com/j0x3n/x-console/backend/internal/server/modules/files"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mail"
	"github.com/j0x3n/x-console/backend/internal/server/modules/maintenance"
	"github.com/j0x3n/x-console/backend/internal/server/modules/mcp"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects"
	"github.com/j0x3n/x-console/backend/internal/server/modules/quotas"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
	"github.com/j0x3n/x-console/backend/internal/server/modules/router"
	"github.com/j0x3n/x-console/backend/internal/server/modules/storage"
	"github.com/j0x3n/x-console/backend/internal/server/modules/vault"
)

// constructors lists every feature module. Add one line per module:
//
//	projects.New,
//
// Keep the list sorted by module number (M1..M13) to make merges easy.
var constructors = []func(*module.Deps) (module.Module, error){
	dashboard.New,
	hosts.New,         // M2/M3
	coding.New,        // M4
	projects.New,      // M5
	notes.New,         // M6
	reminders.New,     // M7
	habits.New,        // M8
	homeassistant.New, // M9
	monitoring.New,    // M10
	calendar.New,      // M11
	focus.New,         // M11
	brief.New,         // M11
	ai.New,            // M12
	automations.New,   // M12
	github.New,        // M13
	linear.New,        // M13
	vault.New,
	mcp.New,     // B43
	drive.New,   // M14
	storage.New, // B24
	backup.New,  // B25

	uploadfiles.New, // B36

	aiagents.New, // B47
	mail.New,     // B53
	router.New,   // B65
	maintenance.New,
	quotas.New,    // B110
	documents.New, // B115
}
