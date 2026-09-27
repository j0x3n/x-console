package app

import (
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/brief"
	"github.com/j0x3n/x-console/backend/internal/server/modules/calendar"
	"github.com/j0x3n/x-console/backend/internal/server/modules/coding"
	"github.com/j0x3n/x-console/backend/internal/server/modules/dashboard"
	"github.com/j0x3n/x-console/backend/internal/server/modules/focus"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear"
	"github.com/j0x3n/x-console/backend/internal/server/modules/monitoring"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects"
	"github.com/j0x3n/x-console/backend/internal/server/modules/reminders"
)

// constructors lists every feature module. Add one line per module:
//
//	projects.New,
//
// Keep the list sorted by module number (M1..M13) to make merges easy.
var constructors = []func(*module.Deps) (module.Module, error){
	dashboard.New,     // M1
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
	github.New,        // M13
	linear.New,        // M13
}
