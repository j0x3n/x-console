package app

import (
	"github.com/j0x3n/x-console/backend/internal/server/module"
	"github.com/j0x3n/x-console/backend/internal/server/modules/notes"
	"github.com/j0x3n/x-console/backend/internal/server/modules/projects"
)

// constructors lists every feature module. Add one line per module:
//
//	projects.New,
//
// Keep the list sorted by module number (M1..M13) to make merges easy.
var constructors = []func(*module.Deps) (module.Module, error){
	projects.New, // M5
	notes.New,    // M6
}
