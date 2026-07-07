// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ecs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/ecs/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestECS_TaskDefinition(t *testing.T) {
	mockError := errors.New("error")

	testCases := map[string]struct {
		taskDefinitionName string
		mockECSClient      func(m *mocks.Mockapi)

		wantErr     error
		wantTaskDef *TaskDefinition
	}{
		"should return wrapped error given error": {
			taskDefinitionName: "task-def",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeTaskDefinition(gomock.Any(), &ecs.DescribeTaskDefinitionInput{
					TaskDefinition: awsv2.String("task-def"),
				}).Return(nil, mockError)
			},
			wantErr: fmt.Errorf("describe task definition %s: %w", "task-def", mockError),
		},
		"returns task definition given a task definition name": {
			taskDefinitionName: "task-def",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeTaskDefinition(gomock.Any(), &ecs.DescribeTaskDefinitionInput{
					TaskDefinition: awsv2.String("task-def"),
				}).Return(&ecs.DescribeTaskDefinitionOutput{
					TaskDefinition: &types.TaskDefinition{
						ContainerDefinitions: []types.ContainerDefinition{
							{
								Environment: []types.KeyValuePair{
									{
										Name:  awsv2.String("COPILOT_SERVICE_NAME"),
										Value: awsv2.String("my-app"),
									},
									{
										Name:  awsv2.String("COPILOT_ENVIRONMENT_NAME"),
										Value: awsv2.String("prod"),
									},
								},
							},
						},
					},
				}, nil)
			},
			wantTaskDef: &TaskDefinition{
				ContainerDefinitions: []types.ContainerDefinition{
					{
						Environment: []types.KeyValuePair{
							{
								Name:  awsv2.String("COPILOT_SERVICE_NAME"),
								Value: awsv2.String("my-app"),
							},
							{
								Name:  awsv2.String("COPILOT_ENVIRONMENT_NAME"),
								Value: awsv2.String("prod"),
							},
						},
					},
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotTaskDef, gotErr := service.TaskDefinition(tc.taskDefinitionName)

			if gotErr != nil {
				require.Equal(t, tc.wantErr, gotErr)
			} else {
				require.Equal(t, tc.wantTaskDef, gotTaskDef)
			}
		})

	}
}

func TestECS_Service(t *testing.T) {
	testCases := map[string]struct {
		clusterName   string
		serviceName   string
		mockECSClient func(m *mocks.Mockapi)

		wantErr error
		wantSvc *Service
	}{
		"success": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"mockService"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("mockService"),
						},
					},
				}, nil)
			},
			wantSvc: &Service{
				ServiceName: awsv2.String("mockService"),
			},
		},
		"errors if failed to describe service": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"mockService"},
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("describe services: some error"),
		},
		"errors if failed to find the service": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"mockService"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("badMockService"),
						},
					},
				}, nil)
			},
			wantErr: fmt.Errorf("cannot find service mockService"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotSvc, gotErr := service.Service(tc.clusterName, tc.serviceName)

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.Equal(t, tc.wantSvc, gotSvc)
				require.NoError(t, tc.wantErr)
			}
		})
	}
}

func TestECS_Services(t *testing.T) {
	testCases := map[string]struct {
		clusterName   string
		services      []string
		mockECSClient func(m *mocks.Mockapi)

		wantErr  string
		wantSvcs []*Service
	}{
		"error if api call error": {
			clusterName: "mockCluster",
			services:    []string{"1"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"1"},
				}).Return(nil, errors.New("some error"))
			},
			wantErr: "describe services: some error",
		},
		"error if api returns failure": {
			clusterName: "mockCluster",
			services:    []string{"1"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"1"},
				}).Return(&ecs.DescribeServicesOutput{
					Failures: []types.Failure{
						{
							Arn:    awsv2.String("arn:1"),
							Reason: awsv2.String("some error"),
						},
					},
				}, nil)
			},
			wantErr: `describe services: {
  Arn: "arn:1",
  Reason: "some error"
}`,
		},
		"error if api returns incorrect count": {
			clusterName: "mockCluster",
			services:    []string{"1", "2"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"1", "2"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("1"),
						},
					},
				}, nil)
			},
			wantErr: "describe services: got 1 services, but expected 2",
		},
		"success with > 10": {
			clusterName: "mockCluster",
			services: []string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
				"11",
			},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("1"),
						},
						{
							ServiceName: awsv2.String("2"),
						},
						{
							ServiceName: awsv2.String("3"),
						},
						{
							ServiceName: awsv2.String("4"),
						},
						{
							ServiceName: awsv2.String("5"),
						},
						{
							ServiceName: awsv2.String("6"),
						},
						{
							ServiceName: awsv2.String("7"),
						},
						{
							ServiceName: awsv2.String("8"),
						},
						{
							ServiceName: awsv2.String("9"),
						},
						{
							ServiceName: awsv2.String("10"),
						},
					},
				}, nil)
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("mockCluster"),
					Services: []string{"11"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("11"),
						},
					},
				}, nil)
			},
			wantSvcs: []*Service{
				{
					ServiceName: awsv2.String("1"),
				},
				{
					ServiceName: awsv2.String("2"),
				},
				{
					ServiceName: awsv2.String("3"),
				},
				{
					ServiceName: awsv2.String("4"),
				},
				{
					ServiceName: awsv2.String("5"),
				},
				{
					ServiceName: awsv2.String("6"),
				},
				{
					ServiceName: awsv2.String("7"),
				},
				{
					ServiceName: awsv2.String("8"),
				},
				{
					ServiceName: awsv2.String("9"),
				},
				{
					ServiceName: awsv2.String("10"),
				},
				{
					ServiceName: awsv2.String("11"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotSvcs, gotErr := service.Services(tc.clusterName, tc.services...)

			if tc.wantErr != "" {
				require.EqualError(t, gotErr, tc.wantErr)
			} else {
				require.Equal(t, tc.wantSvcs, gotSvcs)
				require.NoError(t, gotErr)
			}
		})
	}
}

func TestECS_ListServicesByNamespace(t *testing.T) {
	testCases := map[string]struct {
		namespace     string
		mockECSClient func(m *mocks.Mockapi)

		wantErr  string
		wantARNs []string
	}{
		"error if api call error": {
			namespace: "mockNamespace",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListServicesByNamespace(gomock.Any(), &ecs.ListServicesByNamespaceInput{
					Namespace: awsv2.String("mockNamespace"),
				}).Return(nil, errors.New("some error"))
			},
			wantErr: "some error",
		},
		"success": {
			namespace: "mockNamespace",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListServicesByNamespace(gomock.Any(), &ecs.ListServicesByNamespaceInput{
					Namespace: awsv2.String("mockNamespace"),
				}).Return(&ecs.ListServicesByNamespaceOutput{
					ServiceArns: []string{"svc1", "svc2", "svc3"},
				}, nil)
			},
			wantARNs: []string{"svc1", "svc2", "svc3"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotARNs, gotErr := service.ListServicesByNamespace(tc.namespace)

			if tc.wantErr != "" {
				require.EqualError(t, gotErr, tc.wantErr)
			} else {
				require.Equal(t, tc.wantARNs, gotARNs)
				require.NoError(t, gotErr)
			}
		})
	}
}

func TestECS_UpdateService(t *testing.T) {
	const (
		clusterName = "mockCluster"
		serviceName = "mockService"
	)
	testCases := map[string]struct {
		forceUpdate   bool
		maxTryNum     int
		mockECSClient func(m *mocks.Mockapi)

		wantErr error
		wantSvc *Service
	}{
		"errors if failed to update service": {

			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().UpdateService(gomock.Any(), &ecs.UpdateServiceInput{
					Cluster: awsv2.String(clusterName),
					Service: awsv2.String(serviceName),
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("update service mockService from cluster mockCluster: some error"),
		},
		"errors if max retries exceeded": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().UpdateService(gomock.Any(), &ecs.UpdateServiceInput{
					Cluster: awsv2.String(clusterName),
					Service: awsv2.String(serviceName),
				}).Return(&ecs.UpdateServiceOutput{
					Service: &types.Service{
						Deployments:  []types.Deployment{{}, {}},
						DesiredCount: 1,
						RunningCount: 2,
						ClusterArn:   awsv2.String(clusterName),
						ServiceName:  awsv2.String(serviceName),
					},
				}, nil)
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String(clusterName),
					Services: []string{serviceName},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							Deployments:  []types.Deployment{{}},
							DesiredCount: 1,
							RunningCount: 2,
							ClusterArn:   awsv2.String(clusterName),
							ServiceName:  awsv2.String(serviceName),
						},
					},
				}, nil).Times(2)
			},
			wantErr: fmt.Errorf("wait until service mockService becomes stable: max retries 2 exceeded"),
		},
		"errors if failed to describe service": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().UpdateService(gomock.Any(), &ecs.UpdateServiceInput{
					Cluster: awsv2.String(clusterName),
					Service: awsv2.String(serviceName),
				}).Return(&ecs.UpdateServiceOutput{
					Service: &types.Service{
						Deployments:  []types.Deployment{{}, {}},
						DesiredCount: 1,
						RunningCount: 2,
						ClusterArn:   awsv2.String(clusterName),
						ServiceName:  awsv2.String(serviceName),
					},
				}, nil)
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String(clusterName),
					Services: []string{serviceName},
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("wait until service mockService becomes stable: describe services: some error"),
		},
		"success": {
			forceUpdate: true,
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().UpdateService(gomock.Any(), &ecs.UpdateServiceInput{
					Cluster:            awsv2.String(clusterName),
					Service:            awsv2.String(serviceName),
					ForceNewDeployment: true,
				}).Return(&ecs.UpdateServiceOutput{
					Service: &types.Service{
						Deployments:  []types.Deployment{{}, {}},
						DesiredCount: 1,
						RunningCount: 2,
						ClusterArn:   awsv2.String(clusterName),
						ServiceName:  awsv2.String(serviceName),
					},
				}, nil)
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String(clusterName),
					Services: []string{serviceName},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							Deployments:  []types.Deployment{{}},
							DesiredCount: 1,
							RunningCount: 2,
							ClusterArn:   awsv2.String(clusterName),
							ServiceName:  awsv2.String(serviceName),
						},
					},
				}, nil)
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String(clusterName),
					Services: []string{serviceName},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							Deployments:  []types.Deployment{{}},
							DesiredCount: 1,
							RunningCount: 1,
							ClusterArn:   awsv2.String(clusterName),
							ServiceName:  awsv2.String(serviceName),
						},
					},
				}, nil)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client:                mockECSClient,
				maxServiceStableTries: 2,
				pollIntervalDuration:  0,
			}
			var opts []UpdateServiceOpts
			if tc.forceUpdate {
				opts = append(opts, WithForceUpdate())
			}

			gotErr := service.UpdateService(clusterName, serviceName, opts...)

			if tc.wantErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.NoError(t, gotErr)
			}
		})

	}
}

func TestECS_Tasks(t *testing.T) {
	testCases := map[string]struct {
		clusterName   string
		serviceName   string
		mockECSClient func(m *mocks.Mockapi)

		wantErr   error
		wantTasks []*Task
	}{
		"errors if failed to list running tasks": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusRunning,
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("list running tasks: some error"),
		},
		"errors if failed to describe running tasks": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusRunning,
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("describe running tasks in cluster mockCluster: some error"),
		},
		"success": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusRunning,
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn"),
						},
					},
				}, nil)
			},
			wantTasks: []*Task{
				{
					TaskArn: awsv2.String("mockTaskArn"),
				},
			},
		},
		"success with pagination": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusRunning,
				}).Return(&ecs.ListTasksOutput{
					NextToken: awsv2.String("mockNextToken"),
					TaskArns:  []string{"mockTaskArn1"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn1"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn1"),
						},
					},
				}, nil)
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusRunning,
					NextToken:     awsv2.String("mockNextToken"),
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn2"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn2"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn2"),
						},
					},
				}, nil)
			},
			wantTasks: []*Task{
				{
					TaskArn: awsv2.String("mockTaskArn1"),
				},
				{
					TaskArn: awsv2.String("mockTaskArn2"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotTasks, gotErr := service.ServiceRunningTasks(tc.clusterName, tc.serviceName)

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.Equal(t, tc.wantTasks, gotTasks)
			}
		})

	}
}

func TestECS_StoppedServiceTasks(t *testing.T) {
	testCases := map[string]struct {
		clusterName   string
		serviceName   string
		mockECSClient func(m *mocks.Mockapi)

		wantErr   error
		wantTasks []*Task
	}{
		"errors if failed to list stopped tasks": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusStopped,
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("list running tasks: some error"),
		},
		"errors if failed to describe stopped tasks": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusStopped,
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(nil, errors.New("some error"))
			},
			wantErr: fmt.Errorf("describe running tasks in cluster mockCluster: some error"),
		},
		"success": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusStopped,
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn"),
						},
					},
				}, nil)
			},
			wantTasks: []*Task{
				{
					TaskArn: awsv2.String("mockTaskArn"),
				},
			},
		},
		"success with pagination": {
			clusterName: "mockCluster",
			serviceName: "mockService",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusStopped,
				}).Return(&ecs.ListTasksOutput{
					NextToken: awsv2.String("mockNextToken"),
					TaskArns:  []string{"mockTaskArn1"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn1"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn1"),
						},
					},
				}, nil)
				m.EXPECT().ListTasks(gomock.Any(), &ecs.ListTasksInput{
					Cluster:       awsv2.String("mockCluster"),
					ServiceName:   awsv2.String("mockService"),
					DesiredStatus: types.DesiredStatusStopped,
					NextToken:     awsv2.String("mockNextToken"),
				}).Return(&ecs.ListTasksOutput{
					NextToken: nil,
					TaskArns:  []string{"mockTaskArn2"},
				}, nil)
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String("mockCluster"),
					Tasks:   []string{"mockTaskArn2"},
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("mockTaskArn2"),
						},
					},
				}, nil)
			},
			wantTasks: []*Task{
				{
					TaskArn: awsv2.String("mockTaskArn1"),
				},
				{
					TaskArn: awsv2.String("mockTaskArn2"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}

			gotTasks, gotErr := service.StoppedServiceTasks(tc.clusterName, tc.serviceName)

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.Equal(t, tc.wantTasks, gotTasks)
			}
		})

	}
}

func TestECS_StopTasks(t *testing.T) {
	mockTasks := []string{"mockTask1", "mockTask2"}
	mockError := errors.New("some error")
	testCases := map[string]struct {
		cluster         string
		stopTasksReason string
		tasks           []string
		mockECSClient   func(m *mocks.Mockapi)

		wantErr error
	}{
		"errors if failed to stop tasks in default cluster": {
			tasks: mockTasks,
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().StopTask(gomock.Any(), &ecs.StopTaskInput{
					Task: awsv2.String("mockTask1"),
				}).Return(&ecs.StopTaskOutput{}, nil)
				m.EXPECT().StopTask(gomock.Any(), &ecs.StopTaskInput{
					Task: awsv2.String("mockTask2"),
				}).Return(&ecs.StopTaskOutput{}, mockError)
			},
			wantErr: fmt.Errorf("stop task mockTask2: some error"),
		},
		"success": {
			tasks:           mockTasks,
			cluster:         "mockCluster",
			stopTasksReason: "some reason",
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().StopTask(gomock.Any(), &ecs.StopTaskInput{
					Cluster: awsv2.String("mockCluster"),
					Reason:  awsv2.String("some reason"),
					Task:    awsv2.String("mockTask1"),
				}).Return(&ecs.StopTaskOutput{}, nil)
				m.EXPECT().StopTask(gomock.Any(), &ecs.StopTaskInput{
					Cluster: awsv2.String("mockCluster"),
					Reason:  awsv2.String("some reason"),
					Task:    awsv2.String("mockTask2"),
				}).Return(&ecs.StopTaskOutput{}, nil)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			service := ECS{
				client: mockECSClient,
			}
			var opts []StopTasksOpts
			if tc.cluster != "" {
				opts = append(opts, WithStopTaskCluster(tc.cluster))
			}
			if tc.stopTasksReason != "" {
				opts = append(opts, WithStopTaskReason(tc.stopTasksReason))
			}
			gotErr := service.StopTasks(tc.tasks, opts...)

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.NoError(t, tc.wantErr)
			}
		})

	}
}

func TestECS_DefaultCluster(t *testing.T) {
	testCases := map[string]struct {
		mockECSClient func(m *mocks.Mockapi)

		wantedError    error
		wantedClusters string
	}{
		"get default clusters success": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(&ecs.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterArn:  awsv2.String("arn:aws:ecs:us-east-1:0123456:cluster/cluster1"),
								ClusterName: awsv2.String("cluster1"),
								Status:      awsv2.String(statusActive),
							},
							{
								ClusterArn:  awsv2.String("arn:aws:ecs:us-east-1:0123456:cluster/cluster2"),
								ClusterName: awsv2.String("cluster2"),
								Status:      awsv2.String(statusActive),
							},
						},
					}, nil)
			},

			wantedClusters: "arn:aws:ecs:us-east-1:0123456:cluster/cluster1",
		},
		"ignore inactive cluster": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(&ecs.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterArn:  awsv2.String("arn:aws:ecs:us-east-1:0123456:cluster/cluster1"),
								ClusterName: awsv2.String("cluster1"),
								Status:      awsv2.String("INACTIVE"),
							},
						},
					}, nil)
			},
			wantedError: fmt.Errorf("default cluster does not exist"),
		},
		"failed to get default clusters": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(nil, errors.New("error"))
			},
			wantedError: fmt.Errorf("get default cluster: %s", "error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			ecs := ECS{
				client: mockECSClient,
			}
			clusters, err := ecs.DefaultCluster()
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedClusters, clusters)
			}
		})
	}
}

func TestECS_HasDefaultCluster(t *testing.T) {
	testCases := map[string]struct {
		mockECSClient func(m *mocks.Mockapi)

		wantedHasDefaultCluster bool
		wantedErr               error
	}{
		"no default cluster": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(&ecs.DescribeClustersOutput{
						Clusters: []types.Cluster{},
					}, nil)
			},
			wantedHasDefaultCluster: false,
		},
		"error getting default cluster": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(nil, errors.New("other error"))
			},
			wantedErr: fmt.Errorf("get default cluster: other error"),
		},
		"has default cluster": {
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{}).
					Return(&ecs.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterArn: awsv2.String("cluster"),
								Status:     awsv2.String(statusActive),
							},
						},
					}, nil)
			},
			wantedHasDefaultCluster: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			ecs := ECS{
				client: mockECSClient,
			}

			hasDefaultCluster, err := ecs.HasDefaultCluster()
			if tc.wantedErr != nil {
				require.EqualError(t, tc.wantedErr, err.Error())
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.wantedHasDefaultCluster, hasDefaultCluster)
		})
	}
}

func TestECS_ActiveClusters(t *testing.T) {
	testCases := map[string]struct {
		inArns        []string
		mockECSClient func(m *mocks.Mockapi)

		wantedError    error
		wantedClusters []string
	}{
		"describe clusters returns error": {
			inArns: []string{"arn1"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeClusters(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("some error"))
			},
			wantedError: fmt.Errorf("describe clusters: some error"),
		},
		"ignore inactive cluster": {
			inArns: []string{"arn1", "arn2"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeClusters(gomock.Any(), &ecs.DescribeClustersInput{
						Clusters: []string{"arn1", "arn2"},
					}).
					Return(&ecs.DescribeClustersOutput{
						Clusters: []types.Cluster{
							{
								ClusterArn: awsv2.String("cluster1"),
								Status:     awsv2.String(statusActive),
							},
							{
								ClusterArn: awsv2.String("cluster2"),
								Status:     awsv2.String("INACTIVE"),
							},
							{
								ClusterArn: awsv2.String("cluster3"),
								Status:     awsv2.String(statusActive),
							},
							{
								ClusterArn: awsv2.String("cluster4"),
								Status:     awsv2.String("random"),
							},
						},
					}, nil)
			},
			wantedClusters: []string{
				"cluster1",
				"cluster3",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			ecs := ECS{
				client: mockECSClient,
			}
			clusters, err := ecs.ActiveClusters(tc.inArns...)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedClusters, clusters)
			}
		})
	}
}

func TestECS_ActiveServices(t *testing.T) {
	mockClusterArn := "arn:aws:ecs:us-west-2:1234567890:cluster/cluster1"
	testCases := map[string]struct {
		inClusterARN  string
		inArns        []string
		mockECSClient func(m *mocks.Mockapi)

		wantedError    error
		wantedServices []string
	}{
		"describe services returns error": {
			inClusterARN: mockClusterArn,
			inArns:       []string{"arn:aws:ecs:us-west-2:1234567890:service/cluster1/svc1", "arn:aws:ecs:us-west-2:1234567890:service/cluster2/svc2"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeServices(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("some error"))
			},
			wantedError: fmt.Errorf("describe services: some error"),
		},
		"ignore inactive service": {
			inClusterARN: "arn:aws:ecs:us-west-2:1234567890:cluster/cluster1",
			inArns:       []string{"arn:aws:ecs:us-west-2:1234567890:service/cluster1/svc1", "arn:aws:ecs:us-west-2:1234567890:service/cluster1/svc2"},
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().
					DescribeServices(gomock.Any(), gomock.Any()).
					Return(&ecs.DescribeServicesOutput{
						Services: []types.Service{
							{
								ServiceArn: awsv2.String("service1"),
								Status:     awsv2.String(statusActive),
							},
							{
								ServiceArn: awsv2.String("service2"),
								Status:     awsv2.String("random"),
							},
						},
					}, nil)
			},
			wantedServices: []string{
				"service1",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			ecs := ECS{
				client: mockECSClient,
			}
			services, err := ecs.ActiveServices(tc.inClusterARN, tc.inArns...)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedServices, services)
			}
		})
	}
}

func TestECS_RunTask(t *testing.T) {
	type input struct {
		cluster         string
		count           int
		subnets         []string
		securityGroups  []string
		taskFamilyName  string
		startedBy       string
		platformVersion string
		enableExec      bool
	}

	runTaskInput := input{
		cluster:         "my-cluster",
		count:           3,
		subnets:         []string{"subnet-1", "subnet-2"},
		securityGroups:  []string{"sg-1", "sg-2"},
		taskFamilyName:  "my-task",
		startedBy:       "task",
		platformVersion: "LATEST",
		enableExec:      true,
	}
	ecsTasks := []types.Task{
		{
			TaskArn: awsv2.String("task-1"),
		},
		{
			TaskArn: awsv2.String("task-2"),
		},
		{
			TaskArn: awsv2.String("task-3"),
		},
	}
	describeTasksInput := ecs.DescribeTasksInput{
		Cluster: awsv2.String("my-cluster"),
		Tasks:   []string{"task-1", "task-2", "task-3"},
		Include: []types.TaskField{types.TaskFieldTags},
	}
	testCases := map[string]struct {
		input

		mockECSClient func(m *mocks.Mockapi)

		wantedError error
		wantedTasks []*Task
	}{
		"run task success": {
			input: runTaskInput,
			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().RunTask(gomock.Any(), &ecs.RunTaskInput{
					Cluster:        awsv2.String("my-cluster"),
					Count:          awsv2.Int32(int32(3)),
					LaunchType:     types.LaunchTypeFargate,
					StartedBy:      awsv2.String("task"),
					TaskDefinition: awsv2.String("my-task"),
					NetworkConfiguration: &types.NetworkConfiguration{
						AwsvpcConfiguration: &types.AwsVpcConfiguration{
							AssignPublicIp: types.AssignPublicIpEnabled,
							Subnets:        []string{"subnet-1", "subnet-2"},
							SecurityGroups: []string{"sg-1", "sg-2"},
						},
					},
					EnableExecuteCommand: true,
					PlatformVersion:      awsv2.String("LATEST"),
					PropagateTags:        types.PropagateTagsTaskDefinition,
				}).Return(&ecs.RunTaskOutput{
					Tasks: ecsTasks,
				}, nil)
				m.EXPECT().WaitUntilTasksRunning(gomock.Any(), &describeTasksInput, gomock.Any()).Times(1)
				m.EXPECT().DescribeTasks(gomock.Any(), &describeTasksInput).Return(&ecs.DescribeTasksOutput{
					Tasks: ecsTasks,
				}, nil)
			},
			wantedTasks: []*Task{
				{
					TaskArn: awsv2.String("task-1"),
				},
				{
					TaskArn: awsv2.String("task-2"),
				},
				{
					TaskArn: awsv2.String("task-3"),
				},
			},
		},
		"run task failed": {
			input: runTaskInput,

			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().RunTask(gomock.Any(), &ecs.RunTaskInput{
					Cluster:        awsv2.String("my-cluster"),
					Count:          awsv2.Int32(int32(3)),
					LaunchType:     types.LaunchTypeFargate,
					StartedBy:      awsv2.String("task"),
					TaskDefinition: awsv2.String("my-task"),
					NetworkConfiguration: &types.NetworkConfiguration{
						AwsvpcConfiguration: &types.AwsVpcConfiguration{
							AssignPublicIp: types.AssignPublicIpEnabled,
							Subnets:        []string{"subnet-1", "subnet-2"},
							SecurityGroups: []string{"sg-1", "sg-2"},
						},
					},
					EnableExecuteCommand: true,
					PlatformVersion:      awsv2.String("LATEST"),
					PropagateTags:        types.PropagateTagsTaskDefinition,
				}).
					Return(&ecs.RunTaskOutput{}, errors.New("error"))
			},
			wantedError: errors.New("run task(s) my-task: error"),
		},
		"failed to call WaitUntilTasksRunning": {
			input: runTaskInput,

			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().RunTask(gomock.Any(), &ecs.RunTaskInput{
					Cluster:        awsv2.String("my-cluster"),
					Count:          awsv2.Int32(int32(3)),
					LaunchType:     types.LaunchTypeFargate,
					StartedBy:      awsv2.String("task"),
					TaskDefinition: awsv2.String("my-task"),
					NetworkConfiguration: &types.NetworkConfiguration{
						AwsvpcConfiguration: &types.AwsVpcConfiguration{
							AssignPublicIp: types.AssignPublicIpEnabled,
							Subnets:        []string{"subnet-1", "subnet-2"},
							SecurityGroups: []string{"sg-1", "sg-2"},
						},
					},
					EnableExecuteCommand: true,
					PlatformVersion:      awsv2.String("LATEST"),
					PropagateTags:        types.PropagateTagsTaskDefinition,
				}).
					Return(&ecs.RunTaskOutput{
						Tasks: ecsTasks,
					}, nil)
				m.EXPECT().WaitUntilTasksRunning(gomock.Any(), &describeTasksInput, gomock.Any()).Return(errors.New("some error"))
			},
			wantedError: errors.New("wait for tasks to be running: some error"),
		},
		"task failed to start": {
			input: runTaskInput,

			mockECSClient: func(m *mocks.Mockapi) {
				m.EXPECT().RunTask(gomock.Any(), &ecs.RunTaskInput{
					Cluster:        awsv2.String("my-cluster"),
					Count:          awsv2.Int32(int32(3)),
					LaunchType:     types.LaunchTypeFargate,
					StartedBy:      awsv2.String("task"),
					TaskDefinition: awsv2.String("my-task"),
					NetworkConfiguration: &types.NetworkConfiguration{
						AwsvpcConfiguration: &types.AwsVpcConfiguration{
							AssignPublicIp: types.AssignPublicIpEnabled,
							Subnets:        []string{"subnet-1", "subnet-2"},
							SecurityGroups: []string{"sg-1", "sg-2"},
						},
					},
					EnableExecuteCommand: true,
					PlatformVersion:      awsv2.String("LATEST"),
					PropagateTags:        types.PropagateTagsTaskDefinition,
				}).
					Return(&ecs.RunTaskOutput{
						Tasks: ecsTasks}, nil)
				m.EXPECT().WaitUntilTasksRunning(gomock.Any(), &describeTasksInput, gomock.Any()).
					Return(errors.New("exceeded max wait time for TasksRunning waiter"))
				m.EXPECT().DescribeTasks(gomock.Any(), &describeTasksInput).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("task-1"),
						},
						{
							TaskArn:       awsv2.String("arn:aws:ecs:us-west-2:123456789:task/4082490ee6c245e09d2145010aa1ba8d"),
							StoppedReason: awsv2.String("Task failed to start"),
							LastStatus:    awsv2.String("STOPPED"),
							Containers: []types.Container{
								{
									Reason:     awsv2.String("CannotPullContainerError: inspect image has been retried 1 time(s)"),
									LastStatus: awsv2.String("STOPPED"),
								},
							},
						},
						{
							TaskArn: awsv2.String("task-3"),
						},
					},
				}, nil)
			},
			wantedError: errors.New("task 4082490e: Task failed to start: CannotPullContainerError: inspect image has been retried 1 time(s)"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockECSClient := mocks.NewMockapi(ctrl)
			tc.mockECSClient(mockECSClient)

			ecs := ECS{
				client: mockECSClient,
			}

			tasks, err := ecs.RunTask(RunTaskInput{
				Count:           tc.count,
				Cluster:         tc.cluster,
				TaskFamilyName:  tc.taskFamilyName,
				Subnets:         tc.subnets,
				SecurityGroups:  tc.securityGroups,
				StartedBy:       tc.startedBy,
				PlatformVersion: tc.platformVersion,
				EnableExec:      tc.enableExec,
			})

			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.Equal(t, tc.wantedTasks, tasks)
			}
		})
	}
}

func TestECS_DescribeTasks(t *testing.T) {
	inCluster := "my-cluster"
	inTaskARNs := []string{"task-1", "task-2", "task-3"}
	testCases := map[string]struct {
		mockAPI     func(m *mocks.Mockapi)
		wantedError error
		wantedTasks []*Task
	}{
		"error describing tasks": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String(inCluster),
					Tasks:   inTaskARNs,
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(nil, errors.New("error describing tasks"))
			},
			wantedError: fmt.Errorf("describe tasks: %w", errors.New("error describing tasks")),
		},
		"successfully described tasks": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeTasks(gomock.Any(), &ecs.DescribeTasksInput{
					Cluster: awsv2.String(inCluster),
					Tasks:   inTaskARNs,
					Include: []types.TaskField{types.TaskFieldTags},
				}).Return(&ecs.DescribeTasksOutput{
					Tasks: []types.Task{
						{
							TaskArn: awsv2.String("task-1"),
						},
						{
							TaskArn: awsv2.String("task-2"),
						},
						{
							TaskArn: awsv2.String("task-3"),
						},
					},
				}, nil)
			},
			wantedTasks: []*Task{
				{
					TaskArn: awsv2.String("task-1"),
				},
				{
					TaskArn: awsv2.String("task-2"),
				},
				{
					TaskArn: awsv2.String("task-3"),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockAPI(mockAPI)

			ecs := ECS{
				client: mockAPI,
			}

			tasks, err := ecs.DescribeTasks(inCluster, inTaskARNs)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedTasks, tasks)
			}
		})
	}
}

func TestECS_ExecuteCommand(t *testing.T) {
	mockExecCmdIn := &ecs.ExecuteCommandInput{
		Cluster:     awsv2.String("mockCluster"),
		Command:     awsv2.String("mockCommand"),
		Interactive: true,
		Container:   awsv2.String("mockContainer"),
		Task:        awsv2.String("mockTask"),
	}
	mockSess := &types.Session{
		SessionId: awsv2.String("mockSessID"),
	}
	mockErr := errors.New("some error")
	testCases := map[string]struct {
		mockAPI         func(m *mocks.Mockapi)
		mockSessStarter func(m *mocks.MockssmSessionStarter)
		wantedError     error
	}{
		"return error if fail to call ExecuteCommand": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().ExecuteCommand(gomock.Any(), mockExecCmdIn).Return(nil, mockErr)
			},
			mockSessStarter: func(m *mocks.MockssmSessionStarter) {},
			wantedError:     &ErrExecuteCommand{err: mockErr},
		},
		"return error if fail to start the session": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().ExecuteCommand(gomock.Any(), &ecs.ExecuteCommandInput{
					Cluster:     awsv2.String("mockCluster"),
					Command:     awsv2.String("mockCommand"),
					Interactive: true,
					Container:   awsv2.String("mockContainer"),
					Task:        awsv2.String("mockTask"),
				}).Return(&ecs.ExecuteCommandOutput{
					Session: mockSess,
				}, nil)
			},
			mockSessStarter: func(m *mocks.MockssmSessionStarter) {
				m.EXPECT().StartSession(mockSess).Return(mockErr)
			},
			wantedError: fmt.Errorf("start session mockSessID using ssm plugin: some error"),
		},
		"success": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().ExecuteCommand(gomock.Any(), mockExecCmdIn).Return(&ecs.ExecuteCommandOutput{
					Session: mockSess,
				}, nil)
			},
			mockSessStarter: func(m *mocks.MockssmSessionStarter) {
				m.EXPECT().StartSession(mockSess).Return(nil)
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPI := mocks.NewMockapi(ctrl)
			mockSessStarter := mocks.NewMockssmSessionStarter(ctrl)
			tc.mockAPI(mockAPI)
			tc.mockSessStarter(mockSessStarter)

			ecs := ECS{
				client: mockAPI,
				newSessStarter: func() ssmSessionStarter {
					return mockSessStarter
				},
			}

			err := ecs.ExecuteCommand(ExecuteCommandInput{
				Cluster:   "mockCluster",
				Command:   "mockCommand",
				Container: "mockContainer",
				Task:      "mockTask",
			})
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestECS_NetworkConfiguration(t *testing.T) {
	testCases := map[string]struct {
		mockAPI func(m *mocks.Mockapi)

		wantedError                error
		wantedNetworkConfiguration *NetworkConfiguration
	}{
		"success": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("crowded-cluster"),
					Services: []string{"cool-service"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("cool-service"),
							NetworkConfiguration: &types.NetworkConfiguration{
								AwsvpcConfiguration: &types.AwsVpcConfiguration{
									AssignPublicIp: types.AssignPublicIp("1.2.3.4"),
									SecurityGroups: []string{"sg-1", "sg-2"},
									Subnets:        []string{"sbn-1", "sbn-2"},
								},
							},
						},
					},
				}, nil)
			},
			wantedNetworkConfiguration: &NetworkConfiguration{
				AssignPublicIp: "1.2.3.4",
				SecurityGroups: []string{"sg-1", "sg-2"},
				Subnets:        []string{"sbn-1", "sbn-2"},
			},
		},
		"fail to describe service": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("crowded-cluster"),
					Services: []string{"cool-service"},
				}).Return(nil, errors.New("some error"))
			},
			wantedError: fmt.Errorf("describe service cool-service: some error"),
		},
		"fail to find awsvpc configuration": {
			mockAPI: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeServices(gomock.Any(), &ecs.DescribeServicesInput{
					Cluster:  awsv2.String("crowded-cluster"),
					Services: []string{"cool-service"},
				}).Return(&ecs.DescribeServicesOutput{
					Services: []types.Service{
						{
							ServiceName: awsv2.String("cool-service"),
						},
					},
				}, nil)
			},
			wantedError: errors.New("cannot find the awsvpc configuration for service cool-service"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAPI := mocks.NewMockapi(ctrl)
			tc.mockAPI(mockAPI)

			e := ECS{
				client: mockAPI,
			}

			inCluster := "crowded-cluster"
			inServiceName := "cool-service"
			got, err := e.NetworkConfiguration(inCluster, inServiceName)
			if tc.wantedError != nil {
				require.EqualError(t, tc.wantedError, err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedNetworkConfiguration, got)
			}
		})
	}
}
