// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package metadata contains helpers for Copilot metadata commit operations.
package metadata

import (
	"context"
	"fmt"
	"time"
)

const (
	// CommitTimeout bounds short metadata writes that must complete after a durable
	// infrastructure mutation has already succeeded.
	CommitTimeout = 30 * time.Second

	// CommitAfterCancellationWarning is shown when a required metadata commit is
	// started after the caller context has already been canceled.
	CommitAfterCancellationWarning = "Command was canceled; finishing required Copilot metadata write with a 30s timeout."
)

// CommitContext returns a short, detached context for required metadata writes
// after a durable infrastructure mutation has already succeeded.
//
// Use this only for short metadata commits that preserve Copilot consistency
// after infrastructure has been durably changed. Pre-mutation reads, validation,
// and normal command-scoped work should continue to use the caller context.
func CommitContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), CommitTimeout)
}

// CommitError reports that a durable mutation succeeded but the required Copilot
// metadata commit failed.
type CommitError struct {
	Mutation string
	Err      error
}

func (e *CommitError) Error() string {
	return fmt.Sprintf("%s succeeded, but Copilot metadata commit failed: %v", e.Mutation, e.Err)
}

func (e *CommitError) Unwrap() error {
	return e.Err
}

// NewCommitError wraps a metadata commit failure after mutation has succeeded.
func NewCommitError(mutation string, err error) error {
	if err == nil {
		return nil
	}
	return &CommitError{
		Mutation: mutation,
		Err:      err,
	}
}
