//go:build !windows

package setup

import "errors"

// IsSetup is always false outside Windows.
func IsSetup() bool { return false }

// Run is only for Windows.
func Run(Options) error { return errors.New("the setup program only runs on Windows") }
