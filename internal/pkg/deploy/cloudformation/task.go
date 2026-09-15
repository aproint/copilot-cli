// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"context"
	"errors"
	"fmt"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aws/aws-sdk-go-v2/aws"
)

// DeployTask deploys a task stack using ctx.
func (cf CloudFormation) DeployTask(ctx context.Context, input *deploy.CreateTaskResourcesInput, opts ...cloudformation.StackOption) error {
	conf := stack.NewTaskStackConfig(input)
	stack, err := toStack(conf)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		opt(stack)
	}

	if err := cf.executeAndRenderChangeSet(ctx, cf.newUpsertChangeSetInput(cf.console, stack)); err != nil {
		var errChangeSetEmpty *cloudformation.ErrChangeSetEmpty
		if !errors.As(err, &errChangeSetEmpty) {
			return err
		}
	}
	return nil
}

// ListTaskStacks returns task stacks for an environment using ctx.
func (cf CloudFormation) ListTaskStacks(ctx context.Context, appName, envName string) ([]deploy.TaskStackInfo, error) {
	taskAppEnvTags := map[string]string{
		deploy.TaskTagKey: "",
		deploy.AppTagKey:  appName,
		deploy.EnvTagKey:  envName,
	}
	tasks, err := cf.cfnClient.ListStacksWithTags(ctx, taskAppEnvTags)

	if err != nil {
		return nil, err
	}
	var outputTaskStacks []deploy.TaskStackInfo
	for _, task := range tasks {

		outputTaskStacks = append(outputTaskStacks, deploy.TaskStackInfo{
			StackName: aws.ToString(task.StackName),
			App:       appName,
			Env:       envName,

			RoleARN: aws.ToString(task.RoleARN),
		})
	}
	return outputTaskStacks, nil
}

// GetTaskStack returns task stack information using ctx.
func (cf CloudFormation) GetTaskStack(ctx context.Context, taskName string) (*deploy.TaskStackInfo, error) {
	stackName := string(stack.NameForTask(taskName))
	desc, err := cf.cfnClient.Describe(ctx, stackName)
	if err != nil {
		return nil, err
	}
	info := deploy.TaskStackInfo{
		StackName: stackName,
		RoleARN:   aws.ToString(desc.RoleARN),
	}
	var isTask bool
	for _, tag := range desc.Tags {
		switch aws.ToString(tag.Key) {
		case deploy.AppTagKey:
			info.App = aws.ToString(tag.Value)
		case deploy.EnvTagKey:
			info.Env = aws.ToString(tag.Value)
		case deploy.TaskTagKey:
			isTask = true
		}
	}
	for _, out := range desc.Outputs {
		switch aws.ToString(out.OutputKey) {
		case stack.TaskOutputS3Bucket:
			info.BucketName = aws.ToString(out.OutputValue)
		}
	}
	if !isTask {
		return nil, fmt.Errorf("%s is not a Copilot task stack", stackName)
	}
	return &info, nil
}

// ListDefaultTaskStacks returns default-cluster task stacks using ctx.
func (cf CloudFormation) ListDefaultTaskStacks(ctx context.Context) ([]deploy.TaskStackInfo, error) {
	tasks, err := cf.cfnClient.ListStacksWithTags(ctx, map[string]string{deploy.TaskTagKey: ""})
	if err != nil {
		return nil, err
	}
	var outputTaskStacks []deploy.TaskStackInfo
	for _, task := range tasks {
		// Eliminate tasks which are tagged for a particular copilot app or env.
		var hasAppTag, hasEnvTag bool
		for _, tag := range task.Tags {
			if aws.ToString(tag.Key) == deploy.AppTagKey {
				hasAppTag = true
			}
			if aws.ToString(tag.Key) == deploy.EnvTagKey {
				hasEnvTag = true
			}
		}
		if hasAppTag || hasEnvTag {
			continue
		}
		outputTaskStacks = append(outputTaskStacks, deploy.TaskStackInfo{
			StackName: aws.ToString(task.StackName),
		})
	}
	return outputTaskStacks, nil
}

// DeleteTask deletes a task stack using ctx.
func (cf CloudFormation) DeleteTask(ctx context.Context, task deploy.TaskStackInfo) error {
	if task.RoleARN != "" {
		return cf.cfnClient.DeleteAndWaitWithRoleARN(ctx, task.StackName, task.RoleARN)
	}
	return cf.cfnClient.DeleteAndWait(ctx, task.StackName)
}
