// Copyright APROINT, s.r.o.
// SPDX-License-Identifier: Apache-2.0

package exec

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCmd_InteractiveRunTerminatesProcessOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tempDir := t.TempDir()
	readyFile := filepath.Join(tempDir, "ready")
	cleanupFile := filepath.Join(tempDir, "cleanup")
	done := make(chan error, 1)
	go func() {
		done <- NewCmd().InteractiveRun(ctx, os.Args[0], []string{
			"-test.run=^TestInteractiveRunHelperProcess$", "--", readyFile, cleanupFile,
		})
	}()

	require.Eventually(t, func() bool {
		_, err := os.Stat(readyFile)
		return err == nil
	}, 2*time.Second, 10*time.Millisecond, "interactive child process did not start")
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("interactive process did not terminate after cancellation")
	}
	if runtime.GOOS != "windows" {
		_, err := os.Stat(cleanupFile)
		require.NoError(t, err, "interactive child did not receive a graceful interrupt")
	}
}

func TestInteractiveRunHelperProcess(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator == -1 || separator+2 >= len(os.Args) {
		return
	}
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	if err := os.WriteFile(os.Args[separator+1], []byte("ready"), 0o600); err != nil {
		os.Exit(2)
	}
	select {
	case <-interrupts:
		if err := os.WriteFile(os.Args[separator+2], []byte("cleanup"), 0o600); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	case <-time.After(30 * time.Second):
		os.Exit(4)
	}
}
