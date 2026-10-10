//go:build !windows

package screentime

// Available reports whether this system can read the foreground window.
func Available() bool { return false }

func foreground() (Window, bool) { return Window{}, false }
