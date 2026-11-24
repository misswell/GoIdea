//go:build !unix

package app

import (
	"context"
	"os"
	"os/exec"
	"time"
)

func remoteCommandContext(ctx context.Context, program string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, program, args...)
	configureRemoteCommand(cmd)
	return cmd
}

func configureRemoteCommand(cmd *exec.Cmd) {
	if cmd != nil && cmd.WaitDelay == 0 {
		cmd.WaitDelay = 2 * time.Second
	}
}

func terminateRemoteCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
