//go:build !windows

package auth

import (
	"os/exec"
	"syscall"
)

// configureCmd puts the child in its own process group and kills the whole
// group on timeout or cancellation.
func configureCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

// checkResolved has no platform restrictions outside Windows.
func checkResolved(string, []string) error { return nil }
