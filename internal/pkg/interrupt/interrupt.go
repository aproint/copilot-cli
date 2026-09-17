// Copyright APROINT, s.r.o.
// SPDX-License-Identifier: Apache-2.0

// Package interrupt carries the root command's first-interrupt notification through context.
package interrupt

import (
	"context"
	"sync"
)

type contextKey struct{}

type notifier struct {
	done chan struct{}
	once sync.Once
}

// returns a context that exposes an interrupt notification and an idempotent notifier.
func WithContext(parent context.Context) (context.Context, func()) {
	n := &notifier{done: make(chan struct{})}
	ctx := context.WithValue(parent, contextKey{}, (<-chan struct{})(n.done))
	return ctx, func() {
		n.once.Do(func() {
			close(n.done)
		})
	}
}

// FromContext returns the interrupt notification carried by ctx, or nil when none is present.
func FromContext(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	done, _ := ctx.Value(contextKey{}).(<-chan struct{})
	return done
}
