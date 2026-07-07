// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
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

// Create creates a new stack set resource, if one already exists then do nothing.
func (ss *StackSet) Create(name, template string, opts ...CreateOrUpdateOption) error {
	in := &cloudformation.CreateStackSetInput{
		StackSetName: awsv2.String(name),
		TemplateBody: awsv2.String(template),
	}
	for _, opt := range opts {
		opt(in)
	}
	_, err := ss.client.CreateStackSet(context.Background(), in)
	if err != nil {
		if !isAlreadyExistingStackSet(err) {
			return fmt.Errorf("create stack set %s: %w", name, err)
		}
	}
	return nil
}

// Describe returns a description of a created stack set.
func (ss *StackSet) Describe(name string) (Description, error) {
	resp, err := ss.client.DescribeStackSet(context.Background(), &cloudformation.DescribeStackSetInput{
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

// DescribeOperation returns a description of the operation.
func (ss *StackSet) DescribeOperation(name, opID string) (Operation, error) {
	resp, err := ss.client.DescribeStackSetOperation(context.Background(), &cloudformation.DescribeStackSetOperationInput{
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

// Update updates all the instances in a stack set with the new template and returns the operation ID.
func (ss *StackSet) Update(name, template string, opts ...CreateOrUpdateOption) (string, error) {
	return ss.update(name, template, opts...)
}

// UpdateAndWait updates a stack set with a new template, and waits until the operation completes.
func (ss *StackSet) UpdateAndWait(name, template string, opts ...CreateOrUpdateOption) error {
	id, err := ss.update(name, template, opts...)
	if err != nil {
		return err
	}
	return ss.WaitForOperation(name, id)
}

func (ss *StackSet) getInstanceSummaries(name string) ([]InstanceSummary, error) {
	summaries, err := ss.InstanceSummaries(name)
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

// DeleteInstance deletes the stackset instance for the stackset with the given name in the given account
// and region and returns the operation ID.
// If there is no instance in the given account and region, this function will return an operation ID
// but the API call will take no action.
func (ss *StackSet) DeleteInstance(name, account, region string) (string, error) {
	out, err := ss.client.DeleteStackInstances(context.Background(), &cloudformation.DeleteStackInstancesInput{
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

// DeleteAllInstances removes all stack instances from a stack set and returns the operation ID.
// If the stack set does not exist, then return [ErrStackSetNotFound].
// If the stack set does not have any instances, then return [ErrStackSetInstancesNotFound].
// Both errors should satisfy [IsEmptyStackSetErr], otherwise it's an unexpected error.
func (ss *StackSet) DeleteAllInstances(name string) (string, error) {
	summaries, err := ss.getInstanceSummaries(name)
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

	out, err := ss.client.DeleteStackInstances(context.Background(), &cloudformation.DeleteStackInstancesInput{
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

// Delete deletes the stack set, if the stack set does not exist then just return nil.
func (ss *StackSet) Delete(name string) error {
	if _, err := ss.client.DeleteStackSet(context.Background(), &cloudformation.DeleteStackSetInput{
		StackSetName: awsv2.String(name),
	}); err != nil {
		if !isNotFoundStackSet(err) {
			return fmt.Errorf("delete stack set %s: %w", name, err)
		}
	}
	return nil
}

// CreateInstances creates new stack instances within the regions
// of the specified AWS accounts and returns the operation ID.
func (ss *StackSet) CreateInstances(name string, accounts, regions []string) (string, error) {
	return ss.createInstances(name, accounts, regions)
}

// CreateInstancesAndWait creates new stack instances in the regions of the specified AWS accounts, and waits until the operation completes.
func (ss *StackSet) CreateInstancesAndWait(name string, accounts, regions []string) error {
	id, err := ss.createInstances(name, accounts, regions)
	if err != nil {
		return err
	}
	return ss.WaitForOperation(name, id)
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

// InstanceSummaries returns a list of unique identifiers for all the stack instances in a stack set.
func (ss *StackSet) InstanceSummaries(name string, opts ...InstanceSummariesOption) ([]InstanceSummary, error) {
	in := &cloudformation.ListStackInstancesInput{
		StackSetName: awsv2.String(name),
	}
	for _, opt := range opts {
		opt(in)
	}

	var summaries []InstanceSummary
	for {
		resp, err := ss.client.ListStackInstances(context.Background(), in)
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

func (ss *StackSet) update(name, template string, opts ...CreateOrUpdateOption) (string, error) {
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
	resp, err := ss.client.UpdateStackSet(context.Background(), in)
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

func (ss *StackSet) createInstances(name string, accounts, regions []string) (string, error) {
	resp, err := ss.client.CreateStackInstances(context.Background(), &cloudformation.CreateStackInstancesInput{
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

// WaitForStackSetLastOperationComplete waits until the stackset's last operation completes.
func (ss *StackSet) WaitForStackSetLastOperationComplete(name string) error {
	for {
		resp, err := ss.client.ListStackSetOperations(context.Background(), &cloudformation.ListStackSetOperationsInput{
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
		time.Sleep(3 * time.Second)
	}
}

// WaitForOperation waits for the operation with opID to reaches a successful completion status.
func (ss *StackSet) WaitForOperation(name, opID string) error {
	for {
		response, err := ss.client.DescribeStackSetOperation(context.Background(), &cloudformation.DescribeStackSetOperationInput{
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
		time.Sleep(3 * time.Second)
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
