//go:build !windows

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"os"
	"os/signal"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type contextRunner struct {
	ctx     context.Context
	started chan<- struct{}
}

func (r contextRunner) Run() error {
	close(r.started)
	<-r.ctx.Done()
	return r.ctx.Err()
}

func TestCmd_InteractiveRunWithContextKeepsRootInterruptNotification(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	t.Cleanup(func() { signal.Stop(interrupts) })
	go func() {
		select {
		case <-interrupts:
			cancel()
		case <-ctx.Done():
		}
	}()

	started := make(chan struct{})
	done := make(chan error, 1)
	cmd := &Cmd{
		command: func(gotCtx context.Context, _ string, _ []string, _ ...CmdOption) cmdRunner {
			return contextRunner{ctx: gotCtx, started: started}
		},
	}
	go func() {
		done <- cmd.InteractiveRunWithContext(ctx, "session-manager-plugin", nil)
	}()
	<-started

	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(os.Interrupt))

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("interactive run suppressed the root interrupt notification")
	}
}
