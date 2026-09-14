// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package main contains the root command.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/aproint/copilot-cli/cmd/copilot/template"
	"github.com/aproint/copilot-cli/internal/pkg/cli"
	"github.com/aproint/copilot-cli/internal/pkg/interrupt"
	"github.com/aproint/copilot-cli/internal/pkg/term/color"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	"github.com/aproint/copilot-cli/internal/pkg/version"
	"github.com/spf13/cobra"
)

type actionRecommender interface {
	RecommendActions() string
}

type exitCodeError interface {
	ExitCode() int
}

func init() {
	color.DisableColorBasedOnEnvVar()
	cobra.EnableCommandSorting = false // Maintain the order in which we add commands.
}

func main() {
	ctx, stop := rootContext()
	defer stop()

	cmd := buildRootCmd()
	if err := cmd.ExecuteContext(ctx); err != nil {
		var ac actionRecommender
		var exitCodeErr exitCodeError

		if errors.As(err, &ac) {
			log.Infoln(ac.RecommendActions())
		}
		if errors.As(err, &exitCodeErr) {
			log.Infoln(err.Error())
			os.Exit(exitCodeErr.ExitCode())
		}
		log.Errorln(err.Error())
		os.Exit(1)
	}
}

func rootContext() (context.Context, func()) {
	return rootContextWithSignals(signal.Notify, signal.Stop, os.Exit)
}

func rootContextWithSignals(
	notify func(chan<- os.Signal, ...os.Signal),
	stopSignals func(chan<- os.Signal),
	exit func(int),
) (context.Context, func()) {
	interruptCtx, notifyInterrupt := interrupt.WithContext(context.Background())
	ctx, cancel := context.WithCancel(interruptCtx)
	sigCh := make(chan os.Signal, 2)
	done := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once

	notify(sigCh, os.Interrupt, syscall.SIGTERM)
	wg.Add(1)
	go func() {
		defer wg.Done()
		handleRootSignals(sigCh, done, notifyInterrupt, cancel, exit)
	}()

	shutdown := func() {
		once.Do(func() {
			stopSignals(sigCh)
			close(done)
			cancel()
			wg.Wait()
		})
	}
	return ctx, shutdown
}

func handleRootSignals(signals <-chan os.Signal, done <-chan struct{}, notifyInterrupt func(), cancel context.CancelFunc, exit func(int)) {
	seenSignal := false
	for {
		select {
		case <-done:
			return
		default:
		}

		select {
		case <-done:
			return
		case sig := <-signals:
			if !seenSignal {
				seenSignal = true
				if sig == os.Interrupt {
					notifyInterrupt()
				}
				cancel()
				continue
			}
			exit(128 + int(sig.(syscall.Signal)))
			return
		}
	}
}

func buildRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "copilot",
		Short: shortDescription,
		Example: `
  Displays the help menu for the "init" command.
  /code $ copilot init --help`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// If we don't set a Run() function the help menu doesn't show up.
			// See https://github.com/spf13/cobra/issues/790
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.SetOut(log.OutputWriter)
	cmd.SetErr(log.DiagnosticWriter)

	// Sets version for --version flag. Version command gives more detailed
	// version information.
	cmd.Version = version.Version
	cmd.SetVersionTemplate("copilot version: {{.Version}}\n")

	// NOTE: Order for each grouping below is significant in that it affects help menu output ordering.
	// "Getting Started" command group.
	cmd.AddCommand(cli.BuildInitCmd())
	cmd.AddCommand(cli.BuildDocsCmd())

	// "Develop" command group.
	cmd.AddCommand(cli.BuildAppCmd())
	cmd.AddCommand(cli.BuildEnvCmd())
	cmd.AddCommand(cli.BuildSvcCmd())
	cmd.AddCommand(cli.BuildJobCmd())
	cmd.AddCommand(cli.BuildTaskCmd())
	cmd.AddCommand(cli.BuildRunLocalCmd())

	// "Extend" command group
	cmd.AddCommand(cli.BuildStorageCmd())
	cmd.AddCommand(cli.BuildSecretCmd())

	// "Settings" command group.
	cmd.AddCommand(cli.BuildVersionCmd())
	cmd.AddCommand(cli.BuildCompletionCmd(cmd))

	// "Release" command group.
	cmd.AddCommand(cli.BuildPipelineCmd())
	cmd.AddCommand(cli.BuildDeployCmd())

	cmd.SetUsageTemplate(template.RootUsage)
	return cmd
}
