//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package atlas

import (
	"os"
	"syscall"
	"unsafe"
)

func stdoutIsTerminal() bool {
	var size [4]uint16
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&size)))
	return errno == 0
}
func windowsTerminalColorLevel() int { return 1 }
