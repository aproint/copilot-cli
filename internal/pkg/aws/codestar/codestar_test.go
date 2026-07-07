// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package codestar

import (
	"context"
	"errors"
	"testing"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codestarconnections"
	"github.com/aws/aws-sdk-go-v2/service/codestarconnections/types"

	"github.com/aproint/copilot-cli/internal/pkg/aws/codestar/mocks"

	"github.com/golang/mock/gomock"

	"github.com/stretchr/testify/require"
)

func TestCodestar_WaitUntilStatusAvailable(t *testing.T) {
	t.Run("times out if connection status not changed to available in allotted time", func(t *testing.T) {
		// GIVEN
		ctx, cancel := context.WithDeadline(context.Background(), time.Now())
		defer cancel()

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().GetConnection(gomock.Any(), gomock.Any()).Return(
			&codestarconnections.GetConnectionOutput{Connection: &types.Connection{
				ConnectionStatus: types.ConnectionStatusPending,
			},
			}, nil).AnyTimes()

		connection := &CodeStar{
			client: m,
		}
		connectionARN := "mockConnectionARN"

		// WHEN
		err := connection.WaitUntilConnectionStatusAvailable(ctx, connectionARN)

		// THEN
		require.EqualError(t, err, "timed out waiting for connection mockConnectionARN status to change from PENDING to AVAILABLE")
	})

	t.Run("returns a wrapped error on GetConnection call failure", func(t *testing.T) {
		// GIVEN
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().GetConnection(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))

		connection := &CodeStar{
			client: m,
		}
		connectionARN := "mockConnectionARN"

		// WHEN
		err := connection.WaitUntilConnectionStatusAvailable(context.Background(), connectionARN)

		// THEN
		require.EqualError(t, err, "get connection details for mockConnectionARN: some error")
	})

	t.Run("waits until connection status is returned as 'available' and exits gracefully", func(t *testing.T) {
		// GIVEN
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		m := mocks.NewMockapi(ctrl)
		connection := &CodeStar{
			client: m,
		}
		connectionARN := "mockConnectionARN"
		m.EXPECT().GetConnection(gomock.Any(), &codestarconnections.GetConnectionInput{
			ConnectionArn: awsv2.String(connectionARN),
		}).Return(
			&codestarconnections.GetConnectionOutput{Connection: &types.Connection{
				ConnectionStatus: types.ConnectionStatusAvailable,
			},
			}, nil)

		// WHEN
		err := connection.WaitUntilConnectionStatusAvailable(context.Background(), connectionARN)

		// THEN
		require.NoError(t, err)
	})
}

func TestCodeStar_GetConnectionARN(t *testing.T) {
	t.Run("returns wrapped error if ListConnections is unsuccessful", func(t *testing.T) {
		// GIVEN
		ctrl := gomock.NewController(t)
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().ListConnections(gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))

		connection := &CodeStar{
			client: m,
		}

		// WHEN
		ARN, err := connection.GetConnectionARN("someConnectionName")

		// THEN
		require.EqualError(t, err, "get list of connections in AWS account: some error")
		require.Equal(t, "", ARN)
	})

	t.Run("returns an error if no connections in the account match the one in the pipeline manifest", func(t *testing.T) {
		// GIVEN
		connectionName := "string cheese"
		ctrl := gomock.NewController(t)
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().ListConnections(gomock.Any(), gomock.Any()).Return(
			&codestarconnections.ListConnectionsOutput{
				Connections: []types.Connection{
					{ConnectionName: awsv2.String("gouda")},
					{ConnectionName: awsv2.String("fontina")},
					{ConnectionName: awsv2.String("brie")},
				},
			}, nil)

		connection := &CodeStar{
			client: m,
		}

		// WHEN
		ARN, err := connection.GetConnectionARN(connectionName)

		// THEN
		require.Equal(t, "", ARN)
		require.EqualError(t, err, "cannot find a connectionARN associated with string cheese")
	})

	t.Run("returns a match", func(t *testing.T) {
		// GIVEN
		connectionName := "string cheese"
		ctrl := gomock.NewController(t)
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().ListConnections(gomock.Any(), gomock.Any()).Return(
			&codestarconnections.ListConnectionsOutput{
				Connections: []types.Connection{
					{
						ConnectionName: awsv2.String("gouda"),
						ConnectionArn:  awsv2.String("notThisOne"),
					},
					{
						ConnectionName: awsv2.String("string cheese"),
						ConnectionArn:  awsv2.String("thisCheesyFakeARN"),
					},
					{
						ConnectionName: awsv2.String("fontina"),
						ConnectionArn:  awsv2.String("norThisOne"),
					},
				},
			}, nil)

		connection := &CodeStar{
			client: m,
		}

		// WHEN
		ARN, err := connection.GetConnectionARN(connectionName)

		// THEN
		require.Equal(t, "thisCheesyFakeARN", ARN)
		require.NoError(t, err)
	})

	t.Run("checks all connections and returns a match when paginated", func(t *testing.T) {
		// GIVEN
		connectionName := "string cheese"
		mockNextToken := "next"
		ctrl := gomock.NewController(t)
		m := mocks.NewMockapi(ctrl)
		m.EXPECT().ListConnections(gomock.Any(), gomock.Any()).Return(
			&codestarconnections.ListConnectionsOutput{
				Connections: []types.Connection{
					{
						ConnectionName: awsv2.String("gouda"),
						ConnectionArn:  awsv2.String("notThisOne"),
					},
					{
						ConnectionName: awsv2.String("fontina"),
						ConnectionArn:  awsv2.String("thisCheesyFakeARN"),
					},
				},
				NextToken: &mockNextToken,
			}, nil)
		m.EXPECT().ListConnections(gomock.Any(), &codestarconnections.ListConnectionsInput{
			NextToken: &mockNextToken,
		}).Return(
			&codestarconnections.ListConnectionsOutput{
				Connections: []types.Connection{
					{
						ConnectionName: awsv2.String("string cheese"),
						ConnectionArn:  awsv2.String("thisOne"),
					},
				},
			}, nil)

		connection := &CodeStar{
			client: m,
		}

		// WHEN
		ARN, err := connection.GetConnectionARN(connectionName)

		// THEN
		require.Equal(t, "thisOne", ARN)
		require.NoError(t, err)
	})
}
