// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package cloudformation provides functionality to deploy ECS resources with AWS CloudFormation.
// This file defines API for deploying a pipeline.
package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
)

const (
	sourceStage                  = "Source"
	connectionARNKey             = "PipelineConnectionARN"
	fmtPipelineCfnTemplateName   = "%s.pipeline.stack.yml"
	cfnLogicalResourceIDPipeline = "Pipeline"
	cfnResourceTypePipeline      = "AWS::CodePipeline::Pipeline"
)

// PipelineExists checks if the pipeline with the provided config exists.
func (cf CloudFormation) PipelineExists(stackConfig StackConfiguration) (bool, error) {
	return cf.PipelineExistsWithContext(context.Background(), stackConfig)
}

// PipelineExistsWithContext checks if the pipeline exists using ctx.
func (cf CloudFormation) PipelineExistsWithContext(ctx context.Context, stackConfig StackConfiguration) (bool, error) {
	_, err := cf.cfnClient.DescribeWithContext(ctx, stackConfig.StackName())
	if err != nil {
		var stackNotFound *cloudformation.ErrStackNotFound
		if !errors.As(err, &stackNotFound) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// CreatePipeline sets up a new CodePipeline for deploying services.
func (cf CloudFormation) CreatePipeline(bucketName string, stackConfig StackConfiguration) error {
	return cf.CreatePipelineWithContext(context.Background(), bucketName, stackConfig)
}

// CreatePipelineWithContext sets up a new CodePipeline using ctx.
func (cf CloudFormation) CreatePipelineWithContext(ctx context.Context, bucketName string, stackConfig StackConfiguration) error {
	templateURL, err := cf.pushTemplateToS3BucketWithContext(ctx, bucketName, stackConfig)
	if err != nil {
		return err
	}
	s, err := toStackFromS3(stackConfig, templateURL)
	if err != nil {
		return err
	}
	err = cf.cfnClient.CreateAndWaitWithContext(ctx, s)
	if err != nil {
		return err
	}

	output, err := cf.cfnClient.OutputsWithContext(ctx, s)
	if err != nil {
		return err
	}
	// If the pipeline has a PipelineConnectionARN in the output map, indicating that it is has a CodeStarConnections source provider, the user needs to update the connection status; Copilot will wait until that happens.
	if output[connectionARNKey] == "" {
		return nil
	}
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(45*time.Minute))
	defer cancel()
	if err = cf.codeStarClient.WaitUntilConnectionStatusAvailable(ctx, output[connectionARNKey]); err != nil {
		return err
	}

	pipelineResourceName, err := cf.pipelinePhysicalResourceIDWithContext(ctx, stackConfig.StackName())
	if err != nil {
		return err
	}
	if err = cf.cpClient.RetryStageExecutionWithContext(ctx, pipelineResourceName, sourceStage); err != nil {
		return err
	}
	return nil
}

func (cf CloudFormation) pipelinePhysicalResourceID(stackName string) (string, error) {
	return cf.pipelinePhysicalResourceIDWithContext(context.Background(), stackName)
}

func (cf CloudFormation) pipelinePhysicalResourceIDWithContext(ctx context.Context, stackName string) (string, error) {
	stackResources, err := cf.cfnClient.StackResourcesWithContext(ctx, stackName)
	if err != nil {
		return "", err
	}
	for _, resource := range stackResources {
		if aws.ToString(resource.LogicalResourceId) == cfnLogicalResourceIDPipeline && aws.ToString(resource.ResourceType) == cfnResourceTypePipeline {
			return aws.ToString(resource.PhysicalResourceId), nil
		}
	}
	return "", fmt.Errorf(`cannot find a resource in stack %s with logical ID "%s" of type "%s"`, stackName, cfnLogicalResourceIDPipeline, cfnResourceTypePipeline)
}

// UpdatePipeline updates an existing CodePipeline for deploying services.
func (cf CloudFormation) UpdatePipeline(bucketName string, stackConfig StackConfiguration) error {
	return cf.UpdatePipelineWithContext(context.Background(), bucketName, stackConfig)
}

// UpdatePipelineWithContext updates an existing CodePipeline using ctx.
func (cf CloudFormation) UpdatePipelineWithContext(ctx context.Context, bucketName string, stackConfig StackConfiguration) error {
	templateURL, err := cf.pushTemplateToS3BucketWithContext(ctx, bucketName, stackConfig)
	if err != nil {
		return err
	}
	s, err := toStackFromS3(stackConfig, templateURL)
	if err != nil {
		return err
	}
	if err := cf.cfnClient.UpdateAndWaitWithContext(ctx, s); err != nil {
		var errNoUpdates *cloudformation.ErrChangeSetEmpty
		if errors.As(err, &errNoUpdates) {
			return nil
		}
		return fmt.Errorf("update pipeline: %w", err)
	}
	return nil
}

// DeletePipeline removes the CodePipeline stack.
func (cf CloudFormation) DeletePipeline(pipeline deploy.Pipeline) error {
	return cf.DeletePipelineWithContext(context.Background(), pipeline)
}

// DeletePipelineWithContext removes the CodePipeline stack using ctx.
func (cf CloudFormation) DeletePipelineWithContext(ctx context.Context, pipeline deploy.Pipeline) error {
	return cf.cfnClient.DeleteAndWaitWithContext(ctx, stack.NameForPipeline(pipeline.AppName, pipeline.Name, pipeline.IsLegacy))
}

func (cf CloudFormation) pushTemplateToS3Bucket(bucket string, config StackConfiguration) (string, error) {
	return cf.pushTemplateToS3BucketWithContext(context.Background(), bucket, config)
}

func (cf CloudFormation) pushTemplateToS3BucketWithContext(ctx context.Context, bucket string, config StackConfiguration) (string, error) {
	template, err := config.Template()
	if err != nil {
		return "", fmt.Errorf("generate template: %w", err)
	}
	reader := strings.NewReader(template)
	url, err := cf.s3Client.UploadWithContext(ctx, bucket, fmt.Sprintf(fmtPipelineCfnTemplateName, config.StackName()), reader)
	if err != nil {
		return "", fmt.Errorf("upload pipeline template to S3 bucket %s: %w", bucket, err)
	}
	return url, nil
}
