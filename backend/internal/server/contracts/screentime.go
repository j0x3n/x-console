package contracts

import "context"

// ScreenTimeAppsKey is where the screentime module registers ScreenTimeApps (B131).
const ScreenTimeAppsKey = "screentime.apps"

// ScreenTimeApps lets other modules use the programs the computer time
// sorted into a category.
type ScreenTimeApps interface {
	// AppsOfCategory returns the programs that the program name alone puts in
	// the category (the user's own rules first, then the built-in ones), among
	// the programs seen so far. Browsers sorted by window title are not in it.
	AppsOfCategory(ctx context.Context, category string) ([]string, error)
}
