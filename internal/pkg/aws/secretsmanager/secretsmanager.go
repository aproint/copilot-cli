// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package secretsmanager provides a client to make API requests to AWS Secrets Manager.
package secretsmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

type api interface {
	CreateSecret(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	DescribeSecret(context.Context, *secretsmanager.DescribeSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// SecretsManager wraps the AWS SecretManager client.
type SecretsManager struct {
	secretsManager api
	sessionRegion  string
}

// New returns a SecretsManager configured against the input SDK v2 config.
func New(cfg awsv2.Config) *SecretsManager {
	return &SecretsManager{
		secretsManager: secretsmanager.NewFromConfig(cfg),
		sessionRegion:  cfg.Region,
	}
}

var secretTags = func() []types.Tag {
	timestamp := time.Now().UTC().Format(time.UnixDate)
	return []types.Tag{
		{
			Key:   awsv2.String("copilot-application"),
			Value: awsv2.String(timestamp),
		},
	}
}

// CreateSecret creates a secret using the default KMS key "aws/secretmanager" to encrypt the secret and returns its ARN.
func (s *SecretsManager) CreateSecret(secretName, secretString string) (string, error) {
	resp, err := s.secretsManager.CreateSecret(context.Background(), &secretsmanager.CreateSecretInput{
		Name:         awsv2.String(secretName),
		SecretString: awsv2.String(secretString),
		Tags:         secretTags(),
	})

	if err != nil {
		var errAlreadyExists *types.ResourceExistsException
		if errors.As(err, &errAlreadyExists) {
			// TODO update secret if value provided?
			return "", &ErrSecretAlreadyExists{
				secretName: secretName,
				parentErr:  err,
			}
		}
		return "", fmt.Errorf("create secret %s: %w", secretName, err)

	}

	return awsv2.ToString(resp.ARN), nil
}

// DeleteSecret force removes the secret from SecretsManager.
func (s *SecretsManager) DeleteSecret(secretName string) error {
	_, err := s.secretsManager.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
		SecretId:                   awsv2.String(secretName),
		ForceDeleteWithoutRecovery: awsv2.Bool(true), // forego the waiting period to delete the secret
	})

	if err != nil {
		return fmt.Errorf("delete secret %s from secrets manager: %w", secretName, err)
	}
	return nil
}

// DescribeSecretOutput is the output returned by DescribeSecret.
type DescribeSecretOutput struct {
	Name        *string
	CreatedDate *time.Time
	Tags        []types.Tag
}

// DescribeSecret retrieves the details of a secret.
func (s *SecretsManager) DescribeSecret(secretName string) (*DescribeSecretOutput, error) {
	resp, err := s.secretsManager.DescribeSecret(context.Background(), &secretsmanager.DescribeSecretInput{
		SecretId: awsv2.String(secretName),
	})
	if err != nil {
		var errNotFound *types.ResourceNotFoundException
		if errors.As(err, &errNotFound) {
			return nil, &ErrSecretNotFound{
				secretName: secretName,
				parentErr:  err,
			}
		}
		return nil, fmt.Errorf("describe secret %s: %w", secretName, err)
	}

	return &DescribeSecretOutput{
		Name:        resp.Name,
		CreatedDate: resp.CreatedDate,
		Tags:        resp.Tags,
	}, nil
}

// GetSecretValue retrieves the value of a secret from AWS Secrets Manager.
// It takes the name of the secret as input and returns the corresponding value as a string.
func (s *SecretsManager) GetSecretValue(ctx context.Context, name string) (string, error) {
	resp, err := s.secretsManager.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: awsv2.String(name),
	})
	if err != nil {
		return "", fmt.Errorf("get secret %q from secrets manager: %w", name, err)
	}
	return awsv2.ToString(resp.SecretString), nil
}

// ErrSecretAlreadyExists occurs if a secret with the same name already exists.
type ErrSecretAlreadyExists struct {
	secretName string
	parentErr  error
}

func (err *ErrSecretAlreadyExists) Error() string {
	return fmt.Sprintf("secret %s already exists", err.secretName)
}

// ErrSecretNotFound occurs if a secret with the given name does not exist.
type ErrSecretNotFound struct {
	secretName string
	parentErr  error
}

func (err *ErrSecretNotFound) Error() string {
	return fmt.Sprintf("secret %s was not found: %s", err.secretName, err.parentErr)
}
