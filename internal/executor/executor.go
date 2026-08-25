package executor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"
)

type Result struct {
	ExitCode        int
	Stdout          string
	Stderr          string
	Duration        time.Duration
	TimedOut        bool
	StdoutTruncated bool
	StderrTruncated bool
	Error           string
}

func Run(parent context.Context, command string, timeout time.Duration, outputLimit int64) Result {
	started := time.Now()
	result := Result{ExitCode: -1}
	if timeout <= 0 {
		result.Error = "invalid command timeout"
		return result
	}
	if outputLimit <= 0 {
		result.Error = "invalid output limit"
		return result
	}

	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	name, args := shellCommand(command)
	cmd := exec.CommandContext(ctx, name, args...)
	configureCommand(cmd)
	stdout := newLimitedBuffer(outputLimit)
	stderr := newLimitedBuffer(outputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	result.Duration = time.Since(started)
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.StdoutTruncated = stdout.Truncated()
	result.StderrTruncated = stderr.Truncated()
	result.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)

	if err == nil {
		result.ExitCode = 0
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
		if result.TimedOut {
			result.Error = "command timed out"
		}
		return result
	}
	if result.TimedOut {
		result.Error = "command timed out"
	} else if errors.Is(ctx.Err(), context.Canceled) {
		result.Error = "command canceled"
	} else {
		result.Error = err.Error()
	}
	return result
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/D", "/S", "/C", command}
	}
	return "/bin/sh", []string{"-c", command}
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int64
	truncated bool
}

func newLimitedBuffer(limit int64) *limitedBuffer {
	return &limitedBuffer{remaining: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	if int64(len(p)) > b.remaining {
		p = p[:max(0, int(b.remaining))]
		b.truncated = true
	}
	if len(p) > 0 {
		_, _ = b.buffer.Write(p)
		b.remaining -= int64(len(p))
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string {
	return b.buffer.String()
}

func (b *limitedBuffer) Truncated() bool {
	return b.truncated
}
