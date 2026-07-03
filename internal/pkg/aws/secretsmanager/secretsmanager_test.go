// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package secretsmanager wraps AWS SecretsManager API functionality.
package secretsmanager

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/secretsmanager/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestSecretsManager_CreateSecret(t *testing.T) {
	mockSecretName := "github-token-backend-badgoose"
	mockSecretString := "H0NKH0NKH0NK"
	mockError := errors.New("mockError")
	mockOutput := &secretsmanager.CreateSecretOutput{
		ARN: awsv2.String("arn-goose"),
	}
	mockAwsErr := &types.ResourceExistsException{}

	tests := map[string]struct {
		inSecretName   string
		inSecretString string
		callMock       func(m *mocks.Mockapi)

		expectedError error
	}{
		"should wrap error returned by CreateSecret": {
			inSecretName:   mockSecretName,
			inSecretString: mockSecretString,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().CreateSecret(gomock.Any(), &secretsmanager.CreateSecretInput{
					Name:         awsv2.String(mockSecretName),
					SecretString: awsv2.String(mockSecretString),
					Tags:         []types.Tag{},
				}).Return(nil, mockError)
			},
			expectedError: fmt.Errorf("create secret %s: %w", mockSecretName, mockError),
		},

		"should return no error if secret already exists": {
			inSecretName:   mockSecretName,
			inSecretString: mockSecretString,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().CreateSecret(gomock.Any(), &secretsmanager.CreateSecretInput{
					Name:         awsv2.String(mockSecretName),
					SecretString: awsv2.String(mockSecretString),
					Tags:         []types.Tag{},
				}).Return(nil, mockAwsErr)
			},
			expectedError: &ErrSecretAlreadyExists{
				secretName: mockSecretName,
				parentErr:  mockAwsErr,
			},
		},

		"should return no error if successful": {
			inSecretName:   mockSecretName,
			inSecretString: mockSecretString,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().CreateSecret(gomock.Any(), &secretsmanager.CreateSecretInput{
					Name:         awsv2.String(mockSecretName),
					SecretString: awsv2.String(mockSecretString),
					Tags:         []types.Tag{},
				}).Return(mockOutput, nil)
			},
			expectedError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSecretsManager := mocks.NewMockapi(ctrl)

			sm := SecretsManager{
				secretsManager: mockSecretsManager,
			}

			tc.callMock(mockSecretsManager)

			// WHEN
			oldSecretTags := secretTags
			defer func() { secretTags = oldSecretTags }()
			secretTags = func() []types.Tag {
				return []types.Tag{}
			}

			_, err := sm.CreateSecret(tc.inSecretName, tc.inSecretString)

			// THEN
			require.Equal(t, tc.expectedError, err)
		})
	}
}

func TestSecretsManager_DeleteSecret(t *testing.T) {
	mockSecretName := "github-token-backend-badgoose"
	mockError := errors.New("mockError")

	tests := map[string]struct {
		inSecretName string
		callMock     func(m *mocks.Mockapi)

		expectedError error
	}{
		"should wrap error returned by DeleteSecret": {
			inSecretName: mockSecretName,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().DeleteSecret(gomock.Any(), &secretsmanager.DeleteSecretInput{
					SecretId:                   awsv2.String(mockSecretName),
					ForceDeleteWithoutRecovery: awsv2.Bool(true),
				}).Return(nil, mockError)
			},
			expectedError: fmt.Errorf("delete secret %s from secrets manager: %w", mockSecretName, mockError),
		},
		"should return no error if successful": {
			inSecretName: mockSecretName,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().DeleteSecret(gomock.Any(), &secretsmanager.DeleteSecretInput{
					SecretId:                   awsv2.String(mockSecretName),
					ForceDeleteWithoutRecovery: awsv2.Bool(true),
				}).Return(nil, nil)
			},
			expectedError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSecretsManager := mocks.NewMockapi(ctrl)
			sm := SecretsManager{
				secretsManager: mockSecretsManager,
			}
			tc.callMock(mockSecretsManager)

			// WHEN
			err := sm.DeleteSecret(tc.inSecretName)

			// THEN
			require.Equal(t, tc.expectedError, err)
		})
	}
}

func TestSecretsManager_DescribeSecret(t *testing.T) {
	mockTime := time.Now()
	mockSecretName := "github-token-backend-badgoose"
	mockError := errors.New("mockError")
	mockAPIOutput := &secretsmanager.DescribeSecretOutput{
		CreatedDate: awsv2.Time(mockTime),
		Name:        awsv2.String(mockSecretName),
		Tags:        []types.Tag{},
	}
	mockOutput := &DescribeSecretOutput{
		CreatedDate: awsv2.Time(mockTime),
		Name:        awsv2.String(mockSecretName),
		Tags:        []types.Tag{},
	}
	mockAwsErr := &types.ResourceNotFoundException{}

	tests := map[string]struct {
		inSecretName string
		callMock     func(m *mocks.Mockapi)

		expectedResp  *DescribeSecretOutput
		expectedError error
	}{
		"should wrap error returned by DescribeSecret": {
			inSecretName: mockSecretName,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSecret(gomock.Any(), &secretsmanager.DescribeSecretInput{
					SecretId: awsv2.String(mockSecretName),
				}).Return(nil, mockError)
			},
			expectedError: fmt.Errorf("describe secret %s: %w", mockSecretName, mockError),
		},

		"should return no error if secret is not found": {
			inSecretName: mockSecretName,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSecret(gomock.Any(), &secretsmanager.DescribeSecretInput{
					SecretId: awsv2.String(mockSecretName),
				}).Return(nil, mockAwsErr)
			},
			expectedError: &ErrSecretNotFound{
				secretName: mockSecretName,
				parentErr:  mockAwsErr,
			},
		},

		"should return no error if successful": {
			inSecretName: mockSecretName,
			callMock: func(m *mocks.Mockapi) {
				m.EXPECT().DescribeSecret(gomock.Any(), &secretsmanager.DescribeSecretInput{
					SecretId: awsv2.String(mockSecretName),
				}).Return(mockAPIOutput, nil)
			},
			expectedResp:  mockOutput,
			expectedError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSecretsManager := mocks.NewMockapi(ctrl)

			sm := SecretsManager{
				secretsManager: mockSecretsManager,
			}

			tc.callMock(mockSecretsManager)

			// WHEN
			oldSecretTags := secretTags
			defer func() { secretTags = oldSecretTags }()
			secretTags = func() []types.Tag {
				return []types.Tag{}
			}

			resp, err := sm.DescribeSecret(tc.inSecretName)

			// THEN
			if tc.expectedError != nil {
				require.EqualError(t, err, tc.expectedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedResp, resp)
			}
		})
	}
}

func TestSecretsManager_GetSecretValue(t *testing.T) {
	tests := map[string]struct {
		secretName string
		setupMock  func(m *mocks.Mockapi)

		want      string
		wantError string
	}{
		"error": {
			secretName: "asdf",
			setupMock: func(m *mocks.Mockapi) {
				m.EXPECT().GetSecretValue(gomock.Any(), &secretsmanager.GetSecretValueInput{
					SecretId: awsv2.String("asdf"),
				}).Return(nil, errors.New("some error"))
			},
			wantError: `get secret "asdf" from secrets manager: some error`,
		},
		"success": {
			secretName: "asdf",
			setupMock: func(m *mocks.Mockapi) {
				m.EXPECT().GetSecretValue(gomock.Any(), &secretsmanager.GetSecretValueInput{
					SecretId: awsv2.String("asdf"),
				}).Return(&secretsmanager.GetSecretValueOutput{
					SecretString: awsv2.String("hi"),
				}, nil)
			},
			want: "hi",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			api := mocks.NewMockapi(ctrl)
			tc.setupMock(api)

			sm := SecretsManager{
				secretsManager: api,
			}

			got, err := sm.GetSecretValue(context.Background(), tc.secretName)
			if tc.wantError != "" {
				require.EqualError(t, err, tc.wantError)
			}
			require.Equal(t, tc.want, got)
		})
	}
}
