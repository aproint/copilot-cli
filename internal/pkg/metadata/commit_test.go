// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package metadata_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/metadata"
	"github.com/stretchr/testify/require"
)

func TestCommitContext(t *testing.T) {
	type contextKey string
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), contextKey("key"), "value"))
	cancelParent()

	ctx, cancelCommit := metadata.CommitContext(parent)
	defer cancelCommit()

	require.NoError(t, ctx.Err())
	require.Equal(t, "value", ctx.Value(contextKey("key")))
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(metadata.CommitTimeout), deadline, time.Second)
}

func TestNewCommitError(t *testing.T) {
	root := errors.New("boom")

	err := metadata.NewCommitError("application infrastructure deployment", root)

	require.EqualError(t, err, "application infrastructure deployment succeeded, but Copilot metadata commit failed: boom")
	require.ErrorIs(t, err, root)
	var commitErr *metadata.CommitError
	require.ErrorAs(t, err, &commitErr)
}
