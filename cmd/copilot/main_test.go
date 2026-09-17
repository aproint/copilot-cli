// Copyright APROINT, s.r.o.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/interrupt"
	"github.com/stretchr/testify/require"
)

func TestHandleRootSignals(t *testing.T) {
	tests := map[string]struct {
		signal          os.Signal
		exitCode        int
		notifyInterrupt bool
	}{
		"SIGINT":  {signal: os.Interrupt, exitCode: 130, notifyInterrupt: true},
		"SIGTERM": {signal: syscall.SIGTERM, exitCode: 143},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			interruptCtx, notifyInterrupt := interrupt.WithContext(context.Background())
			ctx, cancel := context.WithCancel(interruptCtx)
			signals := make(chan os.Signal, 2)
			done := make(chan struct{})
			exited := make(chan int, 1)
			interruptBeforeCancel := make(chan bool, 1)
			go handleRootSignals(signals, done, notifyInterrupt, func() {
				select {
				case <-interrupt.FromContext(ctx):
					interruptBeforeCancel <- true
				default:
					interruptBeforeCancel <- false
				}
				cancel()
			}, func(code int) {
				exited <- code
			})

			signals <- tc.signal
			require.Eventually(t, func() bool {
				return ctx.Err() == context.Canceled
			}, time.Second, time.Millisecond)
			require.Equal(t, tc.notifyInterrupt, <-interruptBeforeCancel)
			if tc.notifyInterrupt {
				requireClosed(t, interrupt.FromContext(ctx))
			} else {
				requireOpen(t, interrupt.FromContext(ctx))
			}
			select {
			case code := <-exited:
				t.Fatalf("first signal unexpectedly requested exit code %d", code)
			default:
			}

			signals <- tc.signal
			require.Equal(t, tc.exitCode, <-exited)
		})
	}
}

func TestRootContextShutdown(t *testing.T) {
	var registered []os.Signal
	var stopped chan<- os.Signal
	stopCalls := 0
	ctx, shutdown := rootContextWithSignals(
		func(ch chan<- os.Signal, signals ...os.Signal) {
			registered = append(registered, signals...)
		},
		func(ch chan<- os.Signal) {
			stopped = ch
			stopCalls++
		},
		func(code int) {
			t.Fatalf("normal shutdown unexpectedly requested exit code %d", code)
		},
	)

	require.ElementsMatch(t, []os.Signal{os.Interrupt, syscall.SIGTERM}, registered)
	shutdown()
	shutdown()

	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.NotNil(t, stopped)
	require.Equal(t, 1, stopCalls)
	requireOpen(t, interrupt.FromContext(ctx))
}

func requireClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	default:
		t.Fatal("channel is not closed")
	}
}

func requireOpen(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("channel is closed")
	default:
	}
}
