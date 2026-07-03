// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package codestar provides a client to make API requests to AWS CodeStar Connections.
package codestar

import (
	"context"
	"fmt"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codestarconnections"
	"github.com/aws/aws-sdk-go-v2/service/codestarconnections/types"
)

type api interface {
	GetConnection(ctx context.Context, input *codestarconnections.GetConnectionInput, opts ...func(*codestarconnections.Options)) (*codestarconnections.GetConnectionOutput, error)
	ListConnections(ctx context.Context, input *codestarconnections.ListConnectionsInput, opts ...func(*codestarconnections.Options)) (*codestarconnections.ListConnectionsOutput, error)
}

// CodeStar represents a client to make requests to AWS CodeStarConnections.
type CodeStar struct {
	client api
}

// New creates a new CodeStar client.
func New(cfg awsv2.Config) *CodeStar {
	return &CodeStar{
		codestarconnections.NewFromConfig(cfg),
	}
}

// WaitUntilConnectionStatusAvailable blocks until the connection status has been updated from `PENDING` to `AVAILABLE` or until the max attempt window expires.
func (c *CodeStar) WaitUntilConnectionStatusAvailable(ctx context.Context, connectionARN string) error {
	var interval time.Duration // Defaults to 0.
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for connection %s status to change from PENDING to AVAILABLE", connectionARN)
		case <-time.After(interval):
			output, err := c.client.GetConnection(ctx, &codestarconnections.GetConnectionInput{ConnectionArn: awsv2.String(connectionARN)})
			if err != nil {
				return fmt.Errorf("get connection details for %s: %w", connectionARN, err)
			}
			if output.Connection.ConnectionStatus == types.ConnectionStatusAvailable {
				return nil
			}
			interval = 5 * time.Second
		}
	}
}

// GetConnectionARN retrieves all of the CSC connections in the current account and returns the ARN correlating to the
// connection name passed in.
func (c *CodeStar) GetConnectionARN(connectionName string) (connectionARN string, err error) {
	output, err := c.client.ListConnections(context.Background(), &codestarconnections.ListConnectionsInput{})
	if err != nil {
		return "", fmt.Errorf("get list of connections in AWS account: %w", err)
	}
	connections := output.Connections
	for output.NextToken != nil {
		output, err = c.client.ListConnections(context.Background(), &codestarconnections.ListConnectionsInput{
			NextToken: output.NextToken,
		})
		if err != nil {
			return "", fmt.Errorf("get list of connections in AWS account: %w", err)
		}
		connections = append(connections, output.Connections...)
	}

	for _, connection := range connections {
		if awsv2.ToString(connection.ConnectionName) == connectionName {
			// Duplicate connection names are supposed to result in replacement, so okay to return first match.
			return awsv2.ToString(connection.ConnectionArn), nil
		}
	}
	return "", fmt.Errorf("cannot find a connectionARN associated with %s", connectionName)
}
