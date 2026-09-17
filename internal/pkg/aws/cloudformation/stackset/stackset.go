// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package stackset provides a client to make API requests to an AWS CloudFormation StackSet resource.
package stackset

import (
	"context"
	"errors"
	"fmt"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

type api interface {
	CreateStackSet(context.Context, *cloudformation.CreateStackSetInput, ...func(*cloudformation.Options)) (*cloudformation.CreateStackSetOutput, error)
	UpdateStackSet(context.Context, *cloudformation.UpdateStackSetInput, ...func(*cloudformation.Options)) (*cloudformation.UpdateStackSetOutput, error)
	ListStackSetOperations(context.Context, *cloudformation.ListStackSetOperationsInput, ...func(*cloudformation.Options)) (*cloudformation.ListStackSetOperationsOutput, error)
	DeleteStackSet(context.Context, *cloudformation.DeleteStackSetInput, ...func(*cloudformation.Options)) (*cloudformation.DeleteStackSetOutput, error)
	DescribeStackSet(context.Context, *cloudformation.DescribeStackSetInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStackSetOutput, error)
	DescribeStackSetOperation(context.Context, *cloudformation.DescribeStackSetOperationInput, ...func(*cloudformation.Options)) (*cloudformation.DescribeStackSetOperationOutput, error)

	CreateStackInstances(context.Context, *cloudformation.CreateStackInstancesInput, ...func(*cloudformation.Options)) (*cloudformation.CreateStackInstancesOutput, error)
	DeleteStackInstances(context.Context, *cloudformation.DeleteStackInstancesInput, ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error)
	ListStackInstances(context.Context, *cloudformation.ListStackInstancesInput, ...func(*cloudformation.Options)) (*cloudformation.ListStackInstancesOutput, error)
}

// StackSet represents an AWS CloudFormation client to interact with stack sets.
type StackSet struct {
	client api
}

// New creates a new client to make requests against stack sets.
func New(cfg awsv2.Config) *StackSet {
	return &StackSet{
		client: cloudformation.NewFromConfig(cfg),
	}
}

// CreateOrUpdateOption allows to initialize or update a stack set with additional properties.
type CreateOrUpdateOption func(interface{})

// Create creates a new stack set resource using ctx.
func (ss *StackSet) Create(ctx context.Context, name, template string, opts ...CreateOrUpdateOption) error {
	in := &cloudformation.CreateStackSetInput{
		StackSetName: awsv2.String(name),
		TemplateBody: awsv2.String(template),
	}
	for _, opt := range opts {
		opt(in)
	}
	_, err := ss.client.CreateStackSet(ctx, in)
	if err != nil {
		if !isAlreadyExistingStackSet(err) {
			return fmt.Errorf("create stack set %s: %w", name, err)
		}
	}
	return nil
}

// Describe returns a description of a stack set using ctx.
func (ss *StackSet) Describe(ctx context.Context, name string) (Description, error) {
	resp, err := ss.client.DescribeStackSet(ctx, &cloudformation.DescribeStackSetInput{
		StackSetName: awsv2.String(name),
	})
	if err != nil {
		return Description{}, fmt.Errorf("describe stack set %s: %w", name, err)
	}
	return Description{
		ID:       awsv2.ToString(resp.StackSet.StackSetId),
		Name:     awsv2.ToString(resp.StackSet.StackSetName),
		Template: awsv2.ToString(resp.StackSet.TemplateBody),
	}, nil
}

// Operation represents information about a stack set operation.
type Operation struct {
	ID     string
	Status OpStatus
	Reason string
}

// DescribeOperation returns a stack set operation using ctx.
func (ss *StackSet) DescribeOperation(ctx context.Context, name, opID string) (Operation, error) {
	resp, err := ss.client.DescribeStackSetOperation(ctx, &cloudformation.DescribeStackSetOperationInput{
		StackSetName: awsv2.String(name),
		OperationId:  awsv2.String(opID),
	})
	if err != nil {
		return Operation{}, fmt.Errorf("describe operation %s for stack set %s: %w", opID, name, err)
	}
	return Operation{
		ID:     opID,
		Status: OpStatus(resp.StackSetOperation.Status),
		Reason: awsv2.ToString(resp.StackSetOperation.StatusReason),
	}, nil
}

// Update updates a stack set using ctx.
func (ss *StackSet) Update(ctx context.Context, name, template string, opts ...CreateOrUpdateOption) (string, error) {
	return ss.update(ctx, name, template, opts...)
}

// UpdateAndWait updates a stack set with a new template, and waits until the operation completes.
func (ss *StackSet) UpdateAndWait(ctx context.Context, name, template string, opts ...CreateOrUpdateOption) error {
	id, err := ss.update(ctx, name, template, opts...)
	if err != nil {
		return err
	}
	return ss.WaitForOperation(ctx, name, id)
}

func (ss *StackSet) getInstanceSummaries(ctx context.Context, name string) ([]InstanceSummary, error) {
	summaries, err := ss.InstanceSummaries(ctx, name)
	if err != nil {
		// If the stack set doesn't exist - just move on.
		if isNotFoundStackSet(errors.Unwrap(err)) {
			return nil, &ErrStackSetNotFound{
				name: name,
			}
		}
		return nil, err
	}

	if len(summaries) == 0 {
		return nil, &ErrStackSetInstancesNotFound{
			name: name,
		}
	}
	return summaries, nil
}

// DeleteInstance deletes a stack set instance using ctx.
func (ss *StackSet) DeleteInstance(ctx context.Context, name, account, region string) (string, error) {
	out, err := ss.client.DeleteStackInstances(ctx, &cloudformation.DeleteStackInstancesInput{
		StackSetName: awsv2.String(name),
		Accounts:     []string{account},
		Regions:      []string{region},
		RetainStacks: awsv2.Bool(false),
	})
	if err != nil {
		return "", fmt.Errorf("delete stack instance in region %v for account %v for stackset %s: %w",
			region, account, name, err)
	}
	return awsv2.ToString(out.OperationId), nil
}

// DeleteAllInstances removes all stack instances using ctx.
func (ss *StackSet) DeleteAllInstances(ctx context.Context, name string) (string, error) {
	summaries, err := ss.getInstanceSummaries(ctx, name)
	if err != nil {
		return "", err
	}

	// We want to delete all the stack instances, so we create a set of account IDs and regions.
	uniqueAccounts := make(map[string]bool)
	uniqueRegions := make(map[string]bool)
	for _, summary := range summaries {
		uniqueAccounts[summary.Account] = true
		uniqueRegions[summary.Region] = true
	}

	var regions []string
	var accounts []string
	for account := range uniqueAccounts {
		accounts = append(accounts, account)
	}
	for region := range uniqueRegions {
		regions = append(regions, region)
	}

	out, err := ss.client.DeleteStackInstances(ctx, &cloudformation.DeleteStackInstancesInput{
		StackSetName: awsv2.String(name),
		Accounts:     accounts,
		Regions:      regions,
		RetainStacks: awsv2.Bool(false),
	})
	if err != nil {
		return "", fmt.Errorf("delete stack instances in regions %v for accounts %v for stackset %s: %w",
			regions, accounts, name, err)
	}
	return awsv2.ToString(out.OperationId), nil
}

// Delete deletes a stack set using ctx.
func (ss *StackSet) Delete(ctx context.Context, name string) error {
	if _, err := ss.client.DeleteStackSet(ctx, &cloudformation.DeleteStackSetInput{
		StackSetName: awsv2.String(name),
	}); err != nil {
		if !isNotFoundStackSet(err) {
			return fmt.Errorf("delete stack set %s: %w", name, err)
		}
	}
	return nil
}

// CreateInstances creates stack instances using ctx.
func (ss *StackSet) CreateInstances(ctx context.Context, name string, accounts, regions []string) (string, error) {
	return ss.createInstances(ctx, name, accounts, regions)
}

// CreateInstancesAndWait creates new stack instances in the regions of the specified AWS accounts, and waits until the operation completes.
func (ss *StackSet) CreateInstancesAndWait(ctx context.Context, name string, accounts, regions []string) error {
	id, err := ss.createInstances(ctx, name, accounts, regions)
	if err != nil {
		return err
	}
	return ss.WaitForOperation(ctx, name, id)
}

// InstanceSummary represents the identifiers for a stack instance.
type InstanceSummary struct {
	StackID string
	Account string
	Region  string
	Status  InstanceStatus
}

// InstanceSummariesOption allows to filter instance summaries to retrieve for the stack set.
type InstanceSummariesOption func(input *cloudformation.ListStackInstancesInput)

// InstanceSummaries returns stack instance summaries using ctx.
func (ss *StackSet) InstanceSummaries(ctx context.Context, name string, opts ...InstanceSummariesOption) ([]InstanceSummary, error) {
	in := &cloudformation.ListStackInstancesInput{
		StackSetName: awsv2.String(name),
	}
	for _, opt := range opts {
		opt(in)
	}

	var summaries []InstanceSummary
	for {
		resp, err := ss.client.ListStackInstances(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("list stack instances for stack set %s: %w", name, err)
		}
		for _, cfnSummary := range resp.Summaries {
			summary := InstanceSummary{
				StackID: awsv2.ToString(cfnSummary.StackId),
				Account: awsv2.ToString(cfnSummary.Account),
				Region:  awsv2.ToString(cfnSummary.Region),
			}
			if status := cfnSummary.StackInstanceStatus; status != nil {
				summary.Status = InstanceStatus(status.DetailedStatus)
			}
			summaries = append(summaries, summary)
		}
		in.NextToken = resp.NextToken
		if in.NextToken == nil {
			break
		}
	}
	return summaries, nil
}

func (ss *StackSet) update(ctx context.Context, name, template string, opts ...CreateOrUpdateOption) (string, error) {
	in := &cloudformation.UpdateStackSetInput{
		StackSetName: awsv2.String(name),
		TemplateBody: awsv2.String(template),
		OperationPreferences: &types.StackSetOperationPreferences{
			RegionConcurrencyType: types.RegionConcurrencyTypeParallel,
		},
	}
	for _, opt := range opts {
		opt(in)
	}
	resp, err := ss.client.UpdateStackSet(ctx, in)
	if err != nil {
		if isOutdatedStackSet(err) {
			return "", &ErrStackSetOutOfDate{
				name:      name,
				parentErr: err,
			}
		}
		return "", fmt.Errorf("update stack set %s: %w", name, err)
	}
	return awsv2.ToString(resp.OperationId), nil
}

func (ss *StackSet) createInstances(ctx context.Context, name string, accounts, regions []string) (string, error) {
	resp, err := ss.client.CreateStackInstances(ctx, &cloudformation.CreateStackInstancesInput{
		StackSetName: awsv2.String(name),
		Accounts:     accounts,
		Regions:      regions,
	})
	if err != nil {
		return "", fmt.Errorf("create stack instances for stack set %s in regions %v for accounts %v: %w",
			name, regions, accounts, err)
	}
	return awsv2.ToString(resp.OperationId), nil
}

// WaitForStackSetLastOperationComplete waits using ctx.
func (ss *StackSet) WaitForStackSetLastOperationComplete(ctx context.Context, name string) error {
	for {
		resp, err := ss.client.ListStackSetOperations(ctx, &cloudformation.ListStackSetOperationsInput{
			StackSetName: awsv2.String(name),
		})
		if err != nil {
			return fmt.Errorf("list operations for stack set %s: %w", name, err)
		}
		if len(resp.Summaries) == 0 {
			return nil
		}
		operation := resp.Summaries[0]
		switch operation.Status {
		case types.StackSetOperationStatusRunning:
		case types.StackSetOperationStatusStopping:
		case types.StackSetOperationStatusQueued:
		default:
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// WaitForOperation waits for an operation using ctx.
func (ss *StackSet) WaitForOperation(ctx context.Context, name, opID string) error {
	for {
		response, err := ss.client.DescribeStackSetOperation(ctx, &cloudformation.DescribeStackSetOperationInput{
			StackSetName: awsv2.String(name),
			OperationId:  awsv2.String(opID),
		})
		if err != nil {
			return fmt.Errorf("describe operation %s for stack set %s: %w", opID, name, err)
		}
		if OpStatus(response.StackSetOperation.Status) == opStatusSucceeded {
			return nil
		}
		if OpStatus(response.StackSetOperation.Status) == opStatusStopped {
			return fmt.Errorf("operation %s for stack set %s was manually stopped", opID, name)
		}
		if OpStatus(response.StackSetOperation.Status) == opStatusFailed {
			return fmt.Errorf("operation %s for stack set %s failed", opID, name)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// WithDescription sets a description for a stack set.
func WithDescription(description string) CreateOrUpdateOption {
	return func(input interface{}) {
		switch v := input.(type) {
		case *cloudformation.CreateStackSetInput:
			{
				v.Description = awsv2.String(description)
			}
		case *cloudformation.UpdateStackSetInput:
			{
				v.Description = awsv2.String(description)
			}
		}
	}
}

// WithExecutionRoleName sets an execution role name for a stack set.
func WithExecutionRoleName(roleName string) CreateOrUpdateOption {
	return func(input interface{}) {
		switch v := input.(type) {
		case *cloudformation.CreateStackSetInput:
			{
				v.ExecutionRoleName = awsv2.String(roleName)
			}
		case *cloudformation.UpdateStackSetInput:
			{
				v.ExecutionRoleName = awsv2.String(roleName)
			}
		}
	}
}

// WithAdministrationRoleARN sets an administration role arn for a stack set.
func WithAdministrationRoleARN(roleARN string) CreateOrUpdateOption {
	return func(input interface{}) {
		switch v := input.(type) {
		case *cloudformation.CreateStackSetInput:
			{
				v.AdministrationRoleARN = awsv2.String(roleARN)
			}
		case *cloudformation.UpdateStackSetInput:
			{
				v.AdministrationRoleARN = awsv2.String(roleARN)
			}
		}
	}
}

// WithTags sets tags to all the resources in a stack set.
func WithTags(tags map[string]string) CreateOrUpdateOption {
	return func(input interface{}) {
		var flatTags []types.Tag
		for k, v := range tags {
			flatTags = append(flatTags, types.Tag{
				Key:   awsv2.String(k),
				Value: awsv2.String(v),
			})
		}

		switch v := input.(type) {
		case *cloudformation.CreateStackSetInput:
			{
				v.Tags = flatTags
			}
		case *cloudformation.UpdateStackSetInput:
			{
				v.Tags = flatTags
			}
		}
	}
}

// WithOperationID sets the operation ID of a stack set operation.
// This functional option can only be used while updating a stack set, otherwise it's a no-op.
func WithOperationID(operationID string) CreateOrUpdateOption {
	return func(input interface{}) {
		switch v := input.(type) {
		case *cloudformation.UpdateStackSetInput:
			{
				v.OperationId = awsv2.String(operationID)
			}
		}
	}
}

// FilterSummariesByAccountID limits the accountID for the stack instance summaries to retrieve.
func FilterSummariesByAccountID(accountID string) InstanceSummariesOption {
	return func(input *cloudformation.ListStackInstancesInput) {
		input.StackInstanceAccount = awsv2.String(accountID)
	}
}

// FilterSummariesByRegion limits the region for the stack instance summaries to retrieve.
func FilterSummariesByRegion(region string) InstanceSummariesOption {
	return func(input *cloudformation.ListStackInstancesInput) {
		input.StackInstanceRegion = awsv2.String(region)
	}
}

// FilterSummariesByDetailedStatus limits the stack instance summaries to the passed status values.
func FilterSummariesByDetailedStatus(values []InstanceStatus) InstanceSummariesOption {
	return func(input *cloudformation.ListStackInstancesInput) {
		for _, value := range values {
			input.Filters = append(input.Filters, types.StackInstanceFilter{
				Name:   types.StackInstanceFilterNameDetailedStatus,
				Values: awsv2.String(string(value)),
			})
		}
	}
}
