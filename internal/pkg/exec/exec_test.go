// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"os"
	osexec "os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/golang/mock/gomock"
)

func TestCmd_Run(t *testing.T) {
	t.Run("should delegate to exec and call Run", func(t *testing.T) {
		// GIVEN
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cmd := &Cmd{
			command: func(ctx context.Context, name string, args []string, opts ...CmdOption) cmdRunner {
				require.Equal(t, "ls", name)
				m := NewMockcmdRunner(ctrl)
				m.EXPECT().Run().Return(nil)
				return m
			},
		}

		// WHEN
		err := cmd.Run("ls", nil)

		// THEN
		require.NoError(t, err)
	})
}

func TestCmd_RunWithContext(t *testing.T) {
	t.Run("should delegate to exec and call Run", func(t *testing.T) {
		// GIVEN
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		cmd := &Cmd{
			command: func(ctx context.Context, name string, args []string, opts ...CmdOption) cmdRunner {
				require.Equal(t, "ls", name)
				m := NewMockcmdRunner(ctrl)
				m.EXPECT().Run().Return(nil)
				return m
			},
		}

		// WHEN
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := cmd.RunWithContext(ctx, "ls", nil)

		// THEN
		require.NoError(t, err)
	})
}

func TestCmd_InteractiveRunWithContextUsesCallerContextAndTerminalStreams(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.WithValue(context.Background(), struct{}{}, "caller context")
	cmd := &Cmd{
		command: func(gotCtx context.Context, name string, args []string, opts ...CmdOption) cmdRunner {
			require.Same(t, ctx, gotCtx)
			require.Equal(t, "session-manager-plugin", name)
			require.Equal(t, []string{"session"}, args)
			process := &osexec.Cmd{}
			for _, opt := range opts {
				opt(process)
			}
			require.Same(t, os.Stdin, process.Stdin)
			require.Same(t, os.Stdout, process.Stdout)
			require.Same(t, os.Stderr, process.Stderr)
			runner := NewMockcmdRunner(ctrl)
			runner.EXPECT().Run().Return(nil)
			return runner
		},
	}

	err := cmd.InteractiveRunWithContext(ctx, "session-manager-plugin", []string{"session"})
	require.NoError(t, err)
}
