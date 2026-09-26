package app

import (
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/github"
	"github.com/j0x3n/x-console/backend/internal/server/modules/habits"
	"github.com/j0x3n/x-console/backend/internal/server/modules/homeassistant"
	"github.com/j0x3n/x-console/backend/internal/server/modules/hosts"
	"github.com/j0x3n/x-console/backend/internal/server/modules/linear"
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
	hosts.New,         // M2/M3
	projects.New,      // M5
	notes.New,         // M6
	reminders.New,     // M7
	habits.New,        // M8
	homeassistant.New, // M9
	github.New,        // M13
	linear.New,        // M13
}
