//go:build !windows

package terminal

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func shellCommand(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh")
}

func initialInput() []byte { return nil }

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
