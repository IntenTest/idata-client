//go:build windows

package executor

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const createNewProcessGroup = 0x00000200

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		// taskkill is included with supported Windows editions and terminates the
		// cmd.exe process tree created for this command.
		kill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			if processErr := cmd.Process.Kill(); processErr != nil && !errors.Is(processErr, os.ErrProcessDone) {
				return processErr
			}
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
}
