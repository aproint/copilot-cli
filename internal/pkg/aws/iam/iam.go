// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package iam provides a client to make API requests to the AWS Identity and Access Management service.
package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	ecsServiceName = "ecs.amazonaws.com"
)

type api interface {
	ListRoleTags(context.Context, *iam.ListRoleTagsInput, ...func(*iam.Options)) (*iam.ListRoleTagsOutput, error)
	DeleteRolePolicy(context.Context, *iam.DeleteRolePolicyInput, ...func(*iam.Options)) (*iam.DeleteRolePolicyOutput, error)
	ListRolePolicies(context.Context, *iam.ListRolePoliciesInput, ...func(*iam.Options)) (*iam.ListRolePoliciesOutput, error)
	DeleteRole(context.Context, *iam.DeleteRoleInput, ...func(*iam.Options)) (*iam.DeleteRoleOutput, error)
	CreateServiceLinkedRole(context.Context, *iam.CreateServiceLinkedRoleInput, ...func(*iam.Options)) (*iam.CreateServiceLinkedRoleOutput, error)
	ListPolicies(context.Context, *iam.ListPoliciesInput, ...func(*iam.Options)) (*iam.ListPoliciesOutput, error)
}

// IAM wraps the AWS SDK's IAM client.
type IAM struct {
	client api
}

// New returns an IAM client configured against the input SDK v2 config.
func New(cfg awsv2.Config) *IAM {
	return &IAM{
		client: iam.NewFromConfig(cfg),
	}
}

// ListRoleTags gathers all the tags associated with an IAM role.
func (c *IAM) ListRoleTags(roleName string) (map[string]string, error) {
	tags := make(map[string]string)
	var marker *string
	for {
		out, err := c.client.ListRoleTags(context.Background(), &iam.ListRoleTagsInput{
			RoleName: awsv2.String(roleName),
			Marker:   marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list role tags for role %s and marker %v: %w", roleName, marker, err)
		}
		for _, tag := range out.Tags {
			tags[awsv2.ToString(tag.Key)] = awsv2.ToString(tag.Value)
		}
		if !out.IsTruncated {
			return tags, nil
		}
		marker = out.Marker
	}
}

// DeleteRole deletes an IAM role based on its ARN.
// If the role does not exist it returns nil.
func (c *IAM) DeleteRole(roleNameOrARN string) error {
	roleName := roleNameOrARN
	if parsed, err := arn.Parse(roleNameOrARN); err == nil {
		// The parameter is an ARN instead!
		// Sample ARN format: arn:aws:iam::1111:role/phonetool-test-CFNExecutionRole
		roleName = strings.TrimPrefix(parsed.Resource, "role/")
	}

	if err := c.deleteRolePolicies(roleName); err != nil {
		return err
	}
	if _, err := c.client.DeleteRole(context.Background(), &iam.DeleteRoleInput{
		RoleName: awsv2.String(roleName),
	}); err != nil {
		if isNotExistErr(err) {
			// The role does not exist, exit successfully.
			return nil
		}
		return fmt.Errorf("delete role named %s: %w", roleName, err)
	}
	return nil
}

// CreateECSServiceLinkedRole creates a Service-Linked Role for Amazon ECS.
// This role is necessary so that Amazon ECS can call AWS APIs.
// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/using-service-linked-roles.html
func (c *IAM) CreateECSServiceLinkedRole() error {
	if _, err := c.client.CreateServiceLinkedRole(context.Background(), &iam.CreateServiceLinkedRoleInput{
		AWSServiceName: awsv2.String(ecsServiceName),
	}); err != nil {
		return fmt.Errorf("create service linked role for %s: %w", ecsServiceName, err)
	}
	return nil
}

// ListPolicyNames returns a list of local policy names.
func (c *IAM) ListPolicyNames() ([]string, error) {
	var policies []types.Policy
	var marker *string
	for {
		output, err := c.client.ListPolicies(context.Background(), &iam.ListPoliciesInput{
			Marker:            marker,
			Scope:             types.PolicyScopeTypeLocal,
			PolicyUsageFilter: types.PolicyUsageTypePermissionsBoundary,
		})
		if err != nil {
			return nil, fmt.Errorf("list IAM policies: %w", err)
		}
		policies = append(policies, output.Policies...)
		if !output.IsTruncated {
			break
		}
		marker = output.Marker
	}
	var policyNames = make([]string, len(policies))
	for i, policy := range policies {
		policyNames[i] = awsv2.ToString(policy.PolicyName)
	}
	return policyNames, nil
}

func (c *IAM) deleteRolePolicies(roleName string) error {
	policyNames, err := c.listRolePolicyNames(roleName)
	if err != nil {
		return err
	}
	for _, policyName := range policyNames {
		if _, err := c.client.DeleteRolePolicy(context.Background(), &iam.DeleteRolePolicyInput{
			PolicyName: awsv2.String(policyName),
			RoleName:   awsv2.String(roleName),
		}); err != nil {
			return fmt.Errorf("delete policy named %s in role %s: %w", policyName, roleName, err)
		}
	}
	return nil
}

func (c *IAM) listRolePolicyNames(roleName string) ([]string, error) {
	var policyNames []string
	var marker *string
	for {
		out, err := c.client.ListRolePolicies(context.Background(), &iam.ListRolePoliciesInput{
			Marker:   marker,
			RoleName: awsv2.String(roleName),
		})
		if err != nil {
			if isNotExistErr(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("list role policies for role %s: %v", roleName, err)
		}
		policyNames = append(policyNames, out.PolicyNames...)
		if !out.IsTruncated {
			return policyNames, nil
		}
		marker = out.Marker
	}
}

func isNotExistErr(err error) bool {
	var notFound *types.NoSuchEntityException
	return errors.As(err, &notFound)
}
