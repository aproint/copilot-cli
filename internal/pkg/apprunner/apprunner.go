// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package apprunner provides a client to retrieve Copilot App Runner information.
package apprunner

import (
	"context"
	"fmt"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/apprunner"
	"github.com/aproint/copilot-cli/internal/pkg/aws/resourcegroups"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
)

const (
	serviceResourceType = "apprunner:service"
)

type appRunnerClient interface {
	StartDeployment(ctx context.Context, svcARN string) (string, error)
	DescribeService(ctx context.Context, svcARN string) (*apprunner.Service, error)
	WaitForOperation(ctx context.Context, operationId, svcARN string) error
}

type resourceGetter interface {
	GetResourcesByTags(ctx context.Context, resourceType string, tags map[string]string) ([]*resourcegroups.Resource, error)
}

// Client retrieves Copilot information from App Runner endpoint.
type Client struct {
	appRunnerClient appRunnerClient
	rgGetter        resourceGetter
}

// New inits a new Client.
func New(rgConfig awsv2.Config) *Client {
	return &Client{
		rgGetter:        resourcegroups.New(rgConfig),
		appRunnerClient: apprunner.New(rgConfig),
	}
}

// ForceUpdateService forces a new update using ctx for discovery, deployment, and waiting.
func (c Client) ForceUpdateService(ctx context.Context, app, env, svc string) error {
	svcARN, err := c.serviceARN(ctx, app, env, svc)
	if err != nil {
		return err
	}
	id, err := c.appRunnerClient.StartDeployment(ctx, svcARN)
	if err != nil {
		return err
	}
	return c.appRunnerClient.WaitForOperation(ctx, id, svcARN)
}

// LastUpdatedAt returns the last service update time using ctx.
func (c Client) LastUpdatedAt(ctx context.Context, app, env, svc string) (time.Time, error) {
	svcARN, err := c.serviceARN(ctx, app, env, svc)
	if err != nil {
		return time.Time{}, err
	}
	desc, err := c.appRunnerClient.DescribeService(ctx, svcARN)
	if err != nil {
		return time.Time{}, fmt.Errorf("describe service: %w", err)
	}
	return desc.DateUpdated, nil
}

func (c Client) serviceARN(ctx context.Context, app, env, svc string) (string, error) {
	services, err := c.rgGetter.GetResourcesByTags(ctx, serviceResourceType, map[string]string{
		deploy.AppTagKey:     app,
		deploy.EnvTagKey:     env,
		deploy.ServiceTagKey: svc,
	})
	if err != nil {
		return "", fmt.Errorf("get App Runner service with tags (%s, %s, %s): %w", app, env, svc, err)
	}
	if len(services) == 0 {
		return "", fmt.Errorf("no App Runner service found for %s in environment %s", svc, env)
	}
	if len(services) > 1 {
		return "", fmt.Errorf("more than one App Runner service with the name %s found in environment %s", svc, env)
	}
	return services[0].ARN, nil
}
