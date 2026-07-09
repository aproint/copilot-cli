// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/spf13/cobra"
)

type listAppOpts struct {
	store applicationLister
	w     io.Writer
}

// Execute writes the existing applications.
func (o *listAppOpts) Execute(_ context.Context) error {
	apps, err := o.store.ListApplications()
	if err != nil {
		return fmt.Errorf("list applications: %w", err)
	}

	for _, app := range apps {
		fmt.Fprintln(o.w, app.Name)
	}

	return nil
}

// buildAppListCommand builds the command to list existing applications.
func buildAppListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "Lists all the applications in your account.",
		Example: `
  List all the applications in your account and region.
  /code $ copilot app ls`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts := listAppOpts{
				w: os.Stdout,
			}
			defaultConfig, err := sessions.ImmutableProvider(sessions.UserAgentExtras("app ls")).DefaultConfig(context.Background())
			if err != nil {
				return fmt.Errorf("default config: %v", err)
			}
			opts.store = newSSMConfigStoreFromConfig(defaultConfig)
			return opts.Execute(cmd.Context())
		}),
	}
	return cmd
}
