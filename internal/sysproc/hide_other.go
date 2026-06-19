//go:build !windows

package sysproc

import "os/exec"

// Hide is a no-op on platforms where child processes do not allocate a
// console window.
func Hide(cmd *exec.Cmd) {}
