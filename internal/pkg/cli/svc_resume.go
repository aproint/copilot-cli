// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"

	"github.com/aproint/copilot-cli/internal/pkg/manifest/manifestinfo"

	"github.com/aproint/copilot-cli/internal/pkg/aws/apprunner"
	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/describe"
	"github.com/aproint/copilot-cli/internal/pkg/term/color"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	termprogress "github.com/aproint/copilot-cli/internal/pkg/term/progress"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aproint/copilot-cli/internal/pkg/term/selector"
	"github.com/spf13/cobra"
)

const (
	svcResumeSvcNamePrompt     = "Which service of %s would you like to resume?"
	svcResumeSvcNameHelpPrompt = "The selected service will be resumed."

	fmtSvcResumeStarted = "Resuming service %s in environment %s."
	fmtSvcResumeFailed  = "Failed to resume service %s in environment %s: %v\n"
	fmtSvcResumeSuccess = "Resumed service %s in environment %s.\n"
)

type resumeSvcVars struct {
	appName string
	svcName string
	envName string
}

type resumeSvcInitClients func(ctx context.Context) error
type resumeSvcOpts struct {
	resumeSvcVars

	store              store
	serviceResumer     serviceResumer
	apprunnerDescriber apprunnerServiceDescriber
	spinner            progress
	sel                deploySelector
	initClients        resumeSvcInitClients
}

// Validate returns an error for any invalid optional flags.
func (o *resumeSvcOpts) Validate() error {
	return nil
}

// Ask prompts for and validates any required flags.
func (o *resumeSvcOpts) Ask(ctx context.Context) error {
	if err := o.validateOrAskApp(ctx); err != nil {
		return err
	}
	return o.validateAndAskSvcEnvName(ctx)
}

// Execute resumes the service through the prompt.
func (o *resumeSvcOpts) Execute(ctx context.Context) error {
	if o.svcName == "" {
		return nil
	}
	if err := o.initClients(ctx); err != nil {
		return err
	}
	svcARN, err := o.apprunnerDescriber.ServiceARN(o.envName)
	if err != nil {
		return err
	}

	o.spinner.Start(fmt.Sprintf(fmtSvcResumeStarted, o.svcName, o.envName))
	if err := o.serviceResumer.ResumeService(svcARN); err != nil {
		o.spinner.Stop(log.Serrorf(fmtSvcResumeFailed, o.svcName, o.envName, err))
		return err
	}
	o.spinner.Stop(log.Ssuccessf(fmtSvcResumeSuccess, o.svcName, o.envName))
	return nil
}

func (o *resumeSvcOpts) validateOrAskApp(ctx context.Context) error {
	if o.appName != "" {
		_, err := o.store.GetApplication(ctx, o.appName)
		return err
	}
	appName, err := o.sel.Application(ctx, svcAppNamePrompt, wkldAppNameHelpPrompt)
	if err != nil {
		return fmt.Errorf("select application: %w", err)
	}
	o.appName = appName

	return nil
}

func (o *resumeSvcOpts) validateAndAskSvcEnvName(ctx context.Context) error {
	if o.envName != "" {
		if _, err := o.store.GetEnvironment(ctx, o.appName, o.envName); err != nil {
			return err
		}
	}

	if o.svcName != "" {
		if _, err := o.store.GetService(ctx, o.appName, o.svcName); err != nil {
			return err
		}
	}
	// Note: we let prompter handle the case when there is only option for user to choose from.
	// This is naturally the case when `o.envName != "" && o.svcName != ""`.
	deployedService, err := o.sel.DeployedService(ctx,
		fmt.Sprintf(svcResumeSvcNamePrompt, color.HighlightUserInput(o.appName)),
		svcResumeSvcNameHelpPrompt,
		o.appName,
		selector.WithEnv(o.envName),
		selector.WithName(o.svcName),
		selector.WithServiceTypesFilter([]string{manifestinfo.RequestDrivenWebServiceType}),
	)
	if err != nil {
		return fmt.Errorf("select deployed service for application %s: %w", o.appName, err)
	}
	o.svcName = deployedService.Name
	o.envName = deployedService.Env
	return nil
}

func newResumeSvcOpts(vars resumeSvcVars) (*resumeSvcOpts, error) {
	sessProvider := sessions.ImmutableProvider(sessions.UserAgentExtras("svc resume"))
	defaultConfig, err := sessProvider.DefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("default config: %v", err)
	}

	configStore := newSSMConfigStoreFromConfig(defaultConfig)
	deployStore, err := deploy.NewStore(sessProvider, configStore)
	if err != nil {
		return nil, fmt.Errorf("connect to deploy store: %w", err)
	}

	opts := &resumeSvcOpts{
		resumeSvcVars: vars,
		store:         configStore,
		sel:           selector.NewDeploySelect(prompt.New(), configStore, deployStore),
		spinner:       termprogress.NewSpinner(log.DiagnosticWriter),
	}
	opts.initClients = func(ctx context.Context) error {
		var a *apprunner.AppRunner
		var d *describe.RDWebServiceDescriber
		env, err := configStore.GetEnvironment(ctx, opts.appName, opts.envName)
		if err != nil {
			return fmt.Errorf("get environment: %w", err)
		}
		svc, err := opts.store.GetService(ctx, opts.appName, opts.svcName)
		if err != nil {
			return err
		}
		switch svc.Type {
		case manifestinfo.RequestDrivenWebServiceType:
			cfg, err := sessProvider.ConfigFromRole(context.Background(), env.ManagerRoleARN, env.Region)
			if err != nil {
				return err
			}
			a = apprunner.New(cfg)
			d, err = describe.NewRDWebServiceDescriber(ctx, describe.NewServiceConfig{
				App:         opts.appName,
				Svc:         opts.svcName,
				ConfigStore: configStore,
			})
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid service type %s", svc.Type)
		}

		if err != nil {
			return fmt.Errorf("creating describer for service %s in environment %s and application %s: %w", opts.svcName, opts.envName, opts.appName, err)
		}
		opts.serviceResumer = a
		opts.apprunnerDescriber = d
		return nil
	}
	return opts, nil
}

// buildSvcResumeCmd builds the command for resuming services in an application.
func buildSvcResumeCmd() *cobra.Command {
	vars := resumeSvcVars{}
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Resumes a paused service.",
		Long:  "Resumes a paused service.",
		Example: `
  Resumes the service named "my-svc" in the "test" environment.
  /code $ copilot svc resume --name my-svc --env test`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newResumeSvcOpts(vars)
			if err != nil {
				return err
			}
			return run(cmd.Context(), opts)
		}),
	}
	cmd.Flags().StringVarP(&vars.appName, appFlag, appFlagShort, tryReadingAppName(), appFlagDescription)
	cmd.Flags().StringVarP(&vars.svcName, nameFlag, nameFlagShort, "", svcFlagDescription)
	cmd.Flags().StringVarP(&vars.envName, envFlag, envFlagShort, "", envFlagDescription)
	return cmd
}
