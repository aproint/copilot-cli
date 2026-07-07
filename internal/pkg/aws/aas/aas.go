// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package aas provides a client to make API requests to Application Auto Scaling.
package aas

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	aas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
)

const (
	// ECS service resource ID format: service/${clusterName}/${serviceName}.
	fmtECSResourceID    = "service/%s/%s"
	ecsServiceNamespace = "ecs"
)

type api interface {
	DescribeScalingPolicies(ctx context.Context, input *aas.DescribeScalingPoliciesInput, opts ...func(*aas.Options)) (*aas.DescribeScalingPoliciesOutput, error)
}

// ApplicationAutoscaling wraps an Amazon Application Auto Scaling client.
type ApplicationAutoscaling struct {
	client api
}

// New returns a ApplicationAutoscaling struct configured against the input SDK v2 config.
func New(cfg awsv2.Config) *ApplicationAutoscaling {
	return &ApplicationAutoscaling{
		client: aas.NewFromConfig(cfg),
	}
}

// ECSServiceAlarmNames returns names of the CloudWatch alarms associated with the
// scaling policies attached to the ECS service.
func (a *ApplicationAutoscaling) ECSServiceAlarmNames(cluster, service string) ([]string, error) {
	resourceID := fmt.Sprintf(fmtECSResourceID, cluster, service)
	var alarms []string
	var err error
	resp := &aas.DescribeScalingPoliciesOutput{}
	for {
		resp, err = a.client.DescribeScalingPolicies(context.Background(), &aas.DescribeScalingPoliciesInput{
			ResourceId:       awsv2.String(resourceID),
			ServiceNamespace: types.ServiceNamespaceEcs,
			NextToken:        resp.NextToken,
		})
		if err != nil {
			return nil, fmt.Errorf("describe scaling policies for ECS service %s/%s: %w", cluster, service, err)
		}
		for _, policy := range resp.ScalingPolicies {
			for _, alarm := range policy.Alarms {
				alarms = append(alarms, awsv2.ToString(alarm.AlarmName))
			}
		}
		if resp.NextToken == nil {
			break
		}
	}
	return alarms, nil
}
