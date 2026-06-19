//go:build windows

// Package sysproc hides the console window that Windows would otherwise
// allocate for child processes spawned from a GUI (no-console) binary.
package sysproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is the CREATE_NO_WINDOW process creation flag. The child
// runs without a console of its own, so no window flashes on screen.
const createNoWindow = 0x08000000

// Hide configures cmd so spawning it does not pop up a console window.
// Must be called after the command is built and before it is started.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
