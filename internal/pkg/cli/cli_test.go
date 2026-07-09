// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

type contextRecorderCmd struct {
	askCtx     context.Context
	executeCtx context.Context
}

func (c *contextRecorderCmd) Validate() error {
	return nil
}

func (c *contextRecorderCmd) Ask(ctx context.Context) error {
	c.askCtx = ctx
	return nil
}

func (c *contextRecorderCmd) Execute(ctx context.Context) error {
	c.executeCtx = ctx
	return nil
}

func TestRunPassesCobraContextToCommand(t *testing.T) {
	expectedCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recorder := &contextRecorderCmd{}
	cmd := &cobra.Command{
		Use: "test",
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), recorder)
		}),
	}

	if err := cmd.ExecuteContext(expectedCtx); err != nil {
		t.Fatalf("execute command: %v", err)
	}

	if recorder.askCtx != expectedCtx {
		t.Fatalf("expected Ask to receive Cobra context")
	}
	if recorder.executeCtx != expectedCtx {
		t.Fatalf("expected Execute to receive Cobra context")
	}
}
