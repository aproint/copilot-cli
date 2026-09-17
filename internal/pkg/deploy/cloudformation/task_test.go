// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/stretchr/testify/require"

	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/mocks"
	"github.com/golang/mock/gomock"
)

func TestCloudFormation_DeployTask(t *testing.T) {
	mockTask := &deploy.CreateTaskResourcesInput{
		Name: "hello",
	}
	when := func(cf CloudFormation) error {
		return cf.DeployTask(context.Background(), mockTask)
	}

	t.Run("returns a wrapped error if creating a change set fails", func(t *testing.T) {
		testDeployTask_OnCreateChangeSetFailure(t, when)
	})
	t.Run("calls Update if stack is already created and returns wrapped error if Update fails", func(t *testing.T) {
		testDeployTask_OnUpdateChangeSetFailure(t, when)
	})
	t.Run("returns nil if the change set is empty when calling Update", func(t *testing.T) {
		testDeployTask_ReturnNilOnEmptyChangeSetWhileUpdatingStack(t, when)
	})
	t.Run("returns an error when the ChangeSet cannot be described for stack changes before rendering", func(t *testing.T) {
		testDeployTask_OnDescribeChangeSetFailure(t, when)
	})
	t.Run("returns an error when stack template body cannot be retrieved to parse resource descriptions", func(t *testing.T) {
		testDeployTask_OnTemplateBodyFailure(t, when)
	})
	t.Run("returns a wrapped error if a streamer fails and cancels the renderer", func(t *testing.T) {
		testDeployTask_StackStreamerFailureShouldCancelRenderer(t, when)
	})
	t.Run("returns an error if stack creation fails", func(t *testing.T) {
		testDeployTask_StreamUntilStackCreationFails(t, "task-hello", when)
	})
	t.Run("renders a stack with addons template if stack creation is successful", func(t *testing.T) {
		testDeployTask_RenderNewlyCreatedStackWithAddons(t, "task-hello", when)
	})
}

var mockDescription1 = &cloudformation.StackDescription{
	Tags: []awscfn.Tag{
		{
			Key: aws.String("copilot-task"),
		},
		{
			Key:   aws.String("copilot-application"),
			Value: aws.String("appname"),
		},
		{
			Key:   aws.String("copilot-environment"),
			Value: aws.String("test"),
		},
	},
	StackName: aws.String("task-database"),
	RoleARN:   aws.String("arn:aws:iam::123456789012:role/appname-test-CFNExecutionRole"),
}
var mockDescription2 = &cloudformation.StackDescription{
	Tags: []awscfn.Tag{
		{
			Key: aws.String("copilot-task"),
		},
		{
			Key:   aws.String("copilot-application"),
			Value: aws.String("otherapp"),
		},
		{
			Key:   aws.String("copilot-environment"),
			Value: aws.String("test"),
		},
	},
	StackName: aws.String("task-example"),
	RoleARN:   aws.String("arn:aws:iam::123456789012:role/otherapp-staging-CFNExecutionRole"),
}

var mockDescription3 = &cloudformation.StackDescription{
	Tags: []awscfn.Tag{
		{
			Key: aws.String("copilot-task"),
		},
	},
	StackName: aws.String("task-default"),
	RoleARN:   aws.String(""),
}

func TestCloudFormation_ListTaskStacks(t *testing.T) {
	testCases := map[string]struct {
		inAppName   string
		mockClient  func(*mocks.MockcfnClient)
		wantedErr   string
		wantedTasks []deploy.TaskStackInfo
	}{
		"successfully gets task stacks while excluding wrongly tagged stack": {
			inAppName: "appname",
			mockClient: func(m *mocks.MockcfnClient) {
				m.EXPECT().ListStacksWithTags(gomock.Any(), map[string]string{
					"copilot-application": "appname",
					"copilot-environment": "test",
					"copilot-task":        "",
				}).Return([]cloudformation.StackDescription{
					*mockDescription1,
				}, nil)
			},
			wantedTasks: []deploy.TaskStackInfo{
				{
					StackName: "task-database",
					App:       "appname",
					Env:       "test",
					RoleARN:   aws.ToString(mockDescription1.RoleARN),
				},
			},
		},
		"error listing stacks": {
			inAppName: "appname",
			mockClient: func(m *mocks.MockcfnClient) {
				m.EXPECT().ListStacksWithTags(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
			},
			wantedErr: "some error",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCf := mocks.NewMockcfnClient(ctrl)
			tc.mockClient(mockCf)

			cf := CloudFormation{cfnClient: mockCf}

			// WHEN
			tasks, err := cf.ListTaskStacks(context.Background(), "appname", "test")

			if tc.wantedErr != "" {
				require.EqualError(t, err, tc.wantedErr)
			} else {
				require.Equal(t, tc.wantedTasks, tasks)
			}
		})
	}
}

func TestCloudFormation_GetTaskDefaultStackInfo(t *testing.T) {
	testCases := map[string]struct {
		inAppName   string
		mockClient  func(*mocks.MockcfnClient)
		wantedErr   string
		wantedTasks []deploy.TaskStackInfo
	}{
		"successfully gets task stacks while excluding wrongly tagged stack": {
			inAppName: "appname",
			mockClient: func(m *mocks.MockcfnClient) {
				m.EXPECT().ListStacksWithTags(gomock.Any(), map[string]string{
					"copilot-task": "",
				}).Return([]cloudformation.StackDescription{
					*mockDescription1,
					*mockDescription2,
					*mockDescription3,
				}, nil)
			},
			wantedTasks: []deploy.TaskStackInfo{
				{
					StackName: "task-default",
				},
			},
		},

		"error listing stacks": {
			inAppName: "appname",
			mockClient: func(m *mocks.MockcfnClient) {
				m.EXPECT().ListStacksWithTags(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
			},
			wantedErr: "some error",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCf := mocks.NewMockcfnClient(ctrl)
			tc.mockClient(mockCf)

			cf := CloudFormation{cfnClient: mockCf}

			// WHEN
			tasks, err := cf.ListDefaultTaskStacks(context.Background())

			if tc.wantedErr != "" {
				require.EqualError(t, err, tc.wantedErr)
			} else {
				require.Equal(t, tc.wantedTasks, tasks)
			}

		})
	}

}

func TestCloudFormation_GetTaskStackUsesCallerContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("sentinel"), "task-stack")
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	client := mocks.NewMockcfnClient(ctrl)
	client.EXPECT().Describe(gomock.Eq(ctx), "task-database").Return(mockDescription1, nil)
	cf := CloudFormation{cfnClient: client}

	info, err := cf.GetTaskStack(ctx, "database")

	require.NoError(t, err)
	require.Equal(t, &deploy.TaskStackInfo{
		StackName: "task-database",
		App:       "appname",
		Env:       "test",
		RoleARN:   aws.ToString(mockDescription1.RoleARN),
	}, info)
}

func TestCloudFormation_DeleteTaskUsesCallerContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("sentinel"), "delete-task")

	t.Run("with role", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockcfnClient(ctrl)
		client.EXPECT().DeleteAndWaitWithRoleARN(gomock.Eq(ctx), "task-database", "role").Return(nil)
		cf := CloudFormation{cfnClient: client}

		err := cf.DeleteTask(ctx, deploy.TaskStackInfo{StackName: "task-database", RoleARN: "role"})

		require.NoError(t, err)
	})

	t.Run("without role", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		client := mocks.NewMockcfnClient(ctrl)
		client.EXPECT().DeleteAndWait(gomock.Eq(ctx), "task-database").Return(nil)
		cf := CloudFormation{cfnClient: client}

		err := cf.DeleteTask(ctx, deploy.TaskStackInfo{StackName: "task-database"})

		require.NoError(t, err)
	})
}
