// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Application is a named collection of environments and services.
type Application struct {
	Name                string            `json:"name"`                          // Name of an Application. Must be unique amongst other apps in the same account.
	AccountID           string            `json:"account"`                       // AccountID this app is mastered in.
	PermissionsBoundary string            `json:"permissionsBoundary,omitempty"` // Existing IAM permissions boundary.
	Domain              string            `json:"domain"`                        // Existing domain name in Route53. An empty domain name means the user does not have one.
	DomainHostedZoneID  string            `json:"domainHostedZoneID"`            // Existing domain hosted zone in Route53. An empty domain name means the user does not have one.
	Version             string            `json:"version"`                       // The version of the app layout in the underlying datastore (e.g. SSM).
	Tags                map[string]string `json:"tags,omitempty"`                // Labels to apply to resources created within the app.
}

// CreateApplication instantiates a new application, validates its uniqueness and stores it in SSM.
func (s *Store) CreateApplication(ctx context.Context, application *Application) error {
	applicationPath := fmt.Sprintf(fmtApplicationPath, application.Name)
	application.Version = schemaVersion

	data, err := marshal(application)
	if err != nil {
		return fmt.Errorf("serializing application %s: %w", application.Name, err)
	}

	_, err = s.ssm.PutParameter(ctx, &ssm.PutParameterInput{
		Name:        aws.String(applicationPath),
		Description: aws.String("Copilot Application"),
		Type:        types.ParameterTypeString,
		Value:       aws.String(data),
		Tags: []types.Tag{
			{
				Key:   aws.String("copilot-application"),
				Value: aws.String(application.Name),
			},
		},
	})

	if err != nil {
		var existsErr *types.ParameterAlreadyExists
		if errors.As(err, &existsErr) {
			return nil
		}
		return fmt.Errorf("create application %s: %w", application.Name, err)
	}
	return nil
}

// UpdateApplication updates the data in SSM about an application.
func (s *Store) UpdateApplication(ctx context.Context, application *Application) error {
	applicationPath := fmt.Sprintf(fmtApplicationPath, application.Name)
	application.Version = schemaVersion

	data, err := marshal(application)
	if err != nil {
		return fmt.Errorf("serializing application %s: %w", application.Name, err)
	}

	if _, err = s.ssm.PutParameter(ctx, &ssm.PutParameterInput{
		Name:        aws.String(applicationPath),
		Description: aws.String("Copilot Application"),
		Type:        types.ParameterTypeString,
		Value:       aws.String(data),
		Overwrite:   aws.Bool(true),
	}); err != nil {
		return fmt.Errorf("update application %s: %w", application.Name, err)
	}
	return nil
}

// GetApplication fetches an application by name. If it can't be found, return a ErrNoSuchApplication
func (s *Store) GetApplication(ctx context.Context, applicationName string) (*Application, error) {
	applicationPath := fmt.Sprintf(fmtApplicationPath, applicationName)
	applicationParam, err := s.ssm.GetParameter(ctx, &ssm.GetParameterInput{
		Name: aws.String(applicationPath),
	})

	if err != nil {
		var notFoundErr *types.ParameterNotFound
		if errors.As(err, &notFoundErr) {
			account, region := s.getCallerAccountAndRegion(ctx)
			return nil, &ErrNoSuchApplication{
				ApplicationName: applicationName,
				AccountID:       account,
				Region:          region,
			}
		}
		return nil, fmt.Errorf("get application %s: %w", applicationName, err)
	}

	var application Application
	if err := json.Unmarshal([]byte(aws.ToString(applicationParam.Parameter.Value)), &application); err != nil {
		return nil, fmt.Errorf("read configuration for application %s: %w", applicationName, err)
	}
	return &application, nil
}

// ListApplications returns the list of existing applications in the customer's account and region.
func (s *Store) ListApplications(ctx context.Context) ([]*Application, error) {
	var applications []*Application
	serializedApplications, err := s.listParams(ctx, rootApplicationPath)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	for _, serializedApplication := range serializedApplications {
		var application Application
		if err := json.Unmarshal([]byte(*serializedApplication), &application); err != nil {
			return nil, fmt.Errorf("read application configuration: %w", err)
		}

		applications = append(applications, &application)
	}
	return applications, nil
}

// DeleteApplication deletes the SSM parameter related to the application.
func (s *Store) DeleteApplication(ctx context.Context, name string) error {
	paramName := fmt.Sprintf(fmtApplicationPath, name)

	_, err := s.ssm.DeleteParameter(ctx, &ssm.DeleteParameterInput{
		Name: aws.String(paramName),
	})

	if err != nil {
		var notFoundErr *types.ParameterNotFound
		if errors.As(err, &notFoundErr) {
			return nil
		}

		return err
	}

	return nil
}
