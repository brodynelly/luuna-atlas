//go:build windows

package atlas

import (
	"os"
	"syscall"
	"unsafe"
)

func stdoutIsTerminal() bool {
	var mode uint32
	r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode").Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&mode)))
	return r != 0
}
func windowsTerminalColorLevel() int {
	var version struct {
		Size, Major, Minor, Build, Platform uint32
		ServicePack                         [128]uint16
	}
	version.Size = uint32(unsafe.Sizeof(version))
	result, _, _ := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlGetVersion").Call(uintptr(unsafe.Pointer(&version)))
	if result == 0 && version.Major >= 10 && version.Build >= 10586 {
		if version.Build >= 14931 {
			return 3
		}
		return 2
	}
	return 1
}
