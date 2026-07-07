// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package codepipeline

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/codepipeline/mocks"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type codepipelineMocks struct {
	cp *mocks.Mockapi
	rg *mocks.MockresourceGetter
}

func TestCodePipeline_GetPipeline(t *testing.T) {
	mockPipelineName := "pipeline-dinder-badgoose-repo"
	mockError := errors.New("mockError")
	mockTime := time.Now()
	mockArn := "arn:aws:codepipeline:us-west-2:1234567890:pipeline-dinder-badgoose-repo"
	mockSourceStage := types.StageDeclaration{
		Name: awsv2.String("Source"),
		Actions: []types.ActionDeclaration{
			{
				ActionTypeId: &types.ActionTypeId{
					Category: types.ActionCategorySource,
					Owner:    types.ActionOwnerThirdParty,
					Provider: awsv2.String("GitHub"),
					Version:  awsv2.String("1"),
				},
				Configuration: map[string]string{
					"Owner":      "badgoose",
					"Repo":       "repo",
					"Branch":     "main",
					"OAuthToken": "****",
				},
				Name: awsv2.String("SourceCodeFor-dinder"),
				OutputArtifacts: []types.OutputArtifact{
					{
						Name: awsv2.String("SCCheckoutArtifact"),
					},
				},
				RunOrder: awsv2.Int32(1),
			},
		},
	}
	mockBuildStage := types.StageDeclaration{
		Name: awsv2.String("Build"),
		Actions: []types.ActionDeclaration{
			{
				ActionTypeId: &types.ActionTypeId{
					Category: types.ActionCategoryBuild,
					Owner:    types.ActionOwnerAws,
					Provider: awsv2.String("CodeBuild"),
					Version:  awsv2.String("1"),
				},
				Configuration: map[string]string{
					"ProjectName": "pipeline-dinder-badgoose-repo-BuildProject",
				},
				InputArtifacts: []types.InputArtifact{
					{
						Name: awsv2.String("SCCheckoutArtifact"),
					},
				},
				Name: awsv2.String("Build"),
				OutputArtifacts: []types.OutputArtifact{
					{
						Name: awsv2.String("BuildOutput"),
					},
				},
				RunOrder: awsv2.Int32(1),
			},
		},
	}
	mockTestStage := types.StageDeclaration{
		Name: awsv2.String("DeployTo-test"),
		Actions: []types.ActionDeclaration{
			{
				ActionTypeId: &types.ActionTypeId{
					Category: types.ActionCategoryDeploy,
					Owner:    types.ActionOwnerAws,
					Provider: awsv2.String("CloudFormation"),
					Version:  awsv2.String("1"),
				},
				Configuration: map[string]string{
					"TemplatePath":          "BuildOutput::infrastructure/test.stack.yml",
					"ActionMode":            "CREATE_UPDATE",
					"Capabilities":          "CAPABILITY_NAMED_IAM",
					"ChangeSetName":         "dinder-test-test",
					"RoleArn":               "arn:aws:iam::1234567890:role/trivia-test-CFNExecutionRole",
					"StackName":             "dinder-test-test",
					"TemplateConfiguration": "BuildOutput::infrastructure/test-test.params.json",
				},
				InputArtifacts: []types.InputArtifact{
					{Name: awsv2.String("BuildOutput")},
				},
				Name:     awsv2.String("CreateOrUpdate-test-test"),
				Region:   awsv2.String("us-west-2"),
				RoleArn:  awsv2.String("arn:aws:iam::12344567890:role/dinder-test-EnvManagerRole"),
				RunOrder: awsv2.Int32(2),
			},
		},
	}
	mockStages := []types.StageDeclaration{mockSourceStage, mockBuildStage, mockTestStage}

	mockStageWithNoAction := types.StageDeclaration{
		Name:    awsv2.String("DummyStage"),
		Actions: []types.ActionDeclaration{},
	}
	mockOutput := &codepipeline.GetPipelineOutput{
		Pipeline: &types.PipelineDeclaration{
			Name:   awsv2.String(mockPipelineName),
			Stages: mockStages,
		},
		Metadata: &types.PipelineMetadata{
			Created:     &mockTime,
			Updated:     &mockTime,
			PipelineArn: awsv2.String(mockArn),
		},
	}

	tests := map[string]struct {
		inPipelineName string
		callMocks      func(m codepipelineMocks)

		expectedOut   *Pipeline
		expectedError error
	}{
		"happy path": {
			inPipelineName: mockPipelineName,
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().GetPipeline(gomock.Any(), &codepipeline.GetPipelineInput{
					Name: awsv2.String(mockPipelineName),
				}).Return(mockOutput, nil)

			},
			expectedOut: &Pipeline{
				Name:      mockPipelineName,
				Region:    "us-west-2",
				AccountID: "1234567890",
				Stages: []*Stage{
					{
						Name:     "Source",
						Category: "Source",
						Provider: "GitHub",
						Details:  "Repository: badgoose/repo",
					},
					{
						Name:     "Build",
						Category: "Build",
						Provider: "CodeBuild",
						Details:  "BuildProject: pipeline-dinder-badgoose-repo-BuildProject",
					},
					{
						Name:     "DeployTo-test",
						Category: "Deploy",
						Provider: "CloudFormation",
						Details:  "StackName: dinder-test-test",
					},
				},
				CreatedAt: mockTime,
				UpdatedAt: mockTime,
			},
			expectedError: nil,
		},
		"should only populate stage name if stage has no actions": {
			inPipelineName: mockPipelineName,
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().GetPipeline(gomock.Any(), &codepipeline.GetPipelineInput{
					Name: awsv2.String(mockPipelineName),
				}).Return(
					&codepipeline.GetPipelineOutput{
						Pipeline: &types.PipelineDeclaration{
							Name:   awsv2.String(mockPipelineName),
							Stages: []types.StageDeclaration{mockSourceStage, mockStageWithNoAction},
						},
						Metadata: &types.PipelineMetadata{
							Created:     &mockTime,
							Updated:     &mockTime,
							PipelineArn: awsv2.String(mockArn),
						},
					}, nil)

			},
			expectedOut: &Pipeline{
				Name:      mockPipelineName,
				Region:    "us-west-2",
				AccountID: "1234567890",
				Stages: []*Stage{
					{
						Name:     "Source",
						Category: "Source",
						Provider: "GitHub",
						Details:  "Repository: badgoose/repo",
					},
					{
						Name:     "DummyStage",
						Category: "",
						Provider: "",
						Details:  "",
					},
				},
				CreatedAt: mockTime,
				UpdatedAt: mockTime,
			},
			expectedError: nil,
		},
		"should wrap error from codepipeline client": {
			inPipelineName: mockPipelineName,
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().GetPipeline(gomock.Any(), &codepipeline.GetPipelineInput{
					Name: awsv2.String(mockPipelineName),
				}).Return(nil, mockError)

			},
			expectedOut:   nil,
			expectedError: fmt.Errorf("get pipeline %s: %w", mockPipelineName, mockError),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockapi(ctrl)
			mockrgClient := mocks.NewMockresourceGetter(ctrl)
			mocks := codepipelineMocks{
				cp: mockClient,
				rg: mockrgClient,
			}
			tc.callMocks(mocks)

			cp := CodePipeline{
				client:   mockClient,
				rgClient: mockrgClient,
			}

			// WHEN
			actualOut, err := cp.GetPipeline(tc.inPipelineName)

			// THEN
			require.Equal(t, tc.expectedError, err)
			require.Equal(t, tc.expectedOut, actualOut)
		})
	}
}

func TestCodePipeline_GetPipelineState(t *testing.T) {
	mockPipelineName := "pipeline-dinder-badgoose-repo"
	mockTime := time.Now()
	mockOutput := &codepipeline.GetPipelineStateOutput{
		PipelineName: awsv2.String(mockPipelineName),
		StageStates: []types.StageState{
			{
				ActionStates: []types.ActionState{
					{
						ActionName:      awsv2.String("action1"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusSucceeded},
					},
					{
						ActionName:      awsv2.String("action2"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusSucceeded},
					},
				},
				StageName: awsv2.String("Source"),
			},
			{
				InboundTransitionState: &types.TransitionState{Enabled: true},
				ActionStates: []types.ActionState{
					{
						ActionName:      awsv2.String("action1"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusFailed},
					},
					{
						ActionName:      awsv2.String("action2"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusInProgress},
					},
					{
						ActionName:      awsv2.String("action3"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusSucceeded},
					},
				},
				StageName: awsv2.String("Build"),
			},
			{
				InboundTransitionState: &types.TransitionState{Enabled: true},
				ActionStates: []types.ActionState{
					{
						ActionName:      awsv2.String("action1"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusSucceeded},
					},
					{
						ActionName:      awsv2.String("TestCommands"),
						LatestExecution: &types.ActionExecution{Status: types.ActionExecutionStatusFailed},
					},
				},
				StageName: awsv2.String("DeployTo-test"),
			},
			{
				InboundTransitionState: &types.TransitionState{Enabled: false},
				StageName:              awsv2.String("DeployTo-prod"),
			},
		},
		Updated: &mockTime,
	}
	mockError := errors.New("mockError")

	tests := map[string]struct {
		inPipelineName string
		callMocks      func(m codepipelineMocks)

		expectedOut   *PipelineState
		expectedError error
	}{
		"happy path": {
			inPipelineName: mockPipelineName,
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().GetPipelineState(gomock.Any(), &codepipeline.GetPipelineStateInput{
					Name: awsv2.String(mockPipelineName),
				}).Return(mockOutput, nil)

			},
			expectedOut: &PipelineState{
				PipelineName: mockPipelineName,
				StageStates: []*StageState{
					{
						StageName: "Source",
						Actions: []StageAction{
							{
								Name:   "action1",
								Status: "Succeeded",
							},
							{
								Name:   "action2",
								Status: "Succeeded",
							},
						},
						Transition: "",
					},
					{
						StageName: "Build",
						Actions: []StageAction{
							{
								Name:   "action1",
								Status: "Failed",
							},
							{
								Name:   "action2",
								Status: "InProgress",
							},
							{
								Name:   "action3",
								Status: "Succeeded",
							},
						},
						Transition: "ENABLED",
					},
					{
						StageName: "DeployTo-test",
						Actions: []StageAction{
							{
								Name:   "action1",
								Status: "Succeeded",
							},
							{
								Name:   "TestCommands",
								Status: "Failed",
							},
						},
						Transition: "ENABLED",
					},
					{
						StageName:  "DeployTo-prod",
						Transition: "DISABLED",
					},
				},
				UpdatedAt: mockTime,
			},
			expectedError: nil,
		},
		"should wrap error from CodePipeline client": {
			inPipelineName: mockPipelineName,
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().GetPipelineState(gomock.Any(), &codepipeline.GetPipelineStateInput{
					Name: awsv2.String(mockPipelineName),
				}).Return(nil, mockError)

			},
			expectedOut:   nil,
			expectedError: fmt.Errorf("get pipeline state %s: %w", mockPipelineName, mockError),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockapi(ctrl)

			mocks := codepipelineMocks{
				cp: mockClient,
			}
			tc.callMocks(mocks)

			cp := CodePipeline{
				client: mockClient,
			}

			// WHEN
			actualOut, err := cp.GetPipelineState(tc.inPipelineName)

			// THEN
			require.Equal(t, tc.expectedError, err)
			require.Equal(t, tc.expectedOut, actualOut)
		})
	}
}

func TestCodePipeline_RetryStageExecution(t *testing.T) {
	mockPipelineName := "pipeline-dinder-badgoose-repo"
	mockStageName := "Source"
	failedActions := types.StageRetryModeFailedActions
	notRetryable := &types.StageNotRetryableException{}
	mockPipelineExecutionID := awsv2.String("12345678-fake-exec-utio-nid987654321")
	mockBadOutput := &codepipeline.ListPipelineExecutionsOutput{
		PipelineExecutionSummaries: []types.PipelineExecutionSummary{},
	}
	mockErr := errors.New("some error")
	mockOutput := &codepipeline.RetryStageExecutionOutput{
		PipelineExecutionId: awsv2.String("12345678-fake-exec-utio-nid987654321"),
	}

	tests := map[string]struct {
		callMocks     func(m codepipelineMocks)
		expectedOut   *string
		expectedError error
	}{
		"returns nil when executes as expected": {
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().ListPipelineExecutions(
					gomock.Any(),
					&codepipeline.ListPipelineExecutionsInput{
						MaxResults:   awsv2.Int32(1),
						PipelineName: awsv2.String(mockPipelineName)}).Return(&codepipeline.ListPipelineExecutionsOutput{
					PipelineExecutionSummaries: []types.PipelineExecutionSummary{
						{
							PipelineExecutionId: awsv2.String("12345678-fake-exec-utio-nid987654321"),
						},
					},
				}, nil)
				m.cp.EXPECT().RetryStageExecution(
					gomock.Any(),
					&codepipeline.RetryStageExecutionInput{
						PipelineExecutionId: mockPipelineExecutionID,
						PipelineName:        awsv2.String(mockPipelineName),
						RetryMode:           failedActions,
						StageName:           awsv2.String(mockStageName),
					}).Return(mockOutput, nil)
			},
			expectedOut: nil,
		},
		"catches error and returns nil if pipeline succeeds before failing so not a 'retry'": {
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().ListPipelineExecutions(
					gomock.Any(),
					&codepipeline.ListPipelineExecutionsInput{
						MaxResults:   awsv2.Int32(1),
						PipelineName: awsv2.String(mockPipelineName)}).Return(&codepipeline.ListPipelineExecutionsOutput{
					PipelineExecutionSummaries: []types.PipelineExecutionSummary{
						{
							PipelineExecutionId: awsv2.String("12345678-fake-exec-utio-nid987654321"),
						},
					},
				}, nil)
				m.cp.EXPECT().RetryStageExecution(
					gomock.Any(),
					&codepipeline.RetryStageExecutionInput{
						PipelineExecutionId: mockPipelineExecutionID,
						PipelineName:        awsv2.String(mockPipelineName),
						RetryMode:           failedActions,
						StageName:           awsv2.String(mockStageName),
					}).Return(nil, notRetryable)
			},
			expectedOut: nil,
		},
		"returns wrapped error if ListPipelineExecutions fails": {
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().ListPipelineExecutions(
					gomock.Any(),
					&codepipeline.ListPipelineExecutionsInput{
						MaxResults:   awsv2.Int32(1),
						PipelineName: awsv2.String(mockPipelineName)}).Return(nil, mockErr)
				m.cp.EXPECT().RetryStageExecution(
					gomock.Any(),
					&codepipeline.RetryStageExecutionInput{
						PipelineExecutionId: mockPipelineExecutionID,
						PipelineName:        awsv2.String(mockPipelineName),
						RetryMode:           failedActions,
						StageName:           awsv2.String(mockStageName),
					}).Times(0)
			},
			expectedOut:   nil,
			expectedError: fmt.Errorf("retrieve pipeline execution ID: list pipeline execution for pipeline-dinder-badgoose-repo: some error"),
		},
		"returns wrapped error if no pipeline execution IDs are returned": {
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().ListPipelineExecutions(
					gomock.Any(),
					&codepipeline.ListPipelineExecutionsInput{
						MaxResults:   awsv2.Int32(1),
						PipelineName: awsv2.String(mockPipelineName)}).Return(mockBadOutput, nil)
				m.cp.EXPECT().RetryStageExecution(
					gomock.Any(),
					&codepipeline.RetryStageExecutionInput{
						PipelineExecutionId: mockPipelineExecutionID,
						PipelineName:        awsv2.String(mockPipelineName),
						RetryMode:           failedActions,
						StageName:           awsv2.String(mockStageName),
					}).Times(0)
			},
			expectedOut:   nil,
			expectedError: fmt.Errorf("retrieve pipeline execution ID: no pipeline execution IDs found for pipeline-dinder-badgoose-repo"),
		},
		"returns wrapped error if RetryStageExecution fails": {
			callMocks: func(m codepipelineMocks) {
				m.cp.EXPECT().ListPipelineExecutions(
					gomock.Any(),
					&codepipeline.ListPipelineExecutionsInput{
						MaxResults:   awsv2.Int32(1),
						PipelineName: awsv2.String(mockPipelineName)}).Return(&codepipeline.ListPipelineExecutionsOutput{
					PipelineExecutionSummaries: []types.PipelineExecutionSummary{
						{
							PipelineExecutionId: awsv2.String("12345678-fake-exec-utio-nid987654321"),
						},
					},
				}, nil)
				m.cp.EXPECT().RetryStageExecution(
					gomock.Any(),
					&codepipeline.RetryStageExecutionInput{
						PipelineExecutionId: mockPipelineExecutionID,
						PipelineName:        awsv2.String(mockPipelineName),
						RetryMode:           failedActions,
						StageName:           awsv2.String(mockStageName),
					}).Return(nil, mockErr)
			},
			expectedOut:   nil,
			expectedError: fmt.Errorf("retry pipeline source stage: some error"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mocks.NewMockapi(ctrl)
			mockrgClient := mocks.NewMockresourceGetter(ctrl)
			mocks := codepipelineMocks{
				cp: mockClient,
				rg: mockrgClient,
			}
			tc.callMocks(mocks)

			cp := CodePipeline{
				client:   mockClient,
				rgClient: mockrgClient,
			}

			// WHEN
			actualErr := cp.RetryStageExecution(mockPipelineName, mockStageName)

			// THEN
			if actualErr != nil {
				require.EqualError(t, actualErr, tc.expectedError.Error())
			}
		})
	}
}
