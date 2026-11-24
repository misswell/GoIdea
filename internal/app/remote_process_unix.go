//go:build unix

package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func remoteCommandContext(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	configureRemoteCommand(cmd)
	return cmd
}

func configureRemoteCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error { return terminateRemoteCommand(cmd) }
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = 2 * time.Second
	}
}

func terminateRemoteCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
