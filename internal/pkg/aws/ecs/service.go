// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package ecs provides a client to make API requests to Amazon Elastic Container Service.
package ecs

import (
	"fmt"
	"strings"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

const (
	// ServiceDeploymentStatusPrimary is the status PRIMARY of an ECS service deployment.
	ServiceDeploymentStatusPrimary = "PRIMARY"
	// ServiceDeploymentStatusActive is the status ACTIVE of an ECS service deployment.
	ServiceDeploymentStatusActive = "ACTIVE"
)

// Service wraps up ECS Service struct.
type Service types.Service

// Deployment contains information of a ECS service Deployment.
type Deployment struct {
	Id             string    `json:"id"`
	DesiredCount   int64     `json:"desiredCount"`
	RunningCount   int64     `json:"runningCount"`
	UpdatedAt      time.Time `json:"updatedAt"`
	LaunchType     string    `json:"launchType"`
	TaskDefinition string    `json:"taskDefinition"`
	Status         string    `json:"status"`
}

// ServiceStatus contains the status info of a service.
type ServiceStatus struct {
	DesiredCount     int64        `json:"desiredCount"`
	RunningCount     int64        `json:"runningCount"`
	Status           string       `json:"status"`
	Deployments      []Deployment `json:"deployments"`
	LastDeploymentAt time.Time    `json:"lastDeploymentAt"` // kept to avoid breaking change
	TaskDefinition   string       `json:"taskDefinition"`   // kept to avoid breaking change
}

// ServiceStatus returns the status of the running service.
func (s *Service) ServiceStatus() ServiceStatus {
	var deployments []Deployment
	for _, dp := range s.Deployments {
		deployments = append(deployments, Deployment{
			Id:             awsv2.ToString(dp.Id),
			DesiredCount:   int64(dp.DesiredCount),
			RunningCount:   int64(dp.RunningCount),
			UpdatedAt:      awsv2.ToTime(dp.UpdatedAt),
			LaunchType:     string(dp.LaunchType),
			TaskDefinition: awsv2.ToString(dp.TaskDefinition),
			Status:         awsv2.ToString(dp.Status),
		})
	}

	return ServiceStatus{
		Status:           awsv2.ToString(s.Status),
		DesiredCount:     int64(s.DesiredCount),
		RunningCount:     int64(s.RunningCount),
		Deployments:      deployments,
		LastDeploymentAt: awsv2.ToTime(s.Deployments[0].UpdatedAt), // FIXME Service assumed to have at least one deployment
		TaskDefinition:   awsv2.ToString(s.Deployments[0].TaskDefinition),
	}
}

// ServiceConnectAliases returns the ECS Service Connect client aliases for a service.
func (s *Service) ServiceConnectAliases() []string {
	if len(s.Deployments) == 0 {
		return nil
	}
	lastDeployment := s.Deployments[0]
	scConfig := lastDeployment.ServiceConnectConfiguration
	if scConfig == nil || !scConfig.Enabled {
		return nil
	}
	var aliases []string
	for _, service := range scConfig.Services {
		defaultName := awsv2.ToString(service.PortName)
		if awsv2.ToString(service.DiscoveryName) != "" {
			defaultName = awsv2.ToString(service.DiscoveryName)
		}
		defaultAlias := fmt.Sprintf("%s.%s", defaultName, awsv2.ToString(scConfig.Namespace))
		if len(service.ClientAliases) == 0 {
			aliases = append(aliases, defaultAlias)
			continue
		}
		for _, clientAlias := range service.ClientAliases {
			alias := defaultAlias
			if awsv2.ToString(clientAlias.DnsName) != "" {
				alias = awsv2.ToString(clientAlias.DnsName)
			}
			aliases = append(aliases, fmt.Sprintf("%s:%v", alias, awsv2.ToInt32(clientAlias.Port)))
		}
	}
	return aliases
}

// LastUpdatedAt returns the last updated time of the ECS service.
func (s *Service) LastUpdatedAt() time.Time {
	return awsv2.ToTime(s.Deployments[0].UpdatedAt)
}

// TargetGroups returns the ARNs of target groups attached to the service.
func (s *Service) TargetGroups() []string {
	var targetGroupARNs []string
	for _, lb := range s.LoadBalancers {
		targetGroupARNs = append(targetGroupARNs, awsv2.ToString(lb.TargetGroupArn))
	}
	return targetGroupARNs
}

// ServiceArn is the arn of an ECS service.
type ServiceArn struct {
	accountID   string
	partition   string
	region      string
	name        string
	clusterName string
}

// ParseServiceArn parses an arn into ServiceArn.
// For example: arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB
func ParseServiceArn(s string) (*ServiceArn, error) {
	parsedArn, err := arn.Parse(s)
	if err != nil {
		return nil, err
	}
	if parsedArn.Service != EndpointsID {
		return nil, fmt.Errorf("expected an ECS arn, but got %q", s)
	}
	resources := strings.Split(parsedArn.Resource, "/")
	if len(resources) != 3 {
		return nil, fmt.Errorf("cannot parse resource for ARN %q", s)
	}
	if resources[0] != "service" {
		return nil, fmt.Errorf("expect an ECS service: got %q", s)
	}
	return &ServiceArn{
		accountID:   parsedArn.AccountID,
		partition:   parsedArn.Partition,
		region:      parsedArn.Region,
		name:        resources[2],
		clusterName: resources[1],
	}, nil
}

func (s *ServiceArn) String() string {
	return fmt.Sprintf("arn:%s:%s:%s:%s:service/%s/%s", s.partition, EndpointsID, s.region, s.accountID, s.clusterName, s.name)
}

// ClusterArn returns the cluster arn.
// For example: arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB
// will return arn:aws:ecs:us-west-2:1234567890:cluster/my-project-test-Cluster-9F7Y0RLP60R7
func (s *ServiceArn) ClusterArn() string {
	return fmt.Sprintf("arn:%s:%s:%s:%s:cluster/%s", s.partition, EndpointsID, s.region, s.accountID, s.clusterName)
}

// ServiceName returns the service name.
// For example: arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB
// will return my-project-test-myService-JSOH5GYBFAIB
func (s *ServiceArn) ServiceName() string {
	return s.name
}

// ClusterName returns the cluster name.
// For example: arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB
// will return my-project-test-Cluster-9F7Y0RLP60R7
func (s *ServiceArn) ClusterName() string {
	return s.clusterName
}

// NetworkConfiguration holds service's NetworkConfiguration.
type NetworkConfiguration struct {
	AssignPublicIp string
	SecurityGroups []string
	Subnets        []string
}
