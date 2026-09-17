// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"context"
	"errors"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/mocks"
	"github.com/aproint/copilot-cli/internal/pkg/template"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestCloudFormation_GetEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	parent := context.WithValue(context.Background(), "key", "value")
	mockClient := mocks.NewMockcfnClient(ctrl)
	mockClient.EXPECT().Describe(parent, "phonetool-test").
		Return(&cloudformation.StackDescription{
			StackId: aws.String("arn:aws:cloudformation:us-west-2:123456789012:stack/phonetool-test/abc123"),
			Outputs: []awscfn.Output{
				{
					OutputKey:   aws.String("EnvironmentManagerRoleARN"),
					OutputValue: aws.String("arn:aws:iam::123456789012:role/manager"),
				},
				{
					OutputKey:   aws.String("CFNExecutionRoleARN"),
					OutputValue: aws.String("arn:aws:iam::123456789012:role/execution"),
				},
			},
		}, nil)

	cf := &CloudFormation{
		cfnClient: mockClient,
	}

	got, err := cf.GetEnvironment(parent, "phonetool", "test")

	require.NoError(t, err)
	require.Equal(t, &config.Environment{
		App:              "phonetool",
		Name:             "test",
		AccountID:        "123456789012",
		Region:           "us-west-2",
		ManagerRoleARN:   "arn:aws:iam::123456789012:role/manager",
		ExecutionRoleARN: "arn:aws:iam::123456789012:role/execution",
	}, got)
}

func TestCloudFormation_DeleteEnvironment(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")
	client := mocks.NewMockcfnClient(ctrl)
	client.EXPECT().TemplateBody(ctx, "phonetool-test").Return("", context.Canceled)
	cf := &CloudFormation{cfnClient: client}

	err := cf.DeleteEnvironment(ctx, "phonetool", "test", "role")

	require.ErrorIs(t, err, context.Canceled)
}

func TestCloudFormation_DeployedEnvironmentParameters(t *testing.T) {
	testCases := map[string]struct {
		inAppName string
		inEnvName string
		inClient  func(ctrl *gomock.Controller) *mocks.MockcfnClient

		wantedParams []awscfn.Parameter
		wantedErr    error
	}{
		"error retrieving metadata": {
			inAppName: "phonetool",
			inEnvName: "test",
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Metadata(context.Background(), gomock.Any()).Return("", errors.New("some error"))
				return m
			},
			wantedErr: errors.New("get metadata of stack \"phonetool-test\": some error"),
		},
		"returns nil if the version is bootstrap": {
			inAppName: "phonetool",
			inEnvName: "test",
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Metadata(context.Background(), gomock.Any()).Return(`Version: bootstrap`, nil)
				return m
			},
		},
		"should return stack parameters from a stack description": {
			inAppName: "phonetool",
			inEnvName: "test",
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Metadata(context.Background(), gomock.Any()).Return(`Version: `, nil)
				m.EXPECT().Describe(context.Background(), "phonetool-test").Return(&cloudformation.StackDescription{
					Parameters: []awscfn.Parameter{
						{
							ParameterKey:   aws.String("name"),
							ParameterValue: aws.String("test"),
						},
					},
				}, nil)
				return m
			},

			wantedParams: []awscfn.Parameter{
				{
					ParameterKey:   aws.String("name"),
					ParameterValue: aws.String("test"),
				},
			},
		},
		"should return the error as is from a failed stack description": {
			inAppName: "phonetool",
			inEnvName: "test",
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Metadata(context.Background(), gomock.Any()).Return(`Version: v1.21.0`, nil)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(nil, errors.New("some error"))
				return m
			},
			wantedErr: errors.New("describe stack phonetool-test: some error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := &CloudFormation{
				cfnClient: tc.inClient(ctrl),
			}

			// WHEN
			actual, err := cf.DeployedEnvironmentParameters(context.Background(), tc.inAppName, tc.inEnvName)
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, tc.wantedParams, actual)
			}
		})
	}
}

func TestCloudFormation_ForceUpdateID(t *testing.T) {
	testCases := map[string]struct {
		inClient func(ctrl *gomock.Controller) *mocks.MockcfnClient

		wanted    string
		wantedErr error
	}{
		"should return stack parameters from a stack description": {
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), "phonetool-test").Return(&cloudformation.StackDescription{
					Outputs: []awscfn.Output{
						{
							OutputKey:   aws.String(template.LastForceDeployIDOutputName),
							OutputValue: aws.String("mockForceUpdateID"),
						},
					},
				}, nil)
				return m
			},
			wanted: "mockForceUpdateID",
		},
		"error describing the stack": {
			inClient: func(ctrl *gomock.Controller) *mocks.MockcfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(nil, errors.New("some error"))
				return m
			},
			wantedErr: errors.New("describe stack phonetool-test: some error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := &CloudFormation{
				cfnClient: tc.inClient(ctrl),
			}

			// WHEN
			actual, err := cf.ForceUpdateOutputID(context.Background(), "phonetool", "test")
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wanted, actual)
			}
		})
	}
}

func TestCloudFormation_UpdateEnvironmentTemplate(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")
	client := mocks.NewMockcfnClient(ctrl)
	params := []awscfn.Parameter{{ParameterKey: aws.String("ALBWorkloads"), ParameterValue: aws.String("frontend")}}
	tags := []awscfn.Tag{{Key: aws.String("copilot-application"), Value: aws.String("phonetool")}}
	client.EXPECT().Describe(ctx, "phonetool-test").Return(&cloudformation.StackDescription{
		Parameters: params,
		Tags:       tags,
	}, nil)
	client.EXPECT().UpdateAndWait(ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, s *cloudformation.Stack) error {
			require.Equal(t, "phonetool-test", s.Name)
			require.Equal(t, params, s.Parameters)
			require.Equal(t, tags, s.Tags)
			require.Equal(t, "hello", s.TemplateBody)
			require.Equal(t, aws.String("arn"), s.RoleARN)
			return nil
		},
	)

	cf := &CloudFormation{cfnClient: client}
	err := cf.UpdateEnvironmentTemplate(ctx, "phonetool", "test", "hello", "arn")

	require.NoError(t, err)
}
