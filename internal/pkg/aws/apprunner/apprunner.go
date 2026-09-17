// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

// Package apprunner provides a client to make API requests to AppRunner Service.
package apprunner

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/apprunner"
	"github.com/aws/aws-sdk-go-v2/service/apprunner/types"
)

const (
	fmtAppRunnerServiceLogGroupName     = "/aws/apprunner/%s/%s/service"
	fmtAppRunnerApplicationLogGroupName = "/aws/apprunner/%s/%s/application"

	// App Runner Statuses
	opStatusSucceeded = "SUCCEEDED"
	opStatusFailed    = "FAILED"
	svcStatusPaused   = "PAUSED"
	svcStatusRunning  = "RUNNING"

	// App Runner ImageRepositoryTypes
	repositoryTypeECR       = "ECR"
	repositoryTypeECRPublic = "ECR_PUBLIC"

	// EndpointsID is the ID to look up the App Runner service endpoint.
	EndpointsID = "apprunner"
)

type api interface {
	DescribeService(ctx context.Context, input *apprunner.DescribeServiceInput, opts ...func(*apprunner.Options)) (*apprunner.DescribeServiceOutput, error)
	ListOperations(ctx context.Context, input *apprunner.ListOperationsInput, opts ...func(*apprunner.Options)) (*apprunner.ListOperationsOutput, error)
	ListServices(ctx context.Context, input *apprunner.ListServicesInput, opts ...func(*apprunner.Options)) (*apprunner.ListServicesOutput, error)
	PauseService(ctx context.Context, input *apprunner.PauseServiceInput, opts ...func(*apprunner.Options)) (*apprunner.PauseServiceOutput, error)
	ResumeService(ctx context.Context, input *apprunner.ResumeServiceInput, opts ...func(*apprunner.Options)) (*apprunner.ResumeServiceOutput, error)
	StartDeployment(ctx context.Context, input *apprunner.StartDeploymentInput, opts ...func(*apprunner.Options)) (*apprunner.StartDeploymentOutput, error)
	DescribeObservabilityConfiguration(ctx context.Context, input *apprunner.DescribeObservabilityConfigurationInput, opts ...func(*apprunner.Options)) (*apprunner.DescribeObservabilityConfigurationOutput, error)
	DescribeVpcIngressConnection(ctx context.Context, input *apprunner.DescribeVpcIngressConnectionInput, opts ...func(*apprunner.Options)) (*apprunner.DescribeVpcIngressConnectionOutput, error)
}

// AppRunner wraps an AWS AppRunner client.
type AppRunner struct {
	client api
}

// New returns a Service configured against the input config.
func New(cfg awsv2.Config) *AppRunner {
	return &AppRunner{
		client: apprunner.NewFromConfig(cfg),
	}
}

// DescribeService returns a description of an AppRunner service using ctx.
func (a *AppRunner) DescribeService(ctx context.Context, svcARN string) (*Service, error) {
	resp, err := a.client.DescribeService(ctx, &apprunner.DescribeServiceInput{
		ServiceArn: awsv2.String(svcARN),
	})
	if err != nil {
		return nil, fmt.Errorf("describe service %s: %w", svcARN, err)
	}
	var envVars []*EnvironmentVariable
	for k, v := range resp.Service.SourceConfiguration.ImageRepository.ImageConfiguration.RuntimeEnvironmentVariables {
		envVars = append(envVars, &EnvironmentVariable{
			Name:  k,
			Value: v,
		})
	}
	sort.SliceStable(envVars, func(i int, j int) bool { return envVars[i].Name < envVars[j].Name })

	var secrets []*EnvironmentSecret
	for k, v := range resp.Service.SourceConfiguration.ImageRepository.ImageConfiguration.RuntimeEnvironmentSecrets {
		secrets = append(secrets, &EnvironmentSecret{
			Name:  k,
			Value: v,
		})
	}
	sort.SliceStable(secrets, func(i int, j int) bool { return secrets[i].Name < secrets[j].Name })

	var observabilityConfiguration ObservabilityConfiguration
	if resp.Service.ObservabilityConfiguration != nil && resp.Service.ObservabilityConfiguration.ObservabilityEnabled {
		if out, err := a.client.DescribeObservabilityConfiguration(ctx, &apprunner.DescribeObservabilityConfigurationInput{
			ObservabilityConfigurationArn: resp.Service.ObservabilityConfiguration.ObservabilityConfigurationArn,
		}); err == nil {
			// NOTE: swallow the error otherwise, because observability is an optional description of the service.
			// Example error: when "EnvManagerRole" doesn't have the "apprunner:ObservabilityConfiguration" permission.
			observabilityConfiguration = ObservabilityConfiguration{
				TraceConfiguration: (*TraceConfiguration)(out.ObservabilityConfiguration.TraceConfiguration),
			}
		}
	}
	return &Service{
		ServiceARN:           awsv2.ToString(resp.Service.ServiceArn),
		Name:                 awsv2.ToString(resp.Service.ServiceName),
		ID:                   awsv2.ToString(resp.Service.ServiceId),
		Status:               string(resp.Service.Status),
		ServiceURL:           awsv2.ToString(resp.Service.ServiceUrl),
		DateCreated:          *resp.Service.CreatedAt,
		DateUpdated:          *resp.Service.UpdatedAt,
		EnvironmentVariables: envVars,
		CPU:                  awsv2.ToString(resp.Service.InstanceConfiguration.Cpu),
		Memory:               awsv2.ToString(resp.Service.InstanceConfiguration.Memory),
		ImageID:              awsv2.ToString(resp.Service.SourceConfiguration.ImageRepository.ImageIdentifier),
		Port:                 awsv2.ToString(resp.Service.SourceConfiguration.ImageRepository.ImageConfiguration.Port),
		Observability:        observabilityConfiguration,
		EnvironmentSecrets:   secrets,
	}, nil
}

// ServiceARN returns the ARN of an App Runner service using ctx for every page.
func (a *AppRunner) ServiceARN(ctx context.Context, svc string) (string, error) {
	var nextToken *string
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		resp, err := a.client.ListServices(ctx, &apprunner.ListServicesInput{
			NextToken: nextToken,
		})
		if err != nil {
			return "", fmt.Errorf("list AppRunner services: %w", err)
		}
		for _, service := range resp.ServiceSummaryList {
			if awsv2.ToString(service.ServiceName) == svc {
				return awsv2.ToString(service.ServiceArn), nil
			}
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return "", fmt.Errorf("no AppRunner service found for %s", svc)
}

// PauseService pauses the running App Runner service using ctx.
func (a *AppRunner) PauseService(ctx context.Context, svcARN string) error {
	resp, err := a.client.PauseService(ctx, &apprunner.PauseServiceInput{
		ServiceArn: awsv2.String(svcARN),
	})
	if err != nil {
		return fmt.Errorf("pause service operation failed: %w", err)
	}
	if resp.OperationId == nil && string(resp.Service.Status) == svcStatusPaused {
		return nil
	}
	if err := a.WaitForOperation(ctx, awsv2.ToString(resp.OperationId), svcARN); err != nil {
		return err
	}
	return nil
}

// ResumeService resumes a paused App Runner service using ctx.
func (a *AppRunner) ResumeService(ctx context.Context, svcARN string) error {
	resp, err := a.client.ResumeService(ctx, &apprunner.ResumeServiceInput{
		ServiceArn: awsv2.String(svcARN),
	})
	if err != nil {
		return fmt.Errorf("resume service operation failed: %w", err)
	}
	if resp.OperationId == nil && string(resp.Service.Status) == svcStatusRunning {
		return nil
	}
	if err := a.WaitForOperation(ctx, awsv2.ToString(resp.OperationId), svcARN); err != nil {
		return err
	}
	return nil
}

// StartDeployment initiates a manual deployment using ctx.
func (a *AppRunner) StartDeployment(ctx context.Context, svcARN string) (string, error) {
	out, err := a.client.StartDeployment(ctx, &apprunner.StartDeploymentInput{
		ServiceArn: awsv2.String(svcARN),
	})
	if err != nil {
		return "", fmt.Errorf("start new deployment: %w", err)
	}
	return awsv2.ToString(out.OperationId), nil
}

// DescribeOperation returns the operation summary using ctx for every page.
func (a *AppRunner) DescribeOperation(ctx context.Context, operationId, svcARN string) (*types.OperationSummary, error) {
	var nextToken *string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := a.client.ListOperations(ctx, &apprunner.ListOperationsInput{
			ServiceArn: awsv2.String(svcARN),
			NextToken:  nextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list operations: %w", err)
		}
		for i := range resp.OperationSummaryList {
			if awsv2.ToString(resp.OperationSummaryList[i].Id) == operationId {
				return &resp.OperationSummaryList[i], nil
			}
		}
		if resp.NextToken == nil {
			break
		}
		nextToken = resp.NextToken
	}
	return nil, fmt.Errorf("no operation found %s", operationId)
}

// WaitForOperation waits for a service operation using ctx.
func (a *AppRunner) WaitForOperation(ctx context.Context, operationId, svcARN string) error {
	for {
		resp, err := a.DescribeOperation(ctx, operationId, svcARN)
		if err != nil {
			return fmt.Errorf("error describing operation %s: %w", operationId, err)
		}
		switch status := string(resp.Status); status {
		case opStatusSucceeded:
			return nil
		case opStatusFailed:
			return &ErrWaitServiceOperationFailed{
				operationId: operationId,
			}
		}
		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// PrivateURL returns the URL associated with a VPC Ingress Connection using ctx.
func (a *AppRunner) PrivateURL(ctx context.Context, vicARN string) (string, error) {
	resp, err := a.client.DescribeVpcIngressConnection(ctx, &apprunner.DescribeVpcIngressConnectionInput{
		VpcIngressConnectionArn: awsv2.String(vicARN),
	})
	if err != nil {
		return "", fmt.Errorf("describe vpc ingress connection %q: %w", vicARN, err)
	}

	return awsv2.ToString(resp.VpcIngressConnection.DomainName), nil
}

// ParseServiceName returns the service name.
// For example: arn:aws:apprunner:us-west-2:1234567890:service/my-service/fc1098ac269245959ba78fd58bdd4bf
// will return my-service
func ParseServiceName(svcARN string) (string, error) {
	parsedARN, err := arn.Parse(svcARN)
	if err != nil {
		return "", err
	}
	resources := strings.Split(parsedARN.Resource, "/")
	if len(resources) != 3 {
		return "", fmt.Errorf("cannot parse resource for ARN %s", svcARN)
	}
	return resources[1], nil
}

// ParseServiceID returns the service id.
// For example: arn:aws:apprunner:us-west-2:1234567890:service/my-service/fc1098ac269245959ba78fd58bdd4bf
// will return fc1098ac269245959ba78fd58bdd4bf
func ParseServiceID(svcARN string) (string, error) {
	parsedARN, err := arn.Parse(svcARN)
	if err != nil {
		return "", err
	}
	resources := strings.Split(parsedARN.Resource, "/")
	if len(resources) != 3 {
		return "", fmt.Errorf("cannot parse resource for ARN %s", svcARN)
	}
	return resources[2], nil
}

// LogGroupName returns the log group name given the app runner service's name.
// An application log group is formatted as "/aws/apprunner/<svcName>/<svcID>/application".
func LogGroupName(svcARN string) (string, error) {
	svcName, err := ParseServiceName(svcARN)
	if err != nil {
		return "", fmt.Errorf("get service name: %w", err)
	}
	svcID, err := ParseServiceID(svcARN)
	if err != nil {
		return "", fmt.Errorf("get service id: %w", err)
	}
	return fmt.Sprintf(fmtAppRunnerApplicationLogGroupName, svcName, svcID), nil
}

// SystemLogGroupName returns the service log group name given the app runner service's name.
// A service log group is formatted as "/aws/apprunner/<svcName>/<svcID>/service".
func SystemLogGroupName(svcARN string) (string, error) {
	svcName, err := ParseServiceName(svcARN)
	if err != nil {
		return "", fmt.Errorf("get service name: %w", err)
	}
	svcID, err := ParseServiceID(svcARN)
	if err != nil {
		return "", fmt.Errorf("get service id: %w", err)
	}
	return fmt.Sprintf(fmtAppRunnerServiceLogGroupName, svcName, svcID), nil
}

// ImageIsSupported returns true if the image identifier is supported by App Runner.
func ImageIsSupported(imageIdentifier string) bool {
	return imageIsECR(imageIdentifier) || imageIsECRPublic(imageIdentifier)
}

// DetermineImageRepositoryType returns the App Runner ImageRepositoryType enum value for the provided image identifier,
// or returns an error if the imageIdentifier is not supported by App Runner or the ImageRepositoryType cannot be
// determined.
func DetermineImageRepositoryType(imageIdentifier string) (string, error) {
	if !ImageIsSupported(imageIdentifier) {
		return "", fmt.Errorf("image is not supported by App Runner: %s", imageIdentifier)
	}

	if imageIsECR(imageIdentifier) {
		return repositoryTypeECR, nil
	}

	if imageIsECRPublic(imageIdentifier) {
		return repositoryTypeECRPublic, nil
	}

	return "", fmt.Errorf("unable to determine the image repository type for image: %s", imageIdentifier)
}

func imageIsECR(imageIdentifier string) bool {
	matched, _ := regexp.Match(`^\d{12}\.dkr\.ecr\.[^\.]+\.amazonaws\.com/`, []byte(imageIdentifier))
	return matched
}

func imageIsECRPublic(imageIdentifier string) bool {
	matched, _ := regexp.Match(`^public\.ecr\.aws/`, []byte(imageIdentifier))
	return matched
}
