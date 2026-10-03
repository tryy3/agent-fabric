//go:build !windows

package local

import (
	"os/exec"
	"syscall"
	"time"
)

// configureKill runs the command in its own process group and kills the whole
// group on cancel or timeout, so children of the command do not outlive it.
func configureKill(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	c.Cancel = func() error {
		if c.Process == nil {
			return nil
		}
		return syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	}
	c.WaitDelay = 2 * time.Second
}
