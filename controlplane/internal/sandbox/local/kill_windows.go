//go:build windows

package local

import (
	"os/exec"
	"time"
)

func configureKill(c *exec.Cmd) {
	c.WaitDelay = 2 * time.Second
}
