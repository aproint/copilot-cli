// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stack

import (
	"context"
	"fmt"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aws/aws-sdk-go-v2/aws"
)

type cfn interface {
	Describe(ctx context.Context, name string) (*cloudformation.StackDescription, error)
	StackResources(ctx context.Context, name string) ([]*cloudformation.StackResource, error)
	Metadata(ctx context.Context, opt cloudformation.MetadataOpts) (string, error)
}

// StackDescription is the description of a cloudformation stack.
type StackDescription struct {
	Parameters map[string]string
	Tags       map[string]string
	Outputs    map[string]string
}

// Resource contains cloudformation stack resource info.
type Resource struct {
	Type       string `json:"type"`
	PhysicalID string `json:"physicalID"`
	LogicalID  string `json:"logicalID,omitempty"`
}

// HumanString returns the stringified Resource struct with human readable format.
func (c Resource) HumanString() string {
	return fmt.Sprintf("%s\t%s\n", c.Type, c.PhysicalID)
}

// StackDescriber retrieves information about a stack.
type StackDescriber struct {
	name string
	cfn  cfn
}

// NewStackDescriber instantiates a new StackDescriber.
func NewStackDescriber(stackName string, cfg aws.Config) *StackDescriber {
	return &StackDescriber{
		name: stackName,
		cfn:  cloudformation.New(cfg),
	}
}

// Describe retrieves information about a CloudFormation stack using ctx.
func (d *StackDescriber) Describe(ctx context.Context) (StackDescription, error) {
	descr, err := d.cfn.Describe(ctx, d.name)
	return d.stackDescription(descr, err)
}

func (d *StackDescriber) stackDescription(descr *cloudformation.StackDescription, err error) (StackDescription, error) {
	if err != nil {
		return StackDescription{}, fmt.Errorf("describe stack %s: %w", d.name, err)
	}
	params := make(map[string]string)
	for _, param := range descr.Parameters {
		params[aws.ToString(param.ParameterKey)] = aws.ToString(param.ParameterValue)
	}
	outputs := make(map[string]string)
	for _, out := range descr.Outputs {
		outputs[aws.ToString(out.OutputKey)] = aws.ToString(out.OutputValue)
	}
	tags := make(map[string]string)
	for _, tag := range descr.Tags {
		tags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return StackDescription{
		Parameters: params,
		Tags:       tags,
		Outputs:    outputs,
	}, nil
}

// Resources retrieves stack resources using ctx.
func (d *StackDescriber) Resources(ctx context.Context) ([]*Resource, error) {
	resources, err := d.cfn.StackResources(ctx, d.name)
	return d.resources(resources, err)
}

func (d *StackDescriber) resources(resources []*cloudformation.StackResource, err error) ([]*Resource, error) {
	if err != nil {
		return nil, fmt.Errorf("retrieve resources for stack %s: %w", d.name, err)
	}
	return flattenResources(resources), nil
}

// StackMetadata returns stack metadata using ctx.
func (d *StackDescriber) StackMetadata(ctx context.Context) (string, error) {
	metadata, err := d.cfn.Metadata(ctx, cloudformation.MetadataWithStackName(d.name))
	return d.stackMetadata(metadata, err)
}

func (d *StackDescriber) stackMetadata(metadata string, err error) (string, error) {
	if err != nil {
		return "", fmt.Errorf("get metadata for stack %s: %w", d.name, err)
	}
	return metadata, nil
}

// StackSetMetadata returns stack set metadata using ctx.
func (d *StackDescriber) StackSetMetadata(ctx context.Context) (string, error) {
	metadata, err := d.cfn.Metadata(ctx, cloudformation.MetadataWithStackSetName(d.name))
	return d.stackSetMetadata(metadata, err)
}

func (d *StackDescriber) stackSetMetadata(metadata string, err error) (string, error) {
	if err != nil {
		return "", fmt.Errorf("get metadata for stack set %s: %w", d.name, err)
	}
	return metadata, nil
}

func flattenResources(stackResources []*cloudformation.StackResource) []*Resource {
	var resources []*Resource
	for _, stackResource := range stackResources {
		resources = append(resources, &Resource{
			Type:       aws.ToString(stackResource.ResourceType),
			PhysicalID: aws.ToString(stackResource.PhysicalResourceId),
			LogicalID:  aws.ToString(stackResource.LogicalResourceId),
		})
	}
	return resources
}
