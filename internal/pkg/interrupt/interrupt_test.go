// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package interrupt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromContext(t *testing.T) {
	t.Run("returns nil without an interrupt notifier", func(t *testing.T) {
		require.Nil(t, FromContext(context.Background()))
		require.Nil(t, FromContext(nil))
	})

	t.Run("propagates through child contexts", func(t *testing.T) {
		ctx, notify := WithContext(context.Background())
		child, cancel := context.WithCancel(ctx)
		defer cancel()

		done := FromContext(child)
		require.NotNil(t, done)
		notify()
		requireClosed(t, done)
	})

	t.Run("broadcasts one idempotent notification", func(t *testing.T) {
		ctx, notify := WithContext(context.Background())
		first := FromContext(ctx)
		second := FromContext(ctx)

		notify()
		notify()

		requireClosed(t, first)
		requireClosed(t, second)
	})
}

func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	default:
		t.Fatal("interrupt notification is not closed")
	}
}
