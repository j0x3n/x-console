//go:build !windows

package setup

import "errors"

// IsSetup is always false outside Windows.
func IsSetup() bool { return false }

// Run is only for Windows.
func Run(Options) error { return errors.New("the setup program only runs on Windows") }

// ExitRevoked is the exit code of "run" when the panel rejected the token.
const ExitRevoked = 3

// ErrRevoked means the agent stopped at once because its token was rejected.
var ErrRevoked = errors.New("the panel rejected the agent token; pair again")

// Background is only for Windows. Linux servers use install.sh (systemd).
func Background(string) (string, error) {
	return "", errors.New("running in the background is only built in on Windows; on Linux use the install script")
}
