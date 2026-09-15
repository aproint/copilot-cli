//go:build !windows

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"os"
	"os/exec"
	"time"
)

const interactiveCancelGracePeriod = 2 * time.Second

// InteractiveRun runs the input command that starts a child process.
func (c *Cmd) InteractiveRun(name string, args []string) error {
	return c.InteractiveRunWithContext(context.Background(), name, args)
}

// InteractiveRunWithContext runs the input command with ctx.
func (c *Cmd) InteractiveRunWithContext(ctx context.Context, name string, args []string) error {
	cmd := c.command(ctx, name, args,
		Stdout(os.Stdout),
		Stdin(os.Stdin),
		Stderr(os.Stderr),
		interruptBeforeKill(interactiveCancelGracePeriod))
	return runWithTerminalRestore(cmd.Run)
}

func interruptBeforeKill(gracePeriod time.Duration) CmdOption {
	return func(cmd *exec.Cmd) {
		cmd.Cancel = func() error {
			return cmd.Process.Signal(os.Interrupt)
		}
		cmd.WaitDelay = gracePeriod
	}
}
