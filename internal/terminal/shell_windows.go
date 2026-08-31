//go:build windows

package terminal

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

func shellCommand(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "cmd.exe", "/D", "/Q")
}

func initialInput() []byte { return []byte("chcp 65001>nul\r\n") }

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNewProcessGroup | createNoWindow}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		kill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
		if err := kill.Run(); err != nil {
			if processErr := cmd.Process.Kill(); processErr != nil && !errors.Is(processErr, os.ErrProcessDone) {
				return processErr
			}
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
}
