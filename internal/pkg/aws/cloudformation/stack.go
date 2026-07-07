// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
)

// Stack represents a AWS CloudFormation stack.
type Stack struct {
	Name string
	*stackConfig
}

type stackConfig struct {
	TemplateBody    string
	TemplateURL     string
	Parameters      []types.Parameter
	Tags            []types.Tag
	RoleARN         *string
	DisableRollback bool
}

// StackOption allows you to initialize a Stack with additional properties.
type StackOption func(s *Stack)

// NewStack creates a stack with the given name and template body.
func NewStack(name, template string, opts ...StackOption) *Stack {
	s := &Stack{
		Name: name,
		stackConfig: &stackConfig{
			TemplateBody: template,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NewStackWithURL creates a stack with a URL to the template.
func NewStackWithURL(name, templateURL string, opts ...StackOption) *Stack {
	s := &Stack{
		Name: name,
		stackConfig: &stackConfig{
			TemplateURL: templateURL,
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithParameters passes parameters to a stack.
func WithParameters(params map[string]string) StackOption {
	return func(s *Stack) {
		var flatParams []types.Parameter
		for k, v := range params {
			flatParams = append(flatParams, types.Parameter{
				ParameterKey:   awsv2.String(k),
				ParameterValue: awsv2.String(v),
			})
		}
		s.Parameters = flatParams
	}
}

// WithTags applies the tags to a stack.
func WithTags(tags map[string]string) StackOption {
	return func(s *Stack) {
		var flatTags []types.Tag
		for k, v := range tags {
			flatTags = append(flatTags, types.Tag{
				Key:   awsv2.String(k),
				Value: awsv2.String(v),
			})
		}
		s.Tags = flatTags
	}
}

// WithRoleARN specifies the role that CloudFormation will assume when creating the stack.
func WithRoleARN(roleARN string) StackOption {
	return func(s *Stack) {
		s.RoleARN = awsv2.String(roleARN)
	}
}

// WithDisableRollback disables CloudFormation's automatic stack rollback upon failure for the stack.
func WithDisableRollback() StackOption {
	return func(s *Stack) {
		s.DisableRollback = true
	}
}

// StackEvent is an alias the SDK's StackEvent type.
type StackEvent types.StackEvent

// StackDescription is an alias the SDK's Stack type.
type StackDescription types.Stack

// StackResource is an alias the SDK's StackResource type.
type StackResource types.StackResource

// SDK returns the underlying struct from the AWS SDK.
func (d *StackDescription) SDK() *types.Stack {
	raw := types.Stack(*d)
	return &raw
}
