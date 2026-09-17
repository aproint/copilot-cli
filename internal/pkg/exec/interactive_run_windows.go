// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const interactiveCancelGracePeriod = 2 * time.Second

// InteractiveRun runs the input command with ctx.
func (c *Cmd) InteractiveRun(ctx context.Context, name string, args []string) error {
	cmd := c.command(ctx, name, args,
		Stdout(os.Stdout),
		Stdin(os.Stdin),
		Stderr(os.Stderr),
		interruptBeforeKill(interactiveCancelGracePeriod))
	return runWithTerminalRestore(cmd.Run)
}

func interruptBeforeKill(gracePeriod time.Duration) CmdOption {
	return func(cmd *exec.Cmd) {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
		cmd.Cancel = func() error {
			if err := windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)); err != nil {
				return cmd.Process.Kill()
			}
			return nil
		}
		cmd.WaitDelay = gracePeriod
	}
}
