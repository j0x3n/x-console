//go:build windows

package coding

// groupGone is not checked on Windows; the process tests are skipped there.
func groupGone(int) bool { return true }
