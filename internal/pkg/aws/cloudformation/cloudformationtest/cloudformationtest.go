// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cloudformationtest

import (
	"context"

	cfn "github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	sdk "github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

// Double is a test double for cloudformation.CloudFormation
type Double struct {
	CreateFn                    func(stack *cfn.Stack) (string, error)
	CreateAndWaitFn             func(stack *cfn.Stack) error
	DescribeChangeSetFn         func(changeSetID, stackName string) (*cfn.ChangeSetDescription, error)
	WaitForCreateFn             func(ctx context.Context, stackName string) error
	UpdateFn                    func(stack *cfn.Stack) (string, error)
	UpdateAndWaitFn             func(stack *cfn.Stack) error
	WaitForUpdateFn             func(ctx context.Context, stackName string) error
	DeleteFn                    func(stackName string) error
	DeleteAndWaitFn             func(stackName string) error
	DeleteAndWaitWithRoleARNFn  func(stackName, roleARN string) error
	DescribeFn                  func(name string) (*cfn.StackDescription, error)
	ExistsFn                    func(name string) (bool, error)
	MetadataFn                  func(opt cfn.MetadataOpts) (string, error)
	TemplateBodyFn              func(name string) (string, error)
	TemplateBodyFromChangeSetFn func(changeSetID, stackName string) (string, error)
	OutputsFn                   func(stack *cfn.Stack) (map[string]string, error)
	EventsFn                    func(stackName string) ([]cfn.StackEvent, error)
	StackResourcesFn            func(name string) ([]*cfn.StackResource, error)
	ErrorEventsFn               func(stackName string) ([]cfn.StackEvent, error)
	ListStacksWithTagsFn        func(tags map[string]string) ([]cfn.StackDescription, error)
	DescribeStackEventsFn       func(input *sdk.DescribeStackEventsInput) (*sdk.DescribeStackEventsOutput, error)
	CancelUpdateStackFn         func(stackName string) error
}

// Create calls the stubbed function.
func (d *Double) Create(_ context.Context, stack *cfn.Stack) (string, error) {
	return d.CreateFn(stack)
}

// CreateAndWait calls the stubbed function.
func (d *Double) CreateAndWait(_ context.Context, stack *cfn.Stack) error {
	return d.CreateAndWaitFn(stack)
}

// DescribeChangeSet calls the stubbed function.
func (d *Double) DescribeChangeSet(_ context.Context, id, stack string) (*cfn.ChangeSetDescription, error) {
	return d.DescribeChangeSetFn(id, stack)
}

// WaitForCreate calls the stubbed function.
func (d *Double) WaitForCreate(ctx context.Context, stack string) error {
	return d.WaitForCreateFn(ctx, stack)
}

// Update calls the stubbed function.
func (d *Double) Update(_ context.Context, stack *cfn.Stack) (string, error) {
	return d.UpdateFn(stack)
}

// UpdateAndWait calls the stubbed function.
func (d *Double) UpdateAndWait(_ context.Context, stack *cfn.Stack) error {
	return d.UpdateAndWaitFn(stack)
}

// WaitForUpdate calls the stubbed function.
func (d *Double) WaitForUpdate(ctx context.Context, stackName string) error {
	return d.WaitForUpdateFn(ctx, stackName)
}

// Delete calls the stubbed function.
func (d *Double) Delete(_ context.Context, stackName string) error {
	return d.DeleteFn(stackName)
}

// DeleteAndWait calls the stubbed function.
func (d *Double) DeleteAndWait(_ context.Context, stackName string) error {
	return d.DeleteAndWaitFn(stackName)
}

// DeleteAndWaitWithRoleARN calls the stubbed function.
func (d *Double) DeleteAndWaitWithRoleARN(_ context.Context, stackName, roleARN string) error {
	return d.DeleteAndWaitWithRoleARNFn(stackName, roleARN)
}

// Describe calls the stubbed function.
func (d *Double) Describe(_ context.Context, name string) (*cfn.StackDescription, error) {
	return d.DescribeFn(name)
}

// Exists calls the stubbed function.
func (d *Double) Exists(_ context.Context, name string) (bool, error) {
	return d.ExistsFn(name)
}

// Metadata calls the stubbed function.
func (d *Double) Metadata(_ context.Context, opt cfn.MetadataOpts) (string, error) {
	return d.MetadataFn(opt)
}

// TemplateBody calls the stubbed function.
func (d *Double) TemplateBody(_ context.Context, name string) (string, error) {
	return d.TemplateBodyFn(name)
}

// TemplateBodyFromChangeSet calls the stubbed function.
func (d *Double) TemplateBodyFromChangeSet(_ context.Context, changeSetID, stackName string) (string, error) {
	return d.TemplateBodyFromChangeSetFn(changeSetID, stackName)
}

// Outputs calls the stubbed function.
func (d *Double) Outputs(_ context.Context, stack *cfn.Stack) (map[string]string, error) {
	return d.OutputsFn(stack)
}

// Events calls the stubbed function.
func (d *Double) Events(_ context.Context, stackName string) ([]cfn.StackEvent, error) {
	return d.EventsFn(stackName)
}

// StackResources calls the stubbed function.
func (d *Double) StackResources(_ context.Context, name string) ([]*cfn.StackResource, error) {
	return d.StackResourcesFn(name)
}

// ErrorEvents calls the stubbed function.
func (d *Double) ErrorEvents(_ context.Context, stackName string) ([]cfn.StackEvent, error) {
	return d.ErrorEventsFn(stackName)
}

// ListStacksWithTags calls the stubbed function.
func (d *Double) ListStacksWithTags(_ context.Context, tags map[string]string) ([]cfn.StackDescription, error) {
	return d.ListStacksWithTagsFn(tags)
}

// DescribeStackEvents calls the stubbed function.
func (d *Double) DescribeStackEvents(_ context.Context, input *sdk.DescribeStackEventsInput) (*sdk.DescribeStackEventsOutput, error) {
	return d.DescribeStackEventsFn(input)
}

// CancelUpdateStack calls the stubbed function.
func (d *Double) CancelUpdateStack(_ context.Context, stackName string) error {
	return d.CancelUpdateStackFn(stackName)
}
