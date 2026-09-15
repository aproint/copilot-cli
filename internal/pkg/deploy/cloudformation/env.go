// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package cloudformation provides functionality to deploy ECS resources with AWS CloudFormation.
package cloudformation

import (
	"context"
	"fmt"

	"github.com/aproint/copilot-cli/internal/pkg/template"
	"github.com/aproint/copilot-cli/internal/pkg/version"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"gopkg.in/yaml.v3"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aproint/copilot-cli/internal/pkg/term/progress"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

// CreateAndRenderEnvironment creates the CloudFormation stack for an environment, and render the stack creation to out.
func (cf CloudFormation) CreateAndRenderEnvironment(ctx context.Context, conf StackConfiguration, bucketARN string) error {
	cfnStack, err := cf.toUploadedStack(ctx, bucketARN, conf)
	if err != nil {
		return err
	}
	in := newRenderEnvironmentInput(cfnStack)
	in.createChangeSet = func(ctx context.Context) (changeSetID string, err error) {
		spinner := progress.NewSpinner(cf.console)
		label := fmt.Sprintf("Proposing infrastructure changes for the %s environment.", cfnStack.Name)
		spinner.Start(label)
		defer stopSpinner(spinner, err, label)
		changeSetID, err = cf.cfnClient.Create(ctx, cfnStack)
		if err != nil {
			return "", err
		}
		return changeSetID, nil
	}
	return cf.executeAndRenderChangeSet(ctx, in)
}

// UpdateAndRenderEnvironment updates the CloudFormation stack for an environment, and render the stack creation to out.
func (cf CloudFormation) UpdateAndRenderEnvironment(ctx context.Context, conf StackConfiguration, bucketARN string, detach bool, opts ...cloudformation.StackOption) error {
	cfnStack, err := cf.toUploadedStack(ctx, bucketARN, conf)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		opt(cfnStack)
	}
	in := newRenderEnvironmentInput(cfnStack)
	in.createChangeSet = func(ctx context.Context) (changeSetID string, err error) {
		spinner := progress.NewSpinner(cf.console)
		label := fmt.Sprintf("Proposing infrastructure changes for the %s environment.", cfnStack.Name)
		spinner.Start(label)
		defer stopSpinner(spinner, err, label)
		changeSetID, err = cf.cfnClient.Update(ctx, cfnStack)
		if err != nil {
			return "", err
		}
		return changeSetID, nil
	}
	in.enableInterrupt = true
	in.detach = detach
	return cf.executeAndRenderChangeSet(ctx, in)
}

func newRenderEnvironmentInput(cfnStack *cloudformation.Stack) *executeAndRenderChangeSetInput {
	return &executeAndRenderChangeSetInput{
		stackName:        cfnStack.Name,
		stackDescription: fmt.Sprintf("Creating the infrastructure for the %s environment.", cfnStack.Name),
	}
}

// DeleteEnvironment deletes an environment stack using ctx.
func (cf CloudFormation) DeleteEnvironment(ctx context.Context, appName, envName, cfnExecRoleARN string) error {
	stackName := stack.NameForEnv(appName, envName)
	description := fmt.Sprintf("Delete environment stack %s", stackName)
	return cf.deleteAndRenderStack(deleteAndRenderInput{
		ctx:         ctx,
		stackName:   stackName,
		description: description,
		deleteFn: func(ctx context.Context) error {
			return cf.cfnClient.DeleteAndWaitWithRoleARN(ctx, stackName, cfnExecRoleARN)
		},
	})
}

// GetEnvironment returns the Environment metadata from the CloudFormation stack.
func (cf CloudFormation) GetEnvironment(ctx context.Context, appName, envName string) (*config.Environment, error) {
	conf := stack.NewBootstrapEnvStackConfig(&stack.EnvConfig{
		App: deploy.AppInformation{
			Name: appName,
		},
		Name: envName,
	})
	descr, err := cf.cfnClient.Describe(ctx, conf.StackName())
	if err != nil {
		return nil, err
	}
	return conf.ToEnvMetadata(descr.SDK())
}

// ForceUpdateOutputID returns the environment stack's force update ID using ctx.
func (cf CloudFormation) ForceUpdateOutputID(ctx context.Context, app, env string) (string, error) {
	stackDescr, err := cf.cachedStack(ctx, stack.NameForEnv(app, env))
	if err != nil {
		return "", err
	}
	for _, output := range stackDescr.Outputs {
		if aws.ToString(output.OutputKey) == template.LastForceDeployIDOutputName {
			return aws.ToString(output.OutputValue), nil
		}
	}
	return "", nil
}

// DeployedEnvironmentParameters returns environment parameters using ctx.
func (cf CloudFormation) DeployedEnvironmentParameters(ctx context.Context, appName, envName string) ([]types.Parameter, error) {
	isInitial, err := cf.isInitialDeployment(ctx, appName, envName)
	if err != nil {
		return nil, err
	}
	if isInitial {
		return nil, nil
	}
	out, err := cf.cachedStack(ctx, stack.NameForEnv(appName, envName))
	if err != nil {
		return nil, err
	}
	return out.Parameters, nil
}

// UpdateEnvironmentTemplate updates an environment stack template using ctx.
func (cf CloudFormation) UpdateEnvironmentTemplate(ctx context.Context, appName, envName, templateBody, cfnExecRoleARN string) error {
	stackName := stack.NameForEnv(appName, envName)
	descr, err := cf.cfnClient.Describe(ctx, stackName)
	if err != nil {
		return fmt.Errorf("describe stack %s: %w", stackName, err)
	}
	s := cloudformation.NewStack(stackName, templateBody)
	s.Parameters = descr.Parameters
	s.Tags = descr.Tags
	s.RoleARN = aws.String(cfnExecRoleARN)
	return cf.cfnClient.UpdateAndWait(ctx, s)
}

func (cf CloudFormation) toUploadedStack(ctx context.Context, artifactBucketARN string, stackConfig StackConfiguration) (*cloudformation.Stack, error) {
	bucketARN, err := arn.Parse(artifactBucketARN)
	if err != nil {
		return nil, err
	}
	url, err := cf.uploadStackTemplateToS3(ctx, bucketARN.Resource, stackConfig)
	if err != nil {
		return nil, err
	}
	cfnStack, err := toStackFromS3(stackConfig, url)
	if err != nil {
		return nil, err
	}
	return cfnStack, nil
}

func (cf CloudFormation) waitAndDescribeStack(ctx context.Context, stackName string) (*cloudformation.StackDescription, error) {
	var (
		stackDescription *cloudformation.StackDescription
		err              error
	)
	for {
		stackDescription, err = cf.cfnClient.Describe(ctx, stackName)
		if err != nil {
			return nil, fmt.Errorf("describe stack %s: %w", stackName, err)
		}

		if cloudformation.StackStatus(stackDescription.StackStatus).InProgress() {
			// There is already an update happening to the environment stack.
			// Best-effort try to wait for the existing update to be over before retrying.
			_ = cf.cfnClient.WaitForUpdate(ctx, stackName)
			continue
		}
		break
	}
	return stackDescription, err
}

func (cf CloudFormation) cachedStack(ctx context.Context, stackName string) (*cloudformation.StackDescription, error) {
	if cf.cachedDeployedStack != nil {
		return cf.cachedDeployedStack, nil
	}
	stackDescr, err := cf.waitAndDescribeStack(ctx, stackName)
	if err != nil {
		return nil, err
	}
	cf.cachedDeployedStack = stackDescr
	return cf.cachedDeployedStack, nil
}

func (cf CloudFormation) isInitialDeployment(ctx context.Context, appName, envName string) (bool, error) {
	raw, err := cf.cfnClient.Metadata(ctx, cloudformation.MetadataWithStackName(stack.NameForEnv(appName, envName)))
	if err != nil {
		return false, fmt.Errorf("get metadata of stack %q: %w", stack.NameForEnv(appName, envName), err)
	}
	metadata := struct {
		Version string `yaml:"Version"`
	}{}
	if err := yaml.Unmarshal([]byte(raw), &metadata); err != nil {
		return false, fmt.Errorf("unmarshal Metadata property to read Version: %w", err)
	}
	return metadata.Version == version.EnvTemplateBootstrap, nil
}
