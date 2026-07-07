// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stackset

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation/stackset/mocks"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const testName = "stackset"

var testError = errors.New("some error")

func apiErr(code string) error {
	return &smithy.GenericAPIError{
		Code: code,
	}
}

func TestStackSet_Create(t *testing.T) {
	const (
		testTemplate           = "body"
		testDescription        = "amazing stack set"
		testExecutionRole      = "execARN"
		testAdministrationRole = "adminARN"
	)
	testTags := map[string]string{
		"owner": "boss",
	}
	testCases := map[string]struct {
		mockClient  func(ctrl *gomock.Controller) api
		wantedError error
	}{
		"succeeds if new stack set": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().CreateStackSet(gomock.Any(), &cloudformation.CreateStackSetInput{
					AdministrationRoleARN: aws.String(testAdministrationRole),
					Description:           aws.String(testDescription),
					ExecutionRoleName:     aws.String(testExecutionRole),
					StackSetName:          aws.String(testName),
					Tags: []types.Tag{
						{
							Key:   aws.String("owner"),
							Value: aws.String("boss"),
						},
					},
					TemplateBody: aws.String(testTemplate),
				})
				return m
			},
		},
		"succeeds if stack set already exists": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Return(nil, apiErr("NameAlreadyExistsException"))
				return m
			},
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().CreateStackSet(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("create stack set %s: %w", testName, testError),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			err := client.Create(testName, testTemplate,
				WithDescription(testDescription),
				WithAdministrationRoleARN(testAdministrationRole),
				WithExecutionRoleName(testExecutionRole),
				WithTags(testTags))

			// THEN
			require.Equal(t, tc.wantedError, err)
		})
	}
}

func TestStackSet_Describe(t *testing.T) {
	testCases := map[string]struct {
		mockClient  func(ctrl *gomock.Controller) api
		wantedDescr Description
		wantedError error
	}{
		"succeeds": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSet(gomock.Any(), &cloudformation.DescribeStackSetInput{
					StackSetName: aws.String(testName),
				}).Return(&cloudformation.DescribeStackSetOutput{
					StackSet: &types.StackSet{
						StackSetId:   aws.String(testName),
						StackSetName: aws.String(testName),
						TemplateBody: aws.String("body"),
					},
				}, nil)
				return m
			},
			wantedDescr: Description{
				ID:       testName,
				Name:     testName,
				Template: "body",
			},
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSet(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("describe stack set %s: %w", testName, testError),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			descr, err := client.Describe(testName)

			// THEN
			require.Equal(t, tc.wantedDescr, descr)
			require.Equal(t, tc.wantedError, err)
		})
	}
}

func TestStackSet_DescribeOperation(t *testing.T) {
	const testOpID = "1"
	testCases := map[string]struct {
		mockClient func(ctrl *gomock.Controller) api

		wantedOp    Operation
		wantedError error
	}{
		"returns the operation description on successful call": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), &cloudformation.DescribeStackSetOperationInput{
					StackSetName: aws.String(testName),
					OperationId:  aws.String(testOpID),
				}).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status:       types.StackSetOperationStatusStopped,
						StatusReason: aws.String("manually stopped"),
					},
				}, nil)
				return m
			},
			wantedOp: Operation{
				ID:     testOpID,
				Status: OpStatus(types.StackSetOperationStatusStopped),
				Reason: "manually stopped",
			},
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: errors.New("describe operation 1 for stack set stackset: some error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			op, err := client.DescribeOperation(testName, testOpID)

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedOp, op)
			}
		})
	}
}

func TestStackSet_Update(t *testing.T) {
	const (
		testTemplate           = "body"
		testDescription        = "amazing stack set"
		testExecutionRole      = "execARN"
		testAdministrationRole = "adminARN"
		testOperationID        = "2"
	)
	var (
		testTags = map[string]string{
			"owner": "boss",
		}
		errOpExists = apiErr("OperationIdAlreadyExistsException")
		errOpInProg = apiErr("OperationInProgressException")
		errOpStale  = apiErr("StaleRequestException")
	)

	testCases := map[string]struct {
		mockClient func(ctrl *gomock.Controller) api

		wantedOpID  string
		wantedError error
	}{
		"updates stack with operation is valid": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), &cloudformation.UpdateStackSetInput{
					OperationId:           aws.String(testOperationID),
					AdministrationRoleARN: aws.String(testAdministrationRole),
					Description:           aws.String(testDescription),
					ExecutionRoleName:     aws.String(testExecutionRole),
					StackSetName:          aws.String(testName),
					Tags: []types.Tag{
						{
							Key:   aws.String("owner"),
							Value: aws.String("boss"),
						},
					},
					OperationPreferences: &types.StackSetOperationPreferences{
						RegionConcurrencyType: types.RegionConcurrencyTypeParallel,
					},
					TemplateBody: aws.String(testTemplate),
				}).Return(&cloudformation.UpdateStackSetOutput{
					OperationId: aws.String(testOperationID),
				}, nil)
				return m
			},
			wantedOpID: testOperationID,
		},
		"returns ErrStackSetOutOfDate if operation exists already": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(nil, errOpExists)
				return m
			},
			wantedError: &ErrStackSetOutOfDate{
				name:      testName,
				parentErr: errOpExists,
			},
		},
		"returns ErrStackSetOutOfDate if operation in progress": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(nil, errOpInProg)
				return m
			},
			wantedError: &ErrStackSetOutOfDate{
				name:      testName,
				parentErr: errOpInProg,
			},
		},
		"returns ErrStackSetOutOfDate if operation is stale": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(nil, errOpStale)
				return m
			},
			wantedError: &ErrStackSetOutOfDate{
				name:      testName,
				parentErr: errOpStale,
			},
		},
		"wrap error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("update stack set %s: %w", testName, testError),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			opID, err := client.Update(testName, testTemplate,
				WithOperationID(testOperationID),
				WithDescription(testDescription),
				WithAdministrationRoleARN(testAdministrationRole),
				WithExecutionRoleName(testExecutionRole),
				WithTags(testTags))

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedOpID, opID)
			}
		})
	}
}

func TestStackSet_UpdateAndWait(t *testing.T) {
	const testTemplate = "body"
	testCases := map[string]struct {
		mockClient  func(ctrl *gomock.Controller) api
		wantedError error
	}{
		"waits until operation succeeds": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateStackSetOutput{
					OperationId: aws.String("1"),
				}, nil)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), &cloudformation.DescribeStackSetOperationInput{
					StackSetName: aws.String(testName),
					OperationId:  aws.String("1"),
				}).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatus(opStatusSucceeded),
					},
				}, nil)
				return m
			},
		},
		"returns a wrapped error if operation stopped": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateStackSetOutput{
					OperationId: aws.String("1"),
				}, nil)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatus(opStatusStopped),
					},
				}, nil)
				return m
			},
			wantedError: fmt.Errorf("operation %s for stack set %s was manually stopped", "1", testName),
		},
		"returns a wrapped error if operation failed": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().UpdateStackSet(gomock.Any(), gomock.Any()).Return(&cloudformation.UpdateStackSetOutput{
					OperationId: aws.String("1"),
				}, nil)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), gomock.Any()).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatus(opStatusFailed),
					},
				}, nil)
				return m
			},
			wantedError: fmt.Errorf("operation %s for stack set %s failed", "1", testName),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			err := client.UpdateAndWait(testName, testTemplate)

			// THEN
			require.Equal(t, tc.wantedError, err)
		})
	}
}

func TestStackSet_DeleteInstance(t *testing.T) {

	testCases := map[string]struct {
		mockClient   func(t *testing.T, ctrl *gomock.Controller) api
		inputAccount string
		inputRegion  string
		wantedOpID   string
		wantedError  error
	}{

		"returns if the account and region aren't found in the list of instances": {
			mockClient: func(t *testing.T, ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("some error"))
				return m
			},
			inputRegion:  "us-west-1",
			inputAccount: "1111",
			wantedError:  errors.New("delete stack instance in region us-west-1 for account 1111 for stackset stackset: some error"),
		},

		"successfully deletes stack instance and returns the operation ID": {
			mockClient: func(t *testing.T, ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *cloudformation.DeleteStackInstancesInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error) {
						require.Equal(t, testName, aws.ToString(in.StackSetName))
						require.ElementsMatch(t, []string{"1111"}, in.Accounts)
						require.ElementsMatch(t, []string{"us-east-1"}, in.Regions)
						require.False(t, aws.ToBool(in.RetainStacks))
						return &cloudformation.DeleteStackInstancesOutput{
							OperationId: aws.String("1"),
						}, nil
					})
				return m
			},
			wantedOpID:   "1",
			inputRegion:  "us-east-1",
			inputAccount: "1111",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(t, ctrl),
			}

			// WHEN
			opID, err := client.DeleteInstance(testName, tc.inputAccount, tc.inputRegion)

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedOpID, opID)
			}
		})
	}
}

func TestStackSet_WaitForStackSetLastOperationComplete(t *testing.T) {
	testCases := map[string]struct {
		mockClient  func(ctrl *gomock.Controller) api
		wantedError error
	}{
		"waits until operation succeeds": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackSetOperations(gomock.Any(), &cloudformation.ListStackSetOperationsInput{
					StackSetName: aws.String(testName),
				}).Return(&cloudformation.ListStackSetOperationsOutput{
					Summaries: []types.StackSetOperationSummary{
						{
							Status: types.StackSetOperationStatusRunning,
						},
					},
				}, nil)
				m.EXPECT().ListStackSetOperations(gomock.Any(), &cloudformation.ListStackSetOperationsInput{
					StackSetName: aws.String(testName),
				}).Return(&cloudformation.ListStackSetOperationsOutput{
					Summaries: []types.StackSetOperationSummary{
						{
							Status: types.StackSetOperationStatusSucceeded,
						},
					},
				}, nil)
				return m
			},
		},
		"return if no operation": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackSetOperations(gomock.Any(), &cloudformation.ListStackSetOperationsInput{
					StackSetName: aws.String(testName),
				}).Return(&cloudformation.ListStackSetOperationsOutput{
					Summaries: []types.StackSetOperationSummary{},
				}, nil)
				return m
			},
		},
		"error if fail to list stackset operation": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackSetOperations(gomock.Any(), &cloudformation.ListStackSetOperationsInput{
					StackSetName: aws.String(testName),
				}).Return(nil, errors.New("some error"))
				return m
			},
			wantedError: fmt.Errorf("list operations for stack set stackset: some error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			err := client.WaitForStackSetLastOperationComplete(testName)

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestStackSet_DeleteAllInstances(t *testing.T) {
	testCases := map[string]struct {
		mockClient func(t *testing.T, ctrl *gomock.Controller) api

		wantedOpID  string
		wantedError error
	}{
		"return ErrStackSetNotFound if the stack set does not exist": {
			mockClient: func(t *testing.T, ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(nil, apiErr("StackSetNotFoundException"))
				return m
			},
			wantedError: &ErrStackSetNotFound{name: testName},
		},
		"returns ErrStackSetInstancesNotFound if there are no stack set instances": {
			mockClient: func(t *testing.T, ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
					Summaries: []types.StackInstanceSummary{},
				}, nil)
				return m
			},
			wantedError: &ErrStackSetInstancesNotFound{name: testName},
		},
		"successfully deletes stack instances and returns the operation ID": {
			mockClient: func(t *testing.T, ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(&cloudformation.ListStackInstancesOutput{
					Summaries: []types.StackInstanceSummary{
						{
							Account: aws.String("1111"),
							Region:  aws.String("us-east-1"),
						},
						{
							Account: aws.String("2222"),
							Region:  aws.String("us-east-1"),
						},
						{
							Account: aws.String("1111"),
							Region:  aws.String("us-west-2"),
						},
					},
				}, nil)
				m.EXPECT().DeleteStackInstances(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in *cloudformation.DeleteStackInstancesInput, _ ...func(*cloudformation.Options)) (*cloudformation.DeleteStackInstancesOutput, error) {
						require.Equal(t, testName, aws.ToString(in.StackSetName))
						require.ElementsMatch(t, []string{"1111", "2222"}, in.Accounts)
						require.ElementsMatch(t, []string{"us-east-1", "us-west-2"}, in.Regions)
						require.False(t, aws.ToBool(in.RetainStacks))
						return &cloudformation.DeleteStackInstancesOutput{
							OperationId: aws.String("1"),
						}, nil
					})
				return m
			},
			wantedOpID: "1",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(t, ctrl),
			}

			// WHEN
			opID, err := client.DeleteAllInstances(testName)

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedOpID, opID)
			}
		})
	}
}

func TestStackSet_Delete(t *testing.T) {
	testCases := map[string]struct {
		mockClient  func(ctrl *gomock.Controller) api
		wantedError error
	}{
		"successfully exits if stack set does not exist": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Return(nil, apiErr("StackSetNotFoundException"))
				return m
			},
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DeleteStackSet(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("delete stack set %s: %w", testName, testError),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			err := client.Delete(testName)

			// THEN
			require.Equal(t, tc.wantedError, err)
		})
	}
}

func TestStackSet_CreateInstances(t *testing.T) {
	var (
		testAccounts = []string{"1234"}
		testRegions  = []string{"us-west-1"}
	)
	testCases := map[string]struct {
		mockClient func(ctrl *gomock.Controller) api

		wantedOpID  string
		wantedError error
	}{
		"successfully creates stack instances": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().CreateStackInstances(gomock.Any(), &cloudformation.CreateStackInstancesInput{
					StackSetName: aws.String(testName),
					Accounts:     testAccounts,
					Regions:      testRegions,
				}).Return(&cloudformation.CreateStackInstancesOutput{
					OperationId: aws.String("1"),
				}, nil)
				return m
			},
			wantedOpID: "1",
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().CreateStackInstances(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("create stack instances for stack set %s in regions %v for accounts %v: %w",
				testName,
				testRegions,
				testAccounts,
				testError),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			opID, err := client.CreateInstances(testName, testAccounts, testRegions)

			// THEN
			if tc.wantedError != nil {
				require.EqualError(t, err, tc.wantedError.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.wantedOpID, opID)
			}
		})
	}
}

func TestStackSet_InstanceSummaries(t *testing.T) {
	const (
		testAccountID = "1234"
		testRegion    = "us-west-1"
	)
	testCases := map[string]struct {
		mockClient      func(ctrl *gomock.Controller) api
		wantedSummaries []InstanceSummary
		wantedError     error
	}{
		"returns summaries": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackInstances(gomock.Any(), &cloudformation.ListStackInstancesInput{
					StackSetName:         aws.String(testName),
					StackInstanceAccount: aws.String(testAccountID),
					StackInstanceRegion:  aws.String(testRegion),
				}).Return(&cloudformation.ListStackInstancesOutput{
					Summaries: []types.StackInstanceSummary{
						{
							StackId: aws.String(testName),
							Account: aws.String(testAccountID),
							Region:  aws.String(testRegion),
						},
					},
				}, nil)
				return m
			},
			wantedSummaries: []InstanceSummary{
				{
					StackID: testName,
					Account: testAccountID,
					Region:  testRegion,
				},
			},
		},
		"wraps error on unexpected failure": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().ListStackInstances(gomock.Any(), gomock.Any()).Return(nil, testError)
				return m
			},
			wantedError: fmt.Errorf("list stack instances for stack set %s: %w", testName, testError),
		},
		"keeps iterating until there is no more next token": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				gomock.InOrder(
					m.EXPECT().ListStackInstances(gomock.Any(), &cloudformation.ListStackInstancesInput{
						StackSetName:         aws.String(testName),
						StackInstanceAccount: aws.String(testAccountID),
						StackInstanceRegion:  aws.String(testRegion),
					}).Return(&cloudformation.ListStackInstancesOutput{
						Summaries: []types.StackInstanceSummary{
							{
								StackId: aws.String("1111"),
								Account: aws.String(testAccountID),
								Region:  aws.String("us-west-2"),
								StackInstanceStatus: &types.StackInstanceComprehensiveStatus{
									DetailedStatus: types.StackInstanceDetailedStatus(instanceStatusRunning),
								},
							},
						},
						NextToken: aws.String("token"),
					}, nil),
					m.EXPECT().ListStackInstances(gomock.Any(), &cloudformation.ListStackInstancesInput{
						StackSetName:         aws.String(testName),
						StackInstanceAccount: aws.String(testAccountID),
						StackInstanceRegion:  aws.String(testRegion),
						NextToken:            aws.String("token"),
					}).Return(&cloudformation.ListStackInstancesOutput{
						Summaries: []types.StackInstanceSummary{
							{
								StackId: aws.String("2222"),
								Account: aws.String(testAccountID),
								Region:  aws.String("us-east-1"),
								StackInstanceStatus: &types.StackInstanceComprehensiveStatus{
									DetailedStatus: types.StackInstanceDetailedStatus(instanceStatusSucceeded),
								},
							},
						},
					}, nil),
				)

				return m
			},
			wantedSummaries: []InstanceSummary{
				{
					StackID: "1111",
					Account: testAccountID,
					Region:  "us-west-2",
					Status:  InstanceStatus(instanceStatusRunning),
				},
				{
					StackID: "2222",
					Account: testAccountID,
					Region:  "us-east-1",
					Status:  InstanceStatus(instanceStatusSucceeded),
				},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			summaries, err := client.InstanceSummaries(
				testName,
				FilterSummariesByAccountID(testAccountID),
				FilterSummariesByRegion(testRegion))

			// THEN
			require.ElementsMatch(t, tc.wantedSummaries, summaries)
			require.Equal(t, tc.wantedError, err)
		})
	}
}

func TestStackSet_WaitForOperation(t *testing.T) {
	const (
		testName = "demo-infrastructure"
		testOpID = "1"
	)
	testCases := map[string]struct {
		mockClient func(ctrl *gomock.Controller) api
		wantedErr  error
	}{
		"returns nil if the operation status is successful": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), &cloudformation.DescribeStackSetOperationInput{
					StackSetName: aws.String(testName),
					OperationId:  aws.String(testOpID),
				}).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatusSucceeded,
					},
				}, nil)
				return m
			},
		},
		"returns an error if the operation stopped": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), &cloudformation.DescribeStackSetOperationInput{
					StackSetName: aws.String(testName),
					OperationId:  aws.String(testOpID),
				}).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatusStopped,
					},
				}, nil)
				return m
			},
			wantedErr: errors.New("operation 1 for stack set demo-infrastructure was manually stopped"),
		},
		"returns an error if the operation failed": {
			mockClient: func(ctrl *gomock.Controller) api {
				m := mocks.NewMockapi(ctrl)
				m.EXPECT().DescribeStackSetOperation(gomock.Any(), &cloudformation.DescribeStackSetOperationInput{
					StackSetName: aws.String(testName),
					OperationId:  aws.String(testOpID),
				}).Return(&cloudformation.DescribeStackSetOperationOutput{
					StackSetOperation: &types.StackSetOperation{
						Status: types.StackSetOperationStatusFailed,
					},
				}, nil)
				return m
			},
			wantedErr: errors.New("operation 1 for stack set demo-infrastructure failed"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := StackSet{
				client: tc.mockClient(ctrl),
			}

			// WHEN
			err := client.WaitForOperation(testName, testOpID)

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
