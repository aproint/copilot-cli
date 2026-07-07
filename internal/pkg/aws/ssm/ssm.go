// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ssm provides a client to make API requests to Amazon Systems Manager.
package ssm

import (
	"context"
	"errors"
	"fmt"
	"sort"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type api interface {
	PutParameter(context.Context, *ssm.PutParameterInput, ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	AddTagsToResource(context.Context, *ssm.AddTagsToResourceInput, ...func(*ssm.Options)) (*ssm.AddTagsToResourceOutput, error)
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// SSM wraps an AWS SSM client.
type SSM struct {
	client api
}

// New returns a SSM service configured against the input SDK v2 config.
func New(cfg awsv2.Config) *SSM {
	return &SSM{
		client: ssm.NewFromConfig(cfg),
	}
}

// PutSecretInput contains fields needed to create or update a secret.
type PutSecretInput struct {
	Name      string
	Value     string
	Overwrite bool
	Tags      map[string]string
}

// PutSecretOutput wraps an ssm PutParameterOutput struct.
type PutSecretOutput ssm.PutParameterOutput

// PutSecret tries to create the secret, and overwrites it if the secret exists and that `Overwrite` is true.
// ErrParameterAlreadyExists is returned if the secret exists and `Overwrite` is false.
func (s *SSM) PutSecret(in PutSecretInput) (*PutSecretOutput, error) {
	// First try to create the secret with the tags.
	out, err := s.createSecret(in)
	if err == nil {
		return out, nil
	}

	// If the parameter already exists and we want to overwrite, we try to overwrite it.
	var errParameterExists *ErrParameterAlreadyExists
	if errors.As(err, &errParameterExists) && in.Overwrite {
		return s.overwriteSecret(in)
	}
	return nil, err
}

// GetSecretValue retrieves the value of a parameter from AWS Systems Manager Parameter Store.
// It takes the name of the parameter as input and returns the corresponding value as a string.
func (s *SSM) GetSecretValue(ctx context.Context, name string) (string, error) {
	resp, err := s.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           awsv2.String(name),
		WithDecryption: awsv2.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("get parameter %q from SSM: %w", name, err)
	}
	return awsv2.ToString(resp.Parameter.Value), nil
}

func (s *SSM) createSecret(in PutSecretInput) (*PutSecretOutput, error) {
	// Create a secret while adding the tags in a single call instead of separate calls to `PutParameter` and
	// `AddTagsToResource` so that there won't be a case where the parameter is created while the tags are not added.

	tags := convertTags(in.Tags)

	input := &ssm.PutParameterInput{
		DataType: awsv2.String("text"),
		Type:     types.ParameterTypeSecureString,
		Name:     awsv2.String(in.Name),
		Value:    awsv2.String(in.Value),
		Tags:     tags,
	}
	output, err := s.client.PutParameter(context.Background(), input)
	if err == nil {
		return (*PutSecretOutput)(output), nil
	}

	var errAlreadyExists *types.ParameterAlreadyExists
	if errors.As(err, &errAlreadyExists) {
		return nil, &ErrParameterAlreadyExists{in.Name}
	}
	return nil, fmt.Errorf("create parameter %s: %w", in.Name, err)
}

func (s *SSM) overwriteSecret(in PutSecretInput) (*PutSecretOutput, error) {
	// SSM API does not allow `Overwrite` to be true while `Tags` are not nil, so we have to overwrite the resource and
	// add the tags in two separate calls.

	input := &ssm.PutParameterInput{
		DataType:  awsv2.String("text"),
		Type:      types.ParameterTypeSecureString,
		Name:      awsv2.String(in.Name),
		Value:     awsv2.String(in.Value),
		Overwrite: awsv2.Bool(in.Overwrite),
	}
	output, err := s.client.PutParameter(context.Background(), input)
	if err != nil {
		return nil, fmt.Errorf("update parameter %s: %w", in.Name, err)
	}

	tags := convertTags(in.Tags)
	_, err = s.client.AddTagsToResource(context.Background(), &ssm.AddTagsToResourceInput{
		ResourceType: types.ResourceTypeForTaggingParameter,
		ResourceId:   awsv2.String(in.Name),
		Tags:         tags,
	})
	if err != nil {
		return nil, fmt.Errorf("add tags to resource %s: %w", in.Name, err)
	}
	return (*PutSecretOutput)(output), nil
}

func convertTags(inTags map[string]string) []types.Tag {
	// Sort the map so that the unit test won't be flaky.
	keys := make([]string, 0, len(inTags))
	for k := range inTags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var tags []types.Tag
	for _, key := range keys {
		tags = append(tags, types.Tag{
			Key:   awsv2.String(key),
			Value: awsv2.String(inTags[key]),
		})
	}
	return tags
}
