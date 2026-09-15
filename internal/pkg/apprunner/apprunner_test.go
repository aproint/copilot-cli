// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package apprunner

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/apprunner/mocks"
	"github.com/aproint/copilot-cli/internal/pkg/aws/apprunner"
	"github.com/aproint/copilot-cli/internal/pkg/aws/resourcegroups"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type clientMocks struct {
	rgMock        *mocks.MockresourceGetter
	appRunnerMock *mocks.MockappRunnerClient
}

func TestClient_ForceUpdateServiceUsesCallerContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller context")
	rg := mocks.NewMockresourceGetter(ctrl)
	appRunner := mocks.NewMockappRunnerClient(ctrl)
	tags := map[string]string{
		deploy.AppTagKey:     "mockApp",
		deploy.EnvTagKey:     "mockEnv",
		deploy.ServiceTagKey: "mockSvc",
	}

	rg.EXPECT().GetResourcesByTags(ctx, serviceResourceType, tags).Return([]*resourcegroups.Resource{
		{ARN: "mockSvcARN"},
	}, nil)
	appRunner.EXPECT().StartDeployment(ctx, "mockSvcARN").Return("mockOperationID", nil)
	appRunner.EXPECT().WaitForOperation(ctx, "mockOperationID", "mockSvcARN").Return(nil)

	client := Client{appRunnerClient: appRunner, rgGetter: rg}
	require.NoError(t, client.ForceUpdateService(ctx, "mockApp", "mockEnv", "mockSvc"))
}

func TestClient_LastUpdatedAtUsesCallerContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller context")
	rg := mocks.NewMockresourceGetter(ctrl)
	appRunner := mocks.NewMockappRunnerClient(ctrl)
	want := time.Unix(1494505756, 0)
	tags := map[string]string{
		deploy.AppTagKey:     "mockApp",
		deploy.EnvTagKey:     "mockEnv",
		deploy.ServiceTagKey: "mockSvc",
	}

	rg.EXPECT().GetResourcesByTags(ctx, serviceResourceType, tags).Return([]*resourcegroups.Resource{
		{ARN: "mockSvcARN"},
	}, nil)
	appRunner.EXPECT().DescribeService(ctx, "mockSvcARN").Return(&apprunner.Service{
		DateUpdated: want,
	}, nil)

	client := Client{appRunnerClient: appRunner, rgGetter: rg}
	got, err := client.LastUpdatedAt(ctx, "mockApp", "mockEnv", "mockSvc")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestClient_ForceUpdateService(t *testing.T) {
	mockError := errors.New("some error")
	const (
		mockApp         = "mockApp"
		mockSvc         = "mockSvc"
		mockEnv         = "mockEnv"
		mockSvcARN      = "mockSvcARN"
		mockOperationID = "mockOperationID"
	)
	getRgInput := map[string]string{
		deploy.AppTagKey:     mockApp,
		deploy.EnvTagKey:     mockEnv,
		deploy.ServiceTagKey: mockSvc,
	}
	tests := map[string]struct {
		mock func(m *clientMocks)

		wantErr error
	}{
		"fail get the app runner service": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).Return(nil, mockError)
			},
			wantErr: fmt.Errorf("get App Runner service with tags (mockApp, mockEnv, mockSvc): some error"),
		},
		"no app runner service found": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{}, nil)
			},
			wantErr: fmt.Errorf("no App Runner service found for mockSvc in environment mockEnv"),
		},
		"more than one app runner service found": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{}, {},
					}, nil)
			},
			wantErr: fmt.Errorf("more than one App Runner service with the name mockSvc found in environment mockEnv"),
		},
		"error if fail to start new deployment": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{
							ARN: mockSvcARN,
						},
					}, nil)
				m.appRunnerMock.EXPECT().StartDeployment(context.Background(), mockSvcARN).Return("", mockError)
			},
			wantErr: fmt.Errorf("some error"),
		},
		"error if fail to wait for deployment": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{
							ARN: mockSvcARN,
						},
					}, nil)
				m.appRunnerMock.EXPECT().StartDeployment(context.Background(), mockSvcARN).Return(mockOperationID, nil)
				m.appRunnerMock.EXPECT().WaitForOperation(context.Background(), mockOperationID, mockSvcARN).Return(mockError)
			},
			wantErr: fmt.Errorf("some error"),
		},
		"success": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{
							ARN: mockSvcARN,
						},
					}, nil)
				m.appRunnerMock.EXPECT().StartDeployment(context.Background(), mockSvcARN).Return(mockOperationID, nil)
				m.appRunnerMock.EXPECT().WaitForOperation(context.Background(), mockOperationID, mockSvcARN).Return(nil)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRg := mocks.NewMockresourceGetter(ctrl)
			mockAppRunner := mocks.NewMockappRunnerClient(ctrl)
			m := &clientMocks{
				rgMock:        mockRg,
				appRunnerMock: mockAppRunner,
			}
			tc.mock(m)

			c := Client{
				appRunnerClient: mockAppRunner,
				rgGetter:        mockRg,
			}

			gotErr := c.ForceUpdateService(context.Background(), mockApp, mockEnv, mockSvc)

			if tc.wantErr != nil {
				require.EqualError(t, gotErr, tc.wantErr.Error())
			} else {
				require.NoError(t, gotErr)
			}
		})
	}
}

func TestClient_LastUpdatedAt(t *testing.T) {
	mockError := errors.New("some error")
	const (
		mockApp    = "mockApp"
		mockSvc    = "mockSvc"
		mockEnv    = "mockEnv"
		mockSvcARN = "mockSvcARN"
	)
	mockTime := time.Unix(1494505756, 0)
	getRgInput := map[string]string{
		deploy.AppTagKey:     mockApp,
		deploy.EnvTagKey:     mockEnv,
		deploy.ServiceTagKey: mockSvc,
	}
	tests := map[string]struct {
		mock func(m *clientMocks)

		wantErr error
		want    time.Time
	}{
		"error if fail to describe service": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{
							ARN: mockSvcARN,
						},
					}, nil)
				m.appRunnerMock.EXPECT().DescribeService(context.Background(), mockSvcARN).Return(nil, mockError)
			},
			wantErr: fmt.Errorf("describe service: some error"),
		},
		"succeed": {
			mock: func(m *clientMocks) {
				m.rgMock.EXPECT().GetResourcesByTags(context.Background(), serviceResourceType, getRgInput).
					Return([]*resourcegroups.Resource{
						{
							ARN: mockSvcARN,
						},
					}, nil)
				m.appRunnerMock.EXPECT().DescribeService(context.Background(), mockSvcARN).Return(&apprunner.Service{
					DateUpdated: mockTime,
				}, nil)
			},
			want: mockTime,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRg := mocks.NewMockresourceGetter(ctrl)
			mockAppRunner := mocks.NewMockappRunnerClient(ctrl)
			m := &clientMocks{
				rgMock:        mockRg,
				appRunnerMock: mockAppRunner,
			}
			tc.mock(m)

			c := Client{
				appRunnerClient: mockAppRunner,
				rgGetter:        mockRg,
			}

			got, gotErr := c.LastUpdatedAt(context.Background(), mockApp, mockEnv, mockSvc)

			if tc.wantErr != nil {
				require.EqualError(t, gotErr, tc.wantErr.Error())
			} else {
				require.NoError(t, gotErr)
				require.Equal(t, tc.want, got)
			}
		})
	}
}
