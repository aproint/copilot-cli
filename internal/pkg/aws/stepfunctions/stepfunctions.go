// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package stepfunctions provides a client to make API requests to Amazon Step Functions.
package stepfunctions

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

type api interface {
	DescribeStateMachine(ctx context.Context, input *sfn.DescribeStateMachineInput, opts ...func(*sfn.Options)) (*sfn.DescribeStateMachineOutput, error)
	StartExecution(ctx context.Context, input *sfn.StartExecutionInput, opts ...func(*sfn.Options)) (*sfn.StartExecutionOutput, error)
}

// StepFunctions wraps an AWS StepFunctions client.
type StepFunctions struct {
	client api
}

// New returns StepFunctions configured against the input SDK v2 config.
func New(cfg awsv2.Config) *StepFunctions {
	return &StepFunctions{
		client: sfn.NewFromConfig(cfg),
	}
}

// StateMachineDefinition returns the JSON-based state machine definition.
func (s *StepFunctions) StateMachineDefinition(stateMachineARN string) (string, error) {
	out, err := s.client.DescribeStateMachine(context.Background(), &sfn.DescribeStateMachineInput{
		StateMachineArn: awsv2.String(stateMachineARN),
	})
	if err != nil {
		return "", fmt.Errorf("describe state machine: %w", err)
	}

	return awsv2.ToString(out.Definition), nil
}

// Execute starts a state machine execution.
func (s *StepFunctions) Execute(arn string) error {
	_, err := s.client.StartExecution(context.Background(), &sfn.StartExecutionInput{
		StateMachineArn: awsv2.String(arn),
	})
	if err != nil {
		return fmt.Errorf("execute state machine %s: %w", arn, err)
	}
	return nil
}
