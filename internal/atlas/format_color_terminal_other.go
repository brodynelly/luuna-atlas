//go:build !windows && !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package atlas

import "os"

func stdoutIsTerminal() bool {
	st, e := os.Stdout.Stat()
	return e == nil && st.Mode()&os.ModeCharDevice != 0
}
func windowsTerminalColorLevel() int { return 1 }
