// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package main contains the root command.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/aproint/copilot-cli/cmd/copilot/template"
	"github.com/aproint/copilot-cli/internal/pkg/cli"
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

const sigtermExitCode = 128 + int(syscall.SIGTERM)

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
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	stop := func() {
		signal.Stop(sigCh)
		cancel()
	}
	go func() {
		<-sigCh
		cancel()
		signal.Stop(sigCh)
		os.Exit(sigtermExitCode)
	}()
	return ctx, stop
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
