// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ecs

import (
	"fmt"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
)

func TestService_TargetGroups(t *testing.T) {
	t.Run("should return correct ARNs", func(t *testing.T) {
		s := Service{
			LoadBalancers: []types.LoadBalancer{
				{
					TargetGroupArn: awsv2.String("group-1"),
				},
				{
					TargetGroupArn: awsv2.String("group-2"),
				},
			},
		}
		got := s.TargetGroups()
		expected := []string{"group-1", "group-2"}
		require.Equal(t, expected, got)
	})
}

func TestService_ServiceStatus(t *testing.T) {
	t.Run("should include active and primary deployments in status", func(t *testing.T) {
		inService := Service{
			Deployments: []types.Deployment{
				{
					Status:       awsv2.String("ACTIVE"),
					Id:           awsv2.String("id-1"),
					DesiredCount: 3,
					RunningCount: 3,
				},
				{
					Status:       awsv2.String("ACTIVE"),
					Id:           awsv2.String("id-3"),
					DesiredCount: 4,
					RunningCount: 2,
				},
				{
					Status:       awsv2.String("PRIMARY"),
					Id:           awsv2.String("id-4"),
					DesiredCount: 10,
					RunningCount: 1,
				},
				{
					Status: awsv2.String("INACTIVE"),
					Id:     awsv2.String("id-5"),
				},
			},
		}
		wanted := ServiceStatus{
			Status:       "",
			DesiredCount: 0,
			RunningCount: 0,
			Deployments: []Deployment{
				{
					Id:           "id-1",
					DesiredCount: 3,
					RunningCount: 3,
					Status:       "ACTIVE",
				},
				{
					Id:           "id-3",
					DesiredCount: 4,
					RunningCount: 2,
					Status:       "ACTIVE",
				},
				{
					Id:           "id-4",
					DesiredCount: 10,
					RunningCount: 1,
					Status:       "PRIMARY",
				},
				{
					Id:     "id-5",
					Status: "INACTIVE",
				},
			},
			LastDeploymentAt: time.Time{},
			TaskDefinition:   "",
		}
		got := inService.ServiceStatus()
		require.Equal(t, got, wanted)
	})
}

func TestService_LastUpdatedAt(t *testing.T) {
	mockTime1 := time.Unix(14945056, 0)
	mockTime2 := time.Unix(14945059, 0)
	t.Run("should return correct last updated value", func(t *testing.T) {
		s := Service{
			Deployments: []types.Deployment{
				{
					UpdatedAt: &mockTime1,
				},
				{
					UpdatedAt: &mockTime2,
				},
			},
		}
		got := s.LastUpdatedAt()
		require.Equal(t, mockTime1, got)
	})
}

func TestService_ServiceConnectAliases(t *testing.T) {
	tests := map[string]struct {
		inService *Service

		wantedError error
		wanted      []string
	}{
		"quit early if not enabled": {
			inService: &Service{
				Deployments: []types.Deployment{
					{
						ServiceConnectConfiguration: &types.ServiceConnectConfiguration{
							Enabled: false,
						},
					},
				},
			},
			wanted: []string{},
		},
		"success": {
			inService: &Service{
				Deployments: []types.Deployment{
					{
						ServiceConnectConfiguration: &types.ServiceConnectConfiguration{
							Enabled:   true,
							Namespace: awsv2.String("foobar.local"),
							Services: []types.ServiceConnectService{
								{
									PortName:      awsv2.String("frontend"),
									DiscoveryName: awsv2.String("front"),
								},
								{
									PortName: awsv2.String("frontend"),
								},
								{
									PortName: awsv2.String("frontend"),
									ClientAliases: []types.ServiceConnectClientAlias{
										{
											Port: awsv2.Int32(5000),
										},
										{
											DnsName: awsv2.String("api"),
											Port:    awsv2.Int32(80),
										},
									},
								},
							},
						},
					},
				},
			},
			wanted: []string{"front.foobar.local", "frontend.foobar.local", "frontend.foobar.local:5000", "api:80"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// WHEN
			get := tc.inService.ServiceConnectAliases()

			// THEN
			require.ElementsMatch(t, get, tc.wanted)
		})
	}
}

func TestParseServiceArn(t *testing.T) {
	tests := map[string]struct {
		inArnStr string

		wantedError error
		wantedArn   ServiceArn
	}{
		"error if invalid arn": {
			inArnStr:    "random string",
			wantedError: fmt.Errorf("arn: invalid prefix"),
		},
		"error if non ecs arn": {
			inArnStr:    "arn:aws:acm:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB",
			wantedError: fmt.Errorf(`expected an ECS arn, but got "arn:aws:acm:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB"`),
		},
		"error if invalid resource": {
			inArnStr:    "arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7",
			wantedError: fmt.Errorf(`cannot parse resource for ARN "arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7"`),
		},
		"error if invalid resource type": {
			inArnStr:    "arn:aws:ecs:us-west-2:1234567890:task/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB",
			wantedError: fmt.Errorf(`expect an ECS service: got "arn:aws:ecs:us-west-2:1234567890:task/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB"`),
		},
		"success": {
			inArnStr: "arn:aws:ecs:us-west-2:1234567890:service/my-project-test-Cluster-9F7Y0RLP60R7/my-project-test-myService-JSOH5GYBFAIB",
			wantedArn: ServiceArn{
				accountID:   "1234567890",
				partition:   "aws",
				region:      "us-west-2",
				name:        "my-project-test-myService-JSOH5GYBFAIB",
				clusterName: "my-project-test-Cluster-9F7Y0RLP60R7",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// WHEN
			arn, err := ParseServiceArn(tc.inArnStr)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedArn, *arn)
			}
		})
	}
}
