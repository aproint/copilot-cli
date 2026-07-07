// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resourcegroups

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/resourcegroups/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	rgapi "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

var testTags = map[string]string{
	"copilot-environment": "test",
}

const (
	testResourceType = "cloudwatch:alarm"

	testArn  = "arn:aws:cloudwatch:us-west-2:1234567890:alarm:SDc-ReadCapacityUnitsLimit-BasicAlarm"
	mockArn1 = "arn:aws:cloudwatch:us-west-2:1234567890:alarm:mockAlarmName1"
	mockArn2 = "arn:aws:cloudwatch:us-west-2:1234567890:alarm:mockAlarmName2"
)

func TestResourceGroups_GetResourcesByTags(t *testing.T) {
	mockRequest := &rgapi.GetResourcesInput{
		PaginationToken:     nil,
		ResourceTypeFilters: []string{testResourceType},
		TagFilters: []types.TagFilter{
			{
				Key:    awsv2.String("copilot-environment"),
				Values: []string{"test"},
			},
		},
	}
	mockResponse := &rgapi.GetResourcesOutput{
		ResourceTagMappingList: []types.ResourceTagMapping{
			{
				ResourceARN: awsv2.String(testArn),
				Tags: []types.Tag{
					{
						Key:   awsv2.String("copilot-environment"),
						Value: awsv2.String("test"),
					},
				},
			},
		},
	}
	mockError := errors.New("some error")

	testCases := map[string]struct {
		inTags         map[string]string
		inResourceType string
		setupMocks     func(m *mocks.Mockapi)
		expectedOut    []*Resource
		expectedErr    error
	}{
		"returns list of arns": {
			inTags:         testTags,
			inResourceType: testResourceType,
			setupMocks: func(m *mocks.Mockapi) {
				m.EXPECT().GetResources(gomock.Any(), mockRequest).Return(mockResponse, nil)
			},
			expectedOut: []*Resource{
				{
					ARN:  testArn,
					Tags: testTags,
				},
			},
			expectedErr: nil,
		},
		"wraps error from API call": {
			inTags:         testTags,
			inResourceType: testResourceType,
			setupMocks: func(m *mocks.Mockapi) {
				m.EXPECT().GetResources(gomock.Any(), mockRequest).Return(nil, mockError)
			},
			expectedOut: nil,
			expectedErr: fmt.Errorf("get resource: some error"),
		},
		"success with pagination": {
			inTags:         testTags,
			inResourceType: testResourceType,
			setupMocks: func(m *mocks.Mockapi) {
				gomock.InOrder(
					m.EXPECT().GetResources(gomock.Any(), mockRequest).Return(&rgapi.GetResourcesOutput{
						PaginationToken: awsv2.String("mockNextToken"),
						ResourceTagMappingList: []types.ResourceTagMapping{
							{
								ResourceARN: awsv2.String(mockArn1),
								Tags:        []types.Tag{{Key: awsv2.String("copilot-environment"), Value: awsv2.String("test")}},
							},
						},
					}, nil),
					m.EXPECT().GetResources(gomock.Any(), &rgapi.GetResourcesInput{
						PaginationToken:     awsv2.String("mockNextToken"),
						ResourceTypeFilters: []string{testResourceType},
						TagFilters: []types.TagFilter{
							{
								Key:    awsv2.String("copilot-environment"),
								Values: []string{"test"},
							},
						},
					}).Return(&rgapi.GetResourcesOutput{
						PaginationToken: nil,
						ResourceTagMappingList: []types.ResourceTagMapping{
							{
								ResourceARN: awsv2.String(mockArn2),
								Tags:        []types.Tag{{Key: awsv2.String("copilot-environment"), Value: awsv2.String("test")}},
							},
						},
					}, nil),
				)
			},
			expectedOut: []*Resource{
				{
					ARN:  mockArn1,
					Tags: testTags,
				},
				{
					ARN:  mockArn2,
					Tags: testTags,
				},
			},
			expectedErr: nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockapi(ctrl)
			rg := &ResourceGroups{client: mockClient}

			// WHEN
			tc.setupMocks(mockClient)
			actualOut, actualErr := rg.GetResourcesByTags(tc.inResourceType, tc.inTags)

			// THEN
			if actualErr != nil {
				require.EqualError(t, tc.expectedErr, actualErr.Error())
			} else {
				require.Equal(t, tc.expectedOut, actualOut)
			}
		})
	}
}
