// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"

	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aproint/copilot-cli/internal/pkg/term/selector"
	"github.com/spf13/cobra"
)

const (
	envListAppNamePrompt = "Which application is the environment in?"
	envListAppNameHelper = "An application is a collection of related services."
)

type listEnvVars struct {
	appName          string
	shouldOutputJSON bool
}

type listEnvOpts struct {
	listEnvVars
	store  store
	prompt prompter
	sel    configSelector

	w io.Writer
}

func newListEnvOpts(vars listEnvVars) (*listEnvOpts, error) {
	sessProvider := sessions.ImmutableProvider(sessions.UserAgentExtras("env ls"))
	defaultConfig, err := sessProvider.DefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	store := newSSMConfigStoreFromConfig(defaultConfig)
	prompter := prompt.New()
	return &listEnvOpts{
		listEnvVars: vars,
		store:       store,
		sel:         selector.NewConfigSelector(prompter, store),
		prompt:      prompter,
		w:           os.Stdout,
	}, nil
}

// Ask asks for fields that are required but not passed in.
func (o *listEnvOpts) Ask(_ context.Context) error {
	if o.appName != "" {
		return nil
	}
	app, err := o.sel.Application(envListAppNamePrompt, envListAppNameHelper)
	if err != nil {
		return fmt.Errorf("select application: %w", err)
	}
	o.appName = app
	return nil
}

// Execute lists the environments through the prompt.
func (o *listEnvOpts) Execute(_ context.Context) error {
	// Ensure the application actually exists before we try to list its environments.
	if _, err := o.store.GetApplication(o.appName); err != nil {
		return err
	}

	envs, err := o.store.ListEnvironments(o.appName)
	if err != nil {
		return err
	}

	var out string
	if o.shouldOutputJSON {
		data, err := o.jsonOutput(envs)
		if err != nil {
			return err
		}
		out = data
	} else {
		out = o.humanOutput(envs)
	}
	fmt.Fprint(o.w, out)

	return nil
}

func (o *listEnvOpts) humanOutput(envs []*config.Environment) string {
	b := &strings.Builder{}
	for _, env := range envs {
		fmt.Fprintln(b, env.Name)
	}
	return b.String()
}

func (o *listEnvOpts) jsonOutput(envs []*config.Environment) (string, error) {
	type serializedEnvs struct {
		Environments []*config.Environment `json:"environments"`
	}
	b, err := json.Marshal(serializedEnvs{Environments: envs})
	if err != nil {
		return "", fmt.Errorf("marshal environments: %w", err)
	}
	return fmt.Sprintf("%s\n", b), nil
}

// buildEnvListCmd builds the command for listing environments in an application.
func buildEnvListCmd() *cobra.Command {
	vars := listEnvVars{}
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "Lists all the environments in an application.",
		Example: `
  Lists all the environments for the frontend application.
  /code $ copilot env ls -a frontend`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newListEnvOpts(vars)
			if err != nil {
				return err
			}
			if err := opts.Ask(cmd.Context()); err != nil {
				return err
			}
			return opts.Execute(cmd.Context())
		}),
	}
	cmd.Flags().StringVarP(&vars.appName, appFlag, appFlagShort, tryReadingAppName(), appFlagDescription)
	cmd.Flags().BoolVar(&vars.shouldOutputJSON, jsonFlag, false, jsonFlagDescription)
	return cmd
}
