// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aproint/copilot-cli/internal/pkg/aws/s3"
	"github.com/spf13/afero"

	awscfn "github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/aws/ecr"
	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/ecs"
	"github.com/aproint/copilot-cli/internal/pkg/term/color"
	"github.com/aproint/copilot-cli/internal/pkg/term/log"
	termprogress "github.com/aproint/copilot-cli/internal/pkg/term/progress"
	"github.com/aproint/copilot-cli/internal/pkg/term/prompt"
	"github.com/aproint/copilot-cli/internal/pkg/term/selector"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/spf13/cobra"
)

const (
	taskDeleteNamePrompt              = "Which task would you like to delete?"
	taskDeleteAppPrompt               = "Which application would you like to delete a task from?"
	taskDeleteEnvPrompt               = "Which environment would you like to delete a task from?"
	fmtTaskDeleteDefaultConfirmPrompt = "Are you sure you want to delete %s from the default cluster?"
	fmtTaskDeleteFromEnvConfirmPrompt = "Are you sure you want to delete %s from application %s and environment %s?"
	taskDeleteConfirmHelp             = "This will delete the task's stack and stop all current executions."
)

var errTaskDeleteCancelled = errors.New("task delete cancelled - no changes made")

type deleteTaskVars struct {
	name             string
	app              string
	env              string
	skipConfirmation bool
	defaultCluster   bool
}

type deleteTaskOpts struct {
	deleteTaskVars
	wsAppName string

	// Dependencies to interact with other modules
	store    store
	prompt   prompter
	spinner  progress
	provider sessionProvider
	sel      wsSelector

	// Generators for env-specific clients
	newTaskSel       func(aws.Config) cfTaskSelector
	newTaskStopper   func(aws.Config) taskStopper
	newImageRemover  func(aws.Config) imageRemover
	newBucketEmptier func(aws.Config) bucketEmptier
	newStackManager  func(aws.Config) taskStackManager

	// Cached variables
	cfg       aws.Config
	hasConfig bool
	stackInfo *deploy.TaskStackInfo
}

func newDeleteTaskOpts(vars deleteTaskVars) (*deleteTaskOpts, error) {
	ws, err := workspace.Use(afero.NewOsFs())
	if err != nil {
		return nil, err
	}

	sessProvider := sessions.ImmutableProvider(sessions.UserAgentExtras("task delete"))
	defaultConfig, err := sessProvider.DefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("default config: %v", err)
	}

	store := newSSMConfigStoreFromConfig(defaultConfig)
	prompter := prompt.New()
	return &deleteTaskOpts{
		deleteTaskVars: vars,
		wsAppName:      tryReadingAppName(),

		store:    store,
		spinner:  termprogress.NewSpinner(log.DiagnosticWriter),
		prompt:   prompter,
		provider: sessProvider,
		sel:      selector.NewLocalWorkloadSelector(prompter, store, ws, selector.OnlyInitializedWorkloads),
		newTaskSel: func(cfg aws.Config) cfTaskSelector {
			cfn := cloudformation.New(cfg, cloudformation.WithProgressTracker(os.Stderr))
			return selector.NewCFTaskSelect(prompter, store, cfn)
		},
		newTaskStopper: func(cfg aws.Config) taskStopper {
			return ecs.New(cfg)
		},
		newStackManager: func(cfg aws.Config) taskStackManager {
			return cloudformation.New(cfg, cloudformation.WithProgressTracker(os.Stderr))
		},
		newImageRemover: func(cfg aws.Config) imageRemover {
			return ecr.New(cfg)
		},
		newBucketEmptier: func(cfg aws.Config) bucketEmptier {
			return s3.New(cfg)
		},
	}, nil
}

// Validate checks that flag inputs are valid.
func (o *deleteTaskOpts) Validate() error {

	if o.name != "" {
		if err := basicNameValidation(o.name); err != nil {
			return err
		}
	}

	// If default flag specified,
	if err := o.validateFlagsWithDefaultCluster(); err != nil {
		return err
	}

	if err := o.validateFlagsWithEnv(); err != nil {
		return err
	}

	return nil
}

func (o *deleteTaskOpts) validateFlagsWithEnv() error {
	if o.app != "" {
		if _, err := o.store.GetApplication(o.app); err != nil {
			return fmt.Errorf("get application: %w", err)
		}
	}

	if o.app != "" && o.env != "" {
		if _, err := o.store.GetEnvironment(o.app, o.env); err != nil {
			return fmt.Errorf("get environment: %w", err)
		}

		if err := o.validateTaskName(); err != nil {
			return fmt.Errorf("get task: %w", err)
		}
	}

	return nil
}

func (o *deleteTaskOpts) validateTaskName() error {
	if o.name != "" {
		// If fully specified, validate that the stack exists and is a task.
		// This check prevents the command from stopping arbitrary tasks or emptying arbitrary ECR
		// repositories.
		_, err := o.getTaskInfo()
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *deleteTaskOpts) validateFlagsWithDefaultCluster() error {
	if !o.defaultCluster {
		return nil
	}

	// If app is specified and the same as the workspace app, don't throw an error.
	// If app is not the same as the workspace app, either
	//   a) there is no WS app and it's specified erroneously, in which case we should error
	//   b) there is a WS app and the flag has been set to a different app, in which case we should error.
	// The app flag defaults to the WS app so there's an edge case where it's possible to specify
	// `copilot task delete --app ws-app --default` and not error out, but this should be taken as
	// specifying "default".
	if o.app != o.wsAppName {
		return fmt.Errorf("cannot specify both `--app` and `--default`")
	}

	if o.env != "" {
		return fmt.Errorf("cannot specify both `--env` and `--default`")
	}

	if err := o.validateTaskName(); err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	return nil
}

func (o *deleteTaskOpts) askAppName() error {
	if o.defaultCluster {
		return nil
	}

	if o.app != "" {
		return nil
	}

	app, err := o.sel.Application(taskDeleteAppPrompt, "", appEnvOptionNone)
	if err != nil {
		return fmt.Errorf("select application name: %w", err)
	}
	if app == appEnvOptionNone {
		o.env = ""
		o.defaultCluster = true
		return nil
	}
	o.app = app
	return nil
}

func (o *deleteTaskOpts) askEnvName() error {
	if o.defaultCluster {
		return nil
	}

	if o.env != "" {
		return nil
	}
	env, err := o.sel.Environment(taskDeleteEnvPrompt, "", o.app, prompt.Option{Value: appEnvOptionNone})
	if err != nil {
		return fmt.Errorf("select environment: %w", err)
	}
	if env == appEnvOptionNone {
		o.env = ""
		o.app = ""
		o.defaultCluster = true
		return nil
	}
	o.env = env
	return nil
}

// Ask prompts for missing information and fills in gaps.
func (o *deleteTaskOpts) Ask() error {
	if err := o.askAppName(); err != nil {
		return err
	}

	if err := o.askEnvName(); err != nil {
		return err
	}

	if err := o.askTaskName(); err != nil {
		return err
	}

	if o.skipConfirmation {
		return nil
	}

	// Confirm deletion
	deletePrompt := fmt.Sprintf(fmtTaskDeleteDefaultConfirmPrompt, color.HighlightUserInput(o.name))
	if o.env != "" && o.app != "" {
		deletePrompt = fmt.Sprintf(
			fmtTaskDeleteFromEnvConfirmPrompt,
			color.HighlightUserInput(o.name),
			color.HighlightUserInput(o.app),
			color.HighlightUserInput(o.env),
		)
	}

	deleteConfirmed, err := o.prompt.Confirm(
		deletePrompt,
		taskDeleteConfirmHelp,
		prompt.WithConfirmFinalMessage())

	if err != nil {
		return fmt.Errorf("task delete confirmation prompt: %w", err)
	}
	if !deleteConfirmed {
		return errTaskDeleteCancelled
	}
	return nil
}

func (o *deleteTaskOpts) getConfig() (aws.Config, error) {
	if o.hasConfig {
		return o.cfg, nil
	}
	if o.defaultCluster {
		cfg, err := o.provider.DefaultConfig(context.Background())
		if err != nil {
			return aws.Config{}, err
		}
		o.cfg = cfg
		o.hasConfig = true
		return cfg, nil
	}
	// Get environment manager role for deleting stack.
	env, err := o.store.GetEnvironment(o.app, o.env)
	if err != nil {
		return aws.Config{}, err
	}
	cfg, err := o.provider.ConfigFromRole(context.Background(), env.ManagerRoleARN, env.Region)
	if err != nil {
		return aws.Config{}, err
	}
	o.cfg = cfg
	o.hasConfig = true
	return cfg, nil
}

func (o *deleteTaskOpts) askTaskName() error {
	if o.name != "" {
		return nil
	}

	cfg, err := o.getConfig()
	if err != nil {
		return fmt.Errorf("get task select session: %w", err)
	}
	sel := o.newTaskSel(cfg)
	if o.defaultCluster {
		task, err := sel.Task(taskDeleteNamePrompt, "", selector.TaskWithDefaultCluster())
		if err != nil {
			return fmt.Errorf("select task from default cluster: %w", err)
		}
		o.name = task
		return nil
	}
	task, err := sel.Task(taskDeleteNamePrompt, "", selector.TaskWithAppEnv(o.app, o.env))
	if err != nil {
		return fmt.Errorf("select task from environment: %w", err)
	}
	o.name = task
	return nil
}

func (o *deleteTaskOpts) Execute() error {
	if err := o.stopTasks(); err != nil {
		return err
	}
	if err := o.clearECRRepository(); err != nil {
		return err
	}
	if err := o.deleteStack(); err != nil {
		return err
	}
	return nil
}

func (o *deleteTaskOpts) stopTasks() error {
	cfg, err := o.getConfig()
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	o.spinner.Start(fmt.Sprintf("Stopping all running tasks in family %s.", color.HighlightUserInput(o.name)))

	// Stop tasks.
	if o.defaultCluster {
		if err = o.newTaskStopper(cfg).StopDefaultClusterTasks(o.name); err != nil {
			o.spinner.Stop(log.Serrorln("Error stopping running tasks in default cluster."))
			return fmt.Errorf("stop running tasks in family %s: %w", o.name, err)
		}
	} else {
		if err = o.newTaskStopper(cfg).StopOneOffTasks(o.app, o.env, o.name); err != nil {
			o.spinner.Stop(log.Serrorln("Error stopping running tasks in environment."))
			return fmt.Errorf("stop running tasks in family %s: %w", o.name, err)
		}
	}
	o.spinner.Stop(log.Ssuccessf("Stopped all running tasks in family %s.\n", color.HighlightUserInput(o.name)))
	return nil
}

func (o *deleteTaskOpts) clearECRRepository() error {
	// ECR Deletion happens from the default profile in app delete. We can do it here too by getting
	// a default session in whichever region we're deleting from.
	defaultConfig, err := o.getConfig()
	if err != nil {
		return err
	}
	if !o.defaultCluster {
		defaultConfig, err = o.provider.DefaultConfigWithRegion(context.Background(), defaultConfig.Region)
		if err != nil {
			return fmt.Errorf("get default config for ECR deletion: %s", err)
		}
	}
	// Best effort to construct ECR repo name.
	ecrRepoName := fmt.Sprintf(deploy.FmtTaskECRRepoName, o.name)

	o.spinner.Start(fmt.Sprintf("Emptying ECR repository for task %s.", color.HighlightUserInput(o.name)))
	err = o.newImageRemover(defaultConfig).ClearRepository(ecrRepoName)
	if err != nil {
		o.spinner.Stop(log.Serrorln("Error emptying ECR repository."))
		return fmt.Errorf("empty ECR repository for task %s: %w", o.name, err)
	}

	o.spinner.Stop(log.Ssuccessf("Emptied ECR repository for task %s.\n", color.HighlightUserInput(o.name)))
	return nil
}

func (o *deleteTaskOpts) emptyS3Bucket(info *deploy.TaskStackInfo) error {
	o.spinner.Start(fmt.Sprintf("Emptying S3 bucket for task %s.", color.HighlightUserInput(o.name)))
	cfg, err := o.getConfig()
	if err != nil {
		return err
	}
	err = o.newBucketEmptier(cfg).EmptyBucket(info.BucketName)
	if err != nil {
		o.spinner.Stop(log.Serrorln("Error emptying S3 bucket."))
		return fmt.Errorf("empty S3 bucket for task %s: %w", o.name, err)
	}

	o.spinner.Stop(log.Ssuccessf("Emptied S3 bucket for task %s.\n", color.HighlightUserInput(o.name)))
	return nil
}

// getTaskInfo returns a struct of information about the task, including the app and env it's deployed to, if
// applicable, and the ARN of any CF role it's associated with.
func (o *deleteTaskOpts) getTaskInfo() (*deploy.TaskStackInfo, error) {
	if o.stackInfo != nil {
		return o.stackInfo, nil
	}
	cfg, err := o.getConfig()
	if err != nil {
		return nil, err
	}
	info, err := o.newStackManager(cfg).GetTaskStack(o.name)

	if err != nil {
		return nil, err
	}
	o.stackInfo = info
	return info, nil
}

func (o *deleteTaskOpts) deleteStack() error {
	cfg, err := o.getConfig()
	if err != nil {
		return err
	}
	info, err := o.getTaskInfo()
	if err != nil {
		// If the stack doesn't exist, don't error.
		var errStackNotExist *awscfn.ErrStackNotFound
		if errors.As(err, &errStackNotExist) {
			return nil
		}
		return err
	}
	if info == nil {
		// Stack does not exist; skip deleting it.
		return nil
	}
	if info.BucketName != "" {
		if err := o.emptyS3Bucket(info); err != nil {
			return err
		}
	}
	o.spinner.Start(fmt.Sprintf("Deleting CloudFormation stack for task %s.", color.HighlightUserInput(o.name)))
	err = o.newStackManager(cfg).DeleteTask(*info)
	if err != nil {
		o.spinner.Stop(log.Serrorln("Error deleting CloudFormation stack."))
		return fmt.Errorf("delete stack for task %s: %w", o.name, err)
	}

	o.spinner.Stop(log.Ssuccessf("Deleted resources of task %s.\n", color.HighlightUserInput(o.name)))
	return nil
}

func (o *deleteTaskOpts) RecommendActions() error {
	return nil
}

// BuildTaskDeleteCmd builds the command to delete application(s).
func BuildTaskDeleteCmd() *cobra.Command {
	vars := deleteTaskVars{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes a one-off task from an application or default cluster.",
		Example: `
  Delete the "test" task from the default cluster.
  /code $ copilot task delete --name test --default

  Delete the "db-migrate" task from the prod environment.
  /code $ copilot task delete --name db-migrate --env prod

  Delete the "test" task without confirmation prompt.
  /code $ copilot task delete --name test --yes`,
		RunE: runCmdE(func(cmd *cobra.Command, args []string) error {
			opts, err := newDeleteTaskOpts(vars)
			if err != nil {
				return err
			}
			return run(opts)
		}),
	}

	cmd.Flags().StringVarP(&vars.app, appFlag, appFlagShort, tryReadingAppName(), appFlagDescription)
	cmd.Flags().StringVarP(&vars.name, nameFlag, nameFlagShort, "", svcFlagDescription)
	cmd.Flags().StringVarP(&vars.env, envFlag, envFlagShort, "", envFlagDescription)
	cmd.Flags().BoolVar(&vars.skipConfirmation, yesFlag, false, yesFlagDescription)
	cmd.Flags().BoolVar(&vars.defaultCluster, taskDefaultFlag, false, taskDeleteDefaultFlagDescription)
	return cmd
}
