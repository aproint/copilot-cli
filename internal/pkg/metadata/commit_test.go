// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCommit(t *testing.T) {
	type contextKey string

	t.Run("normally completed write is bounded and does not warn", func(t *testing.T) {
		parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), contextKey("key"), "value"))
		var warnings []string

		err := Commit(parent, "application deployment", func(message string) {
			warnings = append(warnings, message)
		}, func(ctx context.Context) error {
			require.NoError(t, ctx.Err())
			require.Equal(t, "value", ctx.Value(contextKey("key")))
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(CommitTimeout), deadline, time.Second)
			return nil
		})

		require.NoError(t, err)
		cancelParent()
		require.Empty(t, warnings)
	})

	t.Run("already canceled parent still writes and warns once", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		var warnings []string
		wrote := false

		err := Commit(parent, "application deployment", func(message string) {
			warnings = append(warnings, message)
		}, func(ctx context.Context) error {
			wrote = true
			require.NoError(t, ctx.Err())
			return nil
		})

		require.NoError(t, err)
		require.True(t, wrote)
		require.Equal(t, []string{CommitAfterCancellationWarning}, warnings)
	})

	t.Run("cancellation during write does not cancel it and warns once", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		var warnings []string
		warned := make(chan struct{})

		err := Commit(parent, "environment deployment", func(message string) {
			warnings = append(warnings, message)
			close(warned)
		}, func(ctx context.Context) error {
			cancel()
			<-warned
			require.NoError(t, ctx.Err())
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, []string{CommitAfterCancellationWarning}, warnings)
	})

	t.Run("cancellation immediately before write returns still warns", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		var warnings []string

		err := Commit(parent, "environment deployment", func(message string) {
			warnings = append(warnings, message)
		}, func(context.Context) error {
			cancel()
			return nil
		})

		require.NoError(t, err)
		require.Equal(t, []string{CommitAfterCancellationWarning}, warnings)
	})

	t.Run("timeout cancels a stuck write", func(t *testing.T) {
		err := commitWithTimeout(context.Background(), "application deployment", time.Millisecond,
			func(string) {},
			func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			})

		require.ErrorIs(t, err, context.DeadlineExceeded)
		var commitErr *CommitError
		require.ErrorAs(t, err, &commitErr)
	})
}

func TestNewCommitError(t *testing.T) {
	root := errors.New("boom")

	err := NewCommitError("application infrastructure deployment", root)

	require.EqualError(t, err, "application infrastructure deployment succeeded, but Copilot metadata commit failed: boom")
	require.ErrorIs(t, err, root)
	var commitErr *CommitError
	require.ErrorAs(t, err, &commitErr)
}
