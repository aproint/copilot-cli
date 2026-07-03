// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package resourcegroups provides a client to make API requests to AWS Resource Groups
package resourcegroups

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
)

const (
	// ResourceTypeStateMachine is the resource type for the state machine of a job.
	ResourceTypeStateMachine = "states:stateMachine"
	// ResourceTypeRDS is the resource type for any rds resources.
	ResourceTypeRDS = "rds"
)

type api interface {
	GetResources(ctx context.Context, input *resourcegroupstaggingapi.GetResourcesInput, opts ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error)
}

// ResourceGroups wraps an AWS ResourceGroups client.
type ResourceGroups struct {
	client api
}

// Resource contains the ARN and the tags of the resource.
type Resource struct {
	ARN  string
	Tags map[string]string
}

// New returns a ResourceGroup struct configured against the input SDK v2 config.
func New(cfg awsv2.Config) *ResourceGroups {
	return &ResourceGroups{
		client: resourcegroupstaggingapi.NewFromConfig(cfg),
	}
}

// GetResourcesByTags gets tag set and ARN for the resource with input resource type and tags.
func (rg *ResourceGroups) GetResourcesByTags(resourceType string, tags map[string]string) ([]*Resource, error) {
	var resources []*Resource
	var tagFilter []types.TagFilter
	for k, v := range tags {
		var values []string
		if v != "" {
			values = []string{v}
		}
		tagFilter = append(tagFilter, types.TagFilter{
			Key:    awsv2.String(k),
			Values: values,
		})
	}
	resourceResp := &resourcegroupstaggingapi.GetResourcesOutput{}
	for {
		var err error
		resourceResp, err = rg.client.GetResources(context.Background(), &resourcegroupstaggingapi.GetResourcesInput{
			PaginationToken:     resourceResp.PaginationToken,
			ResourceTypeFilters: []string{resourceType},
			TagFilters:          tagFilter,
		})
		if err != nil {
			return nil, fmt.Errorf("get resource: %w", err)
		}
		for _, resourceTagMapping := range resourceResp.ResourceTagMappingList {
			tags := make(map[string]string)
			for _, tag := range resourceTagMapping.Tags {
				if tag.Key == nil {
					continue
				}
				tags[*tag.Key] = awsv2.ToString(tag.Value)
			}
			resources = append(resources, &Resource{
				ARN:  awsv2.ToString(resourceTagMapping.ResourceARN),
				Tags: tags,
			})
		}
		// usually pagination token is "" when it doesn't have any next page. However, since it
		// is type *string, it is safer for us to check nil value for it as well.
		if token := resourceResp.PaginationToken; awsv2.ToString(token) == "" {
			break
		}
	}

	return resources, nil
}
