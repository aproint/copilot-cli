// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package aas

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/aas/mocks"
	"github.com/aws/aws-sdk-go-v2/aws"
	aas "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	"github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type aasMocks struct {
	client *mocks.Mockapi
}

func TestCloudWatch_ECSServiceAutoscalingAlarms(t *testing.T) {
	const (
		mockCluster    = "mockCluster"
		mockService    = "mockService"
		mockResourceID = "service/mockCluster/mockService"
		mockNextToken  = "mockNextToken"
	)
	mockError := errors.New("some error")

	testCases := map[string]struct {
		setupMocks func(m aasMocks)

		wantErr        error
		wantAlarmNames []string
	}{
		"errors if failed to retrieve auto scaling alarm names": {
			setupMocks: func(m aasMocks) {
				m.client.EXPECT().DescribeScalingPolicies(gomock.Any(), gomock.Any()).Return(nil, mockError)
			},

			wantErr: fmt.Errorf("describe scaling policies for ECS service mockCluster/mockService: some error"),
		},
		"success": {
			setupMocks: func(m aasMocks) {
				m.client.EXPECT().DescribeScalingPolicies(context.Background(), &aas.DescribeScalingPoliciesInput{
					ResourceId:       aws.String(mockResourceID),
					ServiceNamespace: types.ServiceNamespaceEcs,
				}).Return(&aas.DescribeScalingPoliciesOutput{
					ScalingPolicies: []types.ScalingPolicy{
						{
							Alarms: []types.Alarm{
								{
									AlarmName: aws.String("mockAlarm1"),
								},
								{
									AlarmName: aws.String("mockAlarm2"),
								},
							},
						},
						{
							Alarms: []types.Alarm{
								{
									AlarmName: aws.String("mockAlarm3"),
								},
							},
						},
					},
				}, nil)
			},

			wantAlarmNames: []string{"mockAlarm1", "mockAlarm2", "mockAlarm3"},
		},
		"success with pagination": {
			setupMocks: func(m aasMocks) {
				gomock.InOrder(
					m.client.EXPECT().DescribeScalingPolicies(context.Background(), &aas.DescribeScalingPoliciesInput{
						ResourceId:       aws.String(mockResourceID),
						ServiceNamespace: types.ServiceNamespaceEcs,
					}).Return(&aas.DescribeScalingPoliciesOutput{
						ScalingPolicies: []types.ScalingPolicy{
							{
								Alarms: []types.Alarm{
									{
										AlarmName: aws.String("mockAlarm1"),
									},
								},
							},
						},
						NextToken: aws.String(mockNextToken),
					}, nil),
					m.client.EXPECT().DescribeScalingPolicies(context.Background(), &aas.DescribeScalingPoliciesInput{
						ResourceId:       aws.String(mockResourceID),
						ServiceNamespace: types.ServiceNamespaceEcs,
						NextToken:        aws.String(mockNextToken),
					}).Return(&aas.DescribeScalingPoliciesOutput{
						ScalingPolicies: []types.ScalingPolicy{
							{
								Alarms: []types.Alarm{
									{
										AlarmName: aws.String("mockAlarm2"),
									},
								},
							},
						},
					}, nil),
				)
			},

			wantAlarmNames: []string{"mockAlarm1", "mockAlarm2"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockapi(ctrl)
			mocks := aasMocks{
				client: mockClient,
			}

			tc.setupMocks(mocks)

			aasSvc := ApplicationAutoscaling{
				client: mockClient,
			}

			gotAlarmNames, gotErr := aasSvc.ECSServiceAlarmNames(mockCluster, mockService)

			if gotErr != nil {
				require.EqualError(t, tc.wantErr, gotErr.Error())
			} else {
				require.Equal(t, tc.wantAlarmNames, gotAlarmNames)
			}
		})

	}
}
