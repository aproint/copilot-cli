//go:build !windows

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCmd_InteractiveRunWithContextTerminatesProcessOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- NewCmd().InteractiveRunWithContext(ctx, "sleep", []string{"30"})
	}()

	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("interactive process did not terminate after cancellation")
	}
}
