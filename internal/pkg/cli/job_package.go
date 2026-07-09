// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/aproint/copilot-cli/internal/pkg/version"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/exec"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aproint/copilot-cli/internal/pkg/term/selector"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

const (
	jobPackageJobNamePrompt = "Which job would you like to generate a CloudFormation template for?"
	jobPackageEnvNamePrompt = "Which environment would you like to package this stack for?"
)

type packageJobVars struct {
	name               string
	envName            string
	appName            string
	tag                string
	outputDir          string
	uploadAssets       bool
	showDiff           bool
	allowWkldDowngrade bool
}

type packageJobOpts struct {
	packageJobVars

	// Interfaces to interact with dependencies.
	ws     wsJobDirReader
	store  store
	runner execRunner
	sel    wsSelector
	prompt prompter

	// Subcommand implementing svc_package's Execute()
	packageCmd    actionCommand
	newPackageCmd func(*packageJobOpts)
}

func newPackageJobOpts(vars packageJobVars) (*packageJobOpts, error) {
	sessProvider := sessions.ImmutableProvider(sessions.UserAgentExtras("job package"))
	defaultConfig, err := sessProvider.DefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	store := newSSMConfigStoreFromConfig(defaultConfig)
	fs := afero.NewOsFs()
	ws, err := workspace.Use(fs)
	if err != nil {
		return nil, err
	}
	prompter := prompt.New()
	opts := &packageJobOpts{
		packageJobVars: vars,
		ws:             ws,
		store:          store,
		runner:         exec.NewCmd(),
		sel:            selector.NewLocalWorkloadSelector(prompter, store, ws, selector.OnlyInitializedWorkloads),
		prompt:         prompter,
	}

	opts.newPackageCmd = func(o *packageJobOpts) {
		opts.packageCmd = &packageSvcOpts{
			packageSvcVars: packageSvcVars{
				name:               o.name,
				envName:            o.envName,
				appName:            o.appName,
				tag:                o.tag,
				outputDir:          o.outputDir,
				uploadAssets:       o.uploadAssets,
				allowWkldDowngrade: o.allowWkldDowngrade,
				showDiff:           o.showDiff,
			},
			runner:            o.runner,
			ws:                ws,
			store:             o.store,
			templateWriter:    os.Stdout,
			unmarshal:         manifest.UnmarshalWorkload,
			newInterpolator:   newManifestInterpolator,
			paramsWriter:      discardFile{},
			addonsWriter:      discardFile{},
			diffWriter:        os.Stdout,
			fs:                fs,
			sessProvider:      sessProvider,
			newStackGenerator: newWorkloadStackGenerator,
			gitShortCommit:    imageTagFromGit(o.runner),
			templateVersion:   version.LatestTemplateVersion(),
		}
	}
	return opts, nil
}

// Validate returns an error if the values provided by the user are invalid.
func (o *packageJobOpts) Validate() error {
	if o.appName == "" {
		return errNoAppInWorkspace
	}
	if o.name != "" {
		names, err := o.ws.ListJobs()
		if err != nil {
			return fmt.Errorf("list jobs in the workspace: %w", err)
		}
		if !slices.Contains(names, o.name) {
			return fmt.Errorf("job '%s' does not exist in the workspace", o.name)
		}
	}
	if o.envName != "" {
		if _, err := o.store.GetEnvironment(context.Background(), o.appName, o.envName); err != nil {
			return err
		}
	}
	return nil
}

// Ask prompts the user for any missing required fields.
func (o *packageJobOpts) Ask(ctx context.Context) error {
	if err := o.askJobName(ctx); err != nil {
		return err
	}
	if err := o.askEnvName(ctx); err != nil {
		return err
	}
	return nil
}

// Execute prints the CloudFormation template of the application for the environment.
func (o *packageJobOpts) Execute(ctx context.Context) error {
	o.newPackageCmd(o)
	return o.packageCmd.Execute(ctx)
}

// RecommendActions suggests recommended actions before the packaged template is used for deployment.
func (o *packageJobOpts) RecommendActions() error {
	return o.packageCmd.RecommendActions()
}

func (o *packageJobOpts) askJobName(ctx context.Context) error {
	if o.name != "" {
		return nil
	}

	name, err := o.sel.Job(ctx, jobPackageJobNamePrompt, "")
	if err != nil {
		return fmt.Errorf("select job: %w", err)
	}
	o.name = name
	return nil
}

func (o *packageJobOpts) askEnvName(ctx context.Context) error {
	if o.envName != "" {
		return nil
	}

	name, err := o.sel.Environment(ctx, jobPackageEnvNamePrompt, "", o.appName)
	if err != nil {
		return fmt.Errorf("select environment: %w", err)
	}
	o.envName = name
	return nil
}

// buildJobPackageCmd builds the command for printing a job's CloudFormation template.
func buildJobPackageCmd() *cobra.Command {
	vars := packageJobVars{}
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Print the AWS CloudFormation template of a job.",
		Long:  `Print the CloudFormation template used to deploy a job to an environment.`,
		Example: `
  Print the CloudFormation template for the "report-generator" job parametrized for the "test" environment.
  /code $ copilot job package -n report-generator -e test

  Write the CloudFormation stack and configuration to a "infrastructure/" sub-directory instead of printing.
  /startcodeblock
  $ copilot job package -n report-generator -e test --output-dir ./infrastructure
  $ ls ./infrastructure
  report-generator-test.stack.yml      report-generator-test.params.yml
  /endcodeblock`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newPackageJobOpts(vars)
			if err != nil {
				return err
			}
			return run(cmd.Context(), opts)
		}),
	}
	cmd.Flags().StringVarP(&vars.name, nameFlag, nameFlagShort, "", jobFlagDescription)
	cmd.Flags().StringVarP(&vars.envName, envFlag, envFlagShort, "", envFlagDescription)
	cmd.Flags().StringVarP(&vars.appName, appFlag, appFlagShort, tryReadingAppName(), appFlagDescription)
	cmd.Flags().StringVar(&vars.tag, imageTagFlag, "", imageTagFlagDescription)
	cmd.Flags().StringVar(&vars.outputDir, stackOutputDirFlag, "", stackOutputDirFlagDescription)
	cmd.Flags().BoolVar(&vars.uploadAssets, uploadAssetsFlag, false, uploadAssetsFlagDescription)
	cmd.Flags().BoolVar(&vars.showDiff, diffFlag, false, diffFlagDescription)
	cmd.Flags().BoolVar(&vars.allowWkldDowngrade, allowDowngradeFlag, false, allowDowngradeFlagDescription)

	cmd.MarkFlagsMutuallyExclusive(diffFlag, stackOutputDirFlag)
	cmd.MarkFlagsMutuallyExclusive(diffFlag, uploadAssetsFlag)
	return cmd
}
