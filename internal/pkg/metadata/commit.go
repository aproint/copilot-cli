// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package metadata contains helpers for Copilot metadata commit operations.
package metadata

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// CommitTimeout bounds short metadata writes that must complete after a durable
	// infrastructure mutation has already succeeded.
	CommitTimeout = 30 * time.Second

	// CommitAfterCancellationWarning is shown when a required metadata commit
	// continues after the caller context is canceled.
	CommitAfterCancellationWarning = "Command canceled; finishing required metadata write (30s timeout)."
)

// Commit performs a required metadata write after a durable infrastructure
// mutation. The write is detached from caller cancellation, bounded by
// CommitTimeout, and wrapped as a partial-success error if it fails.
func Commit(parent context.Context, mutation string, warn func(string), write func(context.Context) error) error {
	return commitWithTimeout(parent, mutation, CommitTimeout, warn, write)
}

func commitWithTimeout(parent context.Context, mutation string, timeout time.Duration, warn func(string), write func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	defer cancel()

	var warnOnce sync.Once
	reportCancellation := func() {
		warnOnce.Do(func() {
			warn(CommitAfterCancellationWarning)
		})
	}
	var completionMu sync.Mutex
	writeComplete := false
	watcherDone := make(chan struct{})
	// Serialize cancellation observation with the completion transition. The
	// parent may be canceled before its AfterFunc callback gets scheduled, so the
	// completion path also samples parent.Err while holding the same lock.
	stopWatcher := context.AfterFunc(parent, func() {
		defer close(watcherDone)
		completionMu.Lock()
		defer completionMu.Unlock()
		if !writeComplete {
			reportCancellation()
		}
	})

	err := write(ctx)
	completionMu.Lock()
	if parent.Err() != nil {
		reportCancellation()
	}
	writeComplete = true
	completionMu.Unlock()
	if stopWatcher() {
		close(watcherDone)
	}
	<-watcherDone
	return NewCommitError(mutation, err)
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
