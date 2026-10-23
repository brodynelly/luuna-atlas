//go:build windows

package atlas

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureHookProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
func killHookProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		killer := exec.Command("taskkill", "/pid", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		killer.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = killer.Run()
		_ = cmd.Process.Kill()
	}
}
