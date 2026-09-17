// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package cloudformation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation"
	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation/cloudformationtest"
	"github.com/aproint/copilot-cli/internal/pkg/aws/cloudformation/stackset"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/mocks"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfn "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	awscfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCloudFormation_DeployApp(t *testing.T) {
	mockApp := &deploy.CreateAppInput{
		Name:      "testapp",
		AccountID: "1234",
		Version:   "v1.29.0",
	}
	testCases := map[string]struct {
		mockStack    func(ctrl *gomock.Controller) cfnClient
		mockStackSet func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		region       string
		want         error
	}{
		"should return an error if infrastructure roles stack fails": {
			mockStack: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", errors.New("error creating stack"))
				m.EXPECT().ErrorEvents(gomock.Any(), gomock.Any()).Return(nil, nil) // No additional error descriptions.
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				return nil
			},
			want: errors.New("error creating stack"),
		},
		"should return a wrapped error if region is invalid when populating the admin role arn": {
			region: "bad-region",
			mockStack: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", &cloudformation.ErrStackAlreadyExists{})
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				return nil
			},
			want: fmt.Errorf("get stack set administrator role arn: find the partition for region bad-region"),
		},
		"should return nil if there are no updates": {
			region: "us-west-2",
			mockStack: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", &cloudformation.ErrStackAlreadyExists{})
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
				return m
			},
		},
		"should return nil if infrastructure roles stackset created for the first time": {
			region: "us-west-2",
			mockStack: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any()).Return("", nil)
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil).
					Do(func(_ context.Context, name, _ string, _ ...stackset.CreateOrUpdateOption) {
						require.Equal(t, "testapp-infrastructure", name)
					})
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				cfnClient:   tc.mockStack(ctrl),
				appStackSet: tc.mockStackSet(t, ctrl),
				region:      tc.region,
				console:     new(discardFile),
			}

			// WHEN
			got := cf.DeployApp(t.Context(), mockApp)

			// THEN
			if tc.want != nil {
				require.EqualError(t, tc.want, got.Error())
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestCloudFormation_UpgradeApplication(t *testing.T) {
	testCases := map[string]struct {
		mockDeployer func(t *testing.T, ctrl *gomock.Controller) *CloudFormation

		wantedErr error
	}{
		"error if fail to get existing application infrastructure stack": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				return &CloudFormation{
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return nil, errors.New("some error")
						},
					},
				}
			},
			wantedErr: fmt.Errorf("get existing application infrastructure stack: some error"),
		},
		"error if fail to update app stack": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				return &CloudFormation{
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", fmt.Errorf("some error")
						},
					},
					renderStackSet: func(_ context.Context, input renderStackSetInput) error {
						return nil
					},
				}
			},
			wantedErr: fmt.Errorf(`upgrade stack "phonetool-infrastructure-roles": some error`),
		},
		// TODO test tags manually
		"error if fail to describe app change set": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				return &CloudFormation{
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", nil
						},
						DescribeChangeSetFn: func(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error) {
							return nil, errors.New("some error")
						},
					},
				}
			},
			wantedErr: fmt.Errorf(`upgrade stack "phonetool-infrastructure-roles": some error`),
		},
		"error if fail to get app change set template": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				return &CloudFormation{
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", nil
						},
						DescribeChangeSetFn: func(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error) {
							return &cloudformation.ChangeSetDescription{}, nil
						},
						TemplateBodyFromChangeSetFn: func(changeSetID, stackName string) (string, error) {
							return "", errors.New("some error")
						},
					},
				}
			},
			wantedErr: fmt.Errorf(`upgrade stack "phonetool-infrastructure-roles": some error`),
		},
		"error if fail to wait until stack set last operation complete": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				mockAppStackSet := mocks.NewMockstackSetClient(ctrl)
				mockAppStackSet.EXPECT().WaitForStackSetLastOperationComplete(gomock.Any(), "phonetool-infrastructure").Return(errors.New("some error"))

				return &CloudFormation{
					console: mockFileWriter{Writer: &strings.Builder{}},
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", nil
						},
						DescribeChangeSetFn: func(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error) {
							return &cloudformation.ChangeSetDescription{}, nil
						},
						TemplateBodyFromChangeSetFn: func(changeSetID, stackName string) (string, error) {
							return ``, nil
						},
						DescribeStackEventsFn: func(input *awscfn.DescribeStackEventsInput) (*awscfn.DescribeStackEventsOutput, error) {
							// just finish the renderer on the first Describe call
							return &awscfn.DescribeStackEventsOutput{
								StackEvents: []awscfntypes.StackEvent{
									{
										Timestamp:         aws.Time(time.Now().Add(1 * time.Hour)),
										LogicalResourceId: aws.String("phonetool-infrastructure-roles"),
										ResourceStatus:    awscfntypes.ResourceStatusUpdateComplete,
									},
								},
							}, nil
						},
					},
					appStackSet: mockAppStackSet,
				}
			},
			wantedErr: fmt.Errorf(`wait for stack set phonetool-infrastructure last operation complete: some error`),
		},
		"success": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				mockAppStackSet := mocks.NewMockstackSetClient(ctrl)
				mockAppStackSet.EXPECT().WaitForStackSetLastOperationComplete(gomock.Any(), "phonetool-infrastructure").Return(nil)
				mockAppStackSet.EXPECT().Describe(gomock.Any(), "phonetool-infrastructure").Return(stackset.Description{}, nil)
				mockAppStackSet.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil)

				return &CloudFormation{
					console: mockFileWriter{Writer: &strings.Builder{}},
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", nil
						},
						DescribeChangeSetFn: func(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error) {
							return &cloudformation.ChangeSetDescription{}, nil
						},
						TemplateBodyFromChangeSetFn: func(changeSetID, stackName string) (string, error) {
							return ``, nil
						},
						DescribeStackEventsFn: func(input *awscfn.DescribeStackEventsInput) (*awscfn.DescribeStackEventsOutput, error) {
							return &awscfn.DescribeStackEventsOutput{
								StackEvents: []awscfntypes.StackEvent{
									{
										Timestamp:         aws.Time(time.Now().Add(1 * time.Hour)),
										LogicalResourceId: aws.String("phonetool-infrastructure-roles"),
										ResourceStatus:    awscfntypes.ResourceStatusUpdateComplete,
									},
								},
							}, nil
						},
					},
					appStackSet: mockAppStackSet,
					region:      "us-west-2",
					renderStackSet: func(_ context.Context, input renderStackSetInput) error {
						_, err := input.createOpFn(t.Context())
						return err
					},
				}
			},
		},
		"success with multiple tries and waitings": {
			mockDeployer: func(t *testing.T, ctrl *gomock.Controller) *CloudFormation {
				mockAppStackSet := mocks.NewMockstackSetClient(ctrl)
				mockAppStackSet.EXPECT().WaitForStackSetLastOperationComplete(gomock.Any(), "phonetool-infrastructure").Return(nil)
				mockAppStackSet.EXPECT().Describe(gomock.Any(), "phonetool-infrastructure").Return(stackset.Description{}, nil)
				mockAppStackSet.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", &stackset.ErrStackSetOutOfDate{})
				mockAppStackSet.EXPECT().WaitForStackSetLastOperationComplete(gomock.Any(), "phonetool-infrastructure").Return(nil)
				mockAppStackSet.EXPECT().Describe(gomock.Any(), "phonetool-infrastructure").Return(stackset.Description{}, nil)
				mockAppStackSet.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil)

				return &CloudFormation{
					console: mockFileWriter{Writer: &strings.Builder{}},
					cfnClient: &cloudformationtest.Double{
						DescribeFn: func(string) (*cloudformation.StackDescription, error) {
							return &cloudformation.StackDescription{}, nil
						},
						UpdateFn: func(*cloudformation.Stack) (string, error) {
							return "", nil
						},
						DescribeChangeSetFn: func(changeSetID, stackName string) (*cloudformation.ChangeSetDescription, error) {
							return &cloudformation.ChangeSetDescription{}, nil
						},
						TemplateBodyFromChangeSetFn: func(changeSetID, stackName string) (string, error) {
							return ``, nil
						},
						DescribeStackEventsFn: func(input *awscfn.DescribeStackEventsInput) (*awscfn.DescribeStackEventsOutput, error) {
							return &awscfn.DescribeStackEventsOutput{
								StackEvents: []awscfntypes.StackEvent{
									{
										Timestamp:         aws.Time(time.Now().Add(1 * time.Hour)),
										LogicalResourceId: aws.String("phonetool-infrastructure-roles"),
										ResourceStatus:    awscfntypes.ResourceStatusUpdateComplete,
									},
								},
							}, nil
						},
					},
					appStackSet: mockAppStackSet,
					region:      "us-west-2",
					renderStackSet: func(_ context.Context, input renderStackSetInput) error {
						_, err := input.createOpFn(t.Context())
						return err
					},
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := tc.mockDeployer(t, ctrl)

			// WHEN
			err := cf.UpgradeApplication(t.Context(), &deploy.CreateAppInput{
				Name: "phonetool",
			})

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCloudFormation_AddEnvToApp(t *testing.T) {
	mockApp := config.Application{
		Name:      "testapp",
		AccountID: "1234",
	}
	testCases := map[string]struct {
		mockStackSet func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		app          *config.Application
		env          *config.Environment
		want         error
	}{
		"with no existing deployments and adding an env": {
			app: &mockApp,
			env: &config.Environment{Name: "test", AccountID: "1234", Region: "us-west-2"},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				body, err := yaml.Marshal(stack.DeployedAppMetadata{})
				require.NoError(t, err)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: string(body),
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, _ string, ops ...stackset.CreateOrUpdateOption) {
						actual := &awscfn.UpdateStackSetInput{}
						ops[0](actual)
						wanted := &awscfn.UpdateStackSetInput{}
						stackset.WithOperationID("1")(wanted)
						require.Equal(t, actual, wanted)
					})
				m.EXPECT().InstanceSummaries(context.Background(), gomock.Any()).Return([]stackset.InstanceSummary{}, nil)
				m.EXPECT().CreateInstances(gomock.Any(), gomock.Any(), []string{"1234"}, []string{"us-west-2"}).Return("", nil)
				return m
			},
		},
		"with no new account ID added": {
			app: &mockApp,
			env: &config.Environment{Name: "test", AccountID: "1234", Region: "us-west-2"},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				body, err := yaml.Marshal(stack.DeployedAppMetadata{
					Metadata: stack.AppResources{
						AppResourcesConfig: stack.AppResourcesConfig{
							Accounts: []string{"1234"},
						},
					},
				})
				require.NoError(t, err)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: string(body),
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil)
				m.EXPECT().InstanceSummaries(context.Background(), gomock.Any()).Return([]stackset.InstanceSummary{}, nil)
				m.EXPECT().CreateInstances(gomock.Any(), gomock.Any(), []string{"1234"}, []string{"us-west-2"}).Return("", nil)
				return m
			},
		},
		"with existing stack instances in same region but different account (no new stack instances, but update stackset)": {
			app: &mockApp,
			env: &config.Environment{Name: "test", AccountID: "1234", Region: "us-west-2"},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				body, err := yaml.Marshal(stack.DeployedAppMetadata{
					Metadata: stack.AppResources{
						AppResourcesConfig: stack.AppResourcesConfig{
							Accounts: []string{"1234"},
							Version:  1,
						},
					},
				})
				require.NoError(t, err)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: string(body),
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, _ string, ops ...stackset.CreateOrUpdateOption) {
						actual := &awscfn.UpdateStackSetInput{}
						ops[0](actual)
						wanted := &awscfn.UpdateStackSetInput{}
						stackset.WithOperationID("2")(wanted)
						require.Equal(t, actual, wanted)
					})
				m.EXPECT().InstanceSummaries(context.Background(), gomock.Any()).Return([]stackset.InstanceSummary{
					{
						Region:  "us-west-2",
						Account: "1234",
					},
				}, nil)
				m.EXPECT().CreateInstances(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				appStackSet: tc.mockStackSet(t, ctrl),
				region:      "us-west-2",
				renderStackSet: func(_ context.Context, input renderStackSetInput) error {
					_, err := input.createOpFn(t.Context())
					return err
				},
			}
			got := cf.AddEnvToApp(context.Background(), &AddEnvToAppOpts{
				App:          tc.app,
				EnvName:      tc.env.Name,
				EnvAccountID: tc.env.AccountID,
				EnvRegion:    tc.env.Region,
			})

			if tc.want != nil {
				require.EqualError(t, got, tc.want.Error())
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestCloudFormation_AddPipelineResourcesToApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")
	stackSet := mocks.NewMockstackSetClient(ctrl)
	wantedErr := errors.New("describe stack set")
	stackSet.EXPECT().Describe(ctx, "testapp-infrastructure").Return(stackset.Description{}, wantedErr)
	cf := CloudFormation{appStackSet: stackSet}

	err := cf.AddPipelineResourcesToApp(ctx, &config.Application{
		Name:      "testapp",
		AccountID: "1234",
	}, "us-west-2")

	require.ErrorIs(t, err, wantedErr)
}

func TestCloudFormation_AddServiceToApp(t *testing.T) {
	mockApp := config.Application{
		Name:      "testapp",
		AccountID: "1234",
	}
	testCases := map[string]struct {
		app          *config.Application
		svcName      string
		mockStackSet func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		want         error
	}{
		"with no existing deployments and adding a service": {
			app:     &mockApp,
			svcName: "TestSvc",
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: `Metadata:
  Version:
  Services: []`,
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, template string, _ ...stackset.CreateOrUpdateOption) {
						configToDeploy, err := stack.AppConfigFrom(&template)
						require.NoError(t, err)
						require.ElementsMatch(t, []stack.AppResourcesWorkload{{Name: "TestSvc", WithECR: true}}, configToDeploy.Workloads)
						require.Empty(t, configToDeploy.Accounts, "there should be no new accounts to deploy")
						require.Equal(t, 1, configToDeploy.Version)
					})
				return m
			},
		},
		"with new app to existing app with existing services": {
			app:     &mockApp,
			svcName: "test",
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: `Metadata:
  Version: 1
  Services:
  - firsttest`,
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, template string, _ ...stackset.CreateOrUpdateOption) {
						configToDeploy, err := stack.AppConfigFrom(&template)
						require.NoError(t, err)
						require.ElementsMatch(t, []stack.AppResourcesWorkload{
							{Name: "test", WithECR: true},
							{Name: "firsttest", WithECR: true},
						}, configToDeploy.Workloads)
						require.Empty(t, configToDeploy.Accounts, "there should be no new accounts to deploy")
						require.Equal(t, 2, configToDeploy.Version)

					})
				return m
			},
		},
		"with existing service to existing app with existing services": {
			app:     &mockApp,
			svcName: "test",
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: `Metadata:
  Version: 1
  Services:
  - test`,
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
				return m
			},
		},
		"with new app to existing app with existing Workloads": {
			app:     &mockApp,
			svcName: "test",
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: `Metadata:
  Version: 1
  Services: "See #5140"
  Workloads:
  - Name: firsttest
    WithECR: true`,
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, template string, _ ...stackset.CreateOrUpdateOption) {
						configToDeploy, err := stack.AppConfigFrom(&template)
						require.NoError(t, err)
						require.ElementsMatch(t, []stack.AppResourcesWorkload{
							{Name: "test", WithECR: true},
							{Name: "firsttest", WithECR: true},
						}, configToDeploy.Workloads)
						require.Empty(t, configToDeploy.Accounts, "there should be no new accounts to deploy")
						require.Equal(t, 2, configToDeploy.Version)

					})
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				appStackSet: tc.mockStackSet(t, ctrl),
				region:      "us-west-2",
				renderStackSet: func(_ context.Context, input renderStackSetInput) error {
					_, err := input.createOpFn(t.Context())
					return err
				},
			}

			got := cf.AddServiceToApp(context.Background(), tc.app, tc.svcName)

			if tc.want != nil {
				require.EqualError(t, got, tc.want.Error())
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func TestCloudFormation_RemoveServiceFromApp(t *testing.T) {
	mockApp := &config.Application{
		Name:      "testapp",
		AccountID: "1234",
	}

	tests := map[string]struct {
		service      string
		mockStackSet func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		want         error
	}{
		"should remove input service from the stack set": {
			service: "test",

			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(stackset.Description{
					Template: `Metadata:
  Version: 1
  Services:
  - firsttest
  - test`,
				}, nil)
				m.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return("", nil).
					Do(func(_ context.Context, _, template string, opts ...stackset.CreateOrUpdateOption) {
						configToDeploy, err := stack.AppConfigFrom(&template)
						require.NoError(t, err)
						require.ElementsMatch(t, []stack.AppResourcesWorkload{{Name: "firsttest", WithECR: true}}, configToDeploy.Workloads)
						require.Empty(t, configToDeploy.Accounts, "config account list should be empty")
						require.Equal(t, 2, configToDeploy.Version)
						require.Equal(t, 5, len(opts))
					})
				return m
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				appStackSet: tc.mockStackSet(t, ctrl),
				region:      "us-west-2",
				renderStackSet: func(_ context.Context, input renderStackSetInput) error {
					_, err := input.createOpFn(t.Context())
					return err
				},
			}

			got := cf.RemoveServiceFromApp(context.Background(), mockApp, tc.service)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestCloudFormation_GetRegionalAppResources(t *testing.T) {
	mockApp := config.Application{Name: "app", AccountID: "12345"}

	testCases := map[string]struct {
		createRegionalMockClient func(ctrl *gomock.Controller) cfnClient
		mockStackSet             func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		wantedResource           stack.AppRegionalResources
		want                     error
	}{
		"should describe stack instances and convert to AppRegionalResources": {
			wantedResource: stack.AppRegionalResources{
				KMSKeyARN:      "arn:aws:kms:us-west-2:01234567890:key/0000",
				S3Bucket:       "tests3-bucket-us-west-2",
				Region:         "us-east-9",
				RepositoryURLs: map[string]string{"phonetool-svc": "123.dkr.ecr.us-west-2.amazonaws.com/phonetool-svc"},
			},
			createRegionalMockClient: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), "cross-region-stack").Return(mockValidAppResourceStack(), nil)
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]stackset.InstanceSummary{
						{
							StackID: "cross-region-stack",
							Region:  "us-east-9",
						},
					}, nil).
					Do(func(_ context.Context, _ string, opt stackset.InstanceSummariesOption) {
						wanted := &awscfn.ListStackInstancesInput{
							StackInstanceAccount: aws.String("12345"),
						}
						actual := &awscfn.ListStackInstancesInput{}
						opt(actual)
						require.Equal(t, wanted, actual)
					})
				return m
			},
		},
		"should propagate describe errors": {
			want: fmt.Errorf("describing application resources: getting outputs for stack cross-region-stack in region us-east-9: error calling cloudformation"),
			createRegionalMockClient: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), "cross-region-stack").Return(nil, errors.New("error calling cloudformation"))
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).Return([]stackset.InstanceSummary{
					{
						StackID: "cross-region-stack",
						Region:  "us-east-9",
					},
				}, nil)
				return m
			},
		},

		"should propagate list stack instances errors": {
			want: fmt.Errorf("describing application resources: error"),
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("error"))
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				regionalClient: func(region string) cfnClient {
					return tc.createRegionalMockClient(ctrl)
				},
				appStackSet: tc.mockStackSet(t, ctrl),
			}

			// WHEN
			got, err := cf.GetRegionalAppResources(context.Background(), &mockApp)

			// THEN
			if tc.want != nil {
				require.Error(t, err)
				require.EqualError(t, err, tc.want.Error())
			} else {
				require.True(t, len(got) == 1, "Expected only one resource")
				// Assert that the application resources are the same.
				require.Equal(t, tc.wantedResource, *got[0])
			}
		})
	}
}

func TestCloudFormation_GetAppResourcesByRegion(t *testing.T) {
	mockApp := config.Application{Name: "app", AccountID: "12345"}

	testCases := map[string]struct {
		createRegionalMockClient func(ctrl *gomock.Controller) cfnClient
		mockStackSet             func(t *testing.T, ctrl *gomock.Controller) stackSetClient
		wantedResource           stack.AppRegionalResources
		region                   string
		want                     error
	}{
		"should describe stack instances and convert to AppRegionalResources": {
			wantedResource: stack.AppRegionalResources{
				KMSKeyARN:      "arn:aws:kms:us-west-2:01234567890:key/0000",
				S3Bucket:       "tests3-bucket-us-west-2",
				Region:         "us-east-9",
				RepositoryURLs: map[string]string{"phonetool-svc": "123.dkr.ecr.us-west-2.amazonaws.com/phonetool-svc"},
			},
			region: "us-east-9",
			createRegionalMockClient: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), "cross-region-stack").Return(mockValidAppResourceStack(), nil)
				return m
			},
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]stackset.InstanceSummary{
						{
							StackID: "cross-region-stack",
							Region:  "us-east-9",
						},
					}, nil).
					Do(func(_ context.Context, _ string, opts ...stackset.InstanceSummariesOption) {
						wanted := &awscfn.ListStackInstancesInput{
							StackInstanceAccount: aws.String("12345"),
							StackInstanceRegion:  aws.String("us-east-9"),
						}
						actual := &awscfn.ListStackInstancesInput{}
						optAcc, optRegion := opts[0], opts[1]
						optAcc(actual)
						optRegion(actual)
						require.Equal(t, wanted, actual)
					})
				return m
			},
		},
		"should error when resources are found": {
			want:   fmt.Errorf("no regional resources for application app in region us-east-9 found"),
			region: "us-east-9",
			mockStackSet: func(t *testing.T, ctrl *gomock.Controller) stackSetClient {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).Return([]stackset.InstanceSummary{}, nil)
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				regionalClient: func(region string) cfnClient {
					return tc.createRegionalMockClient(ctrl)
				},
				appStackSet: tc.mockStackSet(t, ctrl),
			}

			// WHEN
			got, err := cf.GetAppResourcesByRegion(context.Background(), &mockApp, tc.region)

			// THEN
			if tc.want != nil {
				require.Error(t, err)
				require.EqualError(t, err, tc.want.Error())
			} else {
				require.NotNil(t, got)
				// Assert that the application resources are the same.
				require.Equal(t, tc.wantedResource, *got)
			}
		})
	}
}

func TestCloudFormation_GetAppResources(t *testing.T) {
	app := &config.Application{Name: "app", AccountID: "12345"}
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")

	t.Run("one region", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		stackSet := mocks.NewMockstackSetClient(ctrl)
		regional := mocks.NewMockcfnClient(ctrl)
		stackSet.EXPECT().InstanceSummaries(ctx, "app-infrastructure", gomock.Any(), gomock.Any()).Return(
			[]stackset.InstanceSummary{{StackID: "cross-region-stack", Region: "us-east-9"}}, nil,
		)
		regional.EXPECT().Describe(ctx, "cross-region-stack").Return(mockValidAppResourceStack(), nil)
		cf := CloudFormation{
			appStackSet: stackSet,
			regionalClient: func(string) cfnClient {
				return regional
			},
		}

		got, err := cf.GetAppResourcesByRegion(ctx, app, "us-east-9")

		require.NoError(t, err)
		require.Equal(t, "us-east-9", got.Region)
	})

	t.Run("all regions", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		stackSet := mocks.NewMockstackSetClient(ctrl)
		regional := mocks.NewMockcfnClient(ctrl)
		stackSet.EXPECT().InstanceSummaries(ctx, "app-infrastructure", gomock.Any()).Return(
			[]stackset.InstanceSummary{{StackID: "cross-region-stack", Region: "us-west-2"}}, nil,
		)
		regional.EXPECT().Describe(ctx, "cross-region-stack").Return(mockValidAppResourceStack(), nil)
		cf := CloudFormation{
			appStackSet: stackSet,
			regionalClient: func(string) cfnClient {
				return regional
			},
		}

		got, err := cf.GetRegionalAppResources(ctx, app)

		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "us-west-2", got[0].Region)
	})
}

func TestCloudFormation_DelegateDNSPermissions(t *testing.T) {
	testCases := map[string]struct {
		app        *config.Application
		accountID  string
		createMock func(ctrl *gomock.Controller) cfnClient
		want       error
	}{
		"Calls Update Stack": {
			app: &config.Application{
				AccountID: "1234",
				Name:      "app",
				Domain:    "amazon.com",
			},
			createMock: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(mockAppRolesStack("stackname", map[string]string{
					"AppDNSDelegatedAccounts": "1234",
				}), nil)
				m.EXPECT().UpdateAndWait(context.Background(), gomock.Any()).Return(nil)
				return m
			},
		},

		"Returns error from Describe Stack": {
			app: &config.Application{
				AccountID: "1234",
				Name:      "app",
				Domain:    "amazon.com",
			},
			want: fmt.Errorf("getting existing application infrastructure stack: error"),
			createMock: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(nil, errors.New("error"))
				return m
			},
		},
		"Returns nil if there are no changeset updates from deployChangeSet": {
			app: &config.Application{
				AccountID: "1234",
				Name:      "app",
				Domain:    "amazon.com",
			},
			want: nil,
			createMock: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(mockAppRolesStack("stackname", map[string]string{
					"AppDNSDelegatedAccounts": "1234",
				}), nil)
				m.EXPECT().UpdateAndWait(context.Background(), gomock.Any()).Return(&cloudformation.ErrChangeSetEmpty{})
				return m
			},
		},
		"Returns error from Update Stack": {
			app: &config.Application{
				AccountID: "1234",
				Name:      "app",
				Domain:    "amazon.com",
			},
			want: fmt.Errorf("updating application to allow DNS delegation: error"),
			createMock: func(ctrl *gomock.Controller) cfnClient {
				m := mocks.NewMockcfnClient(ctrl)
				m.EXPECT().Describe(context.Background(), gomock.Any()).Return(mockAppRolesStack("stackname", map[string]string{
					"AppDNSDelegatedAccounts": "1234",
				}), nil)
				m.EXPECT().UpdateAndWait(context.Background(), gomock.Any()).Return(errors.New("error"))
				return m
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			cf := CloudFormation{
				cfnClient: tc.createMock(ctrl),
			}

			// WHEN
			got := cf.DelegateDNSPermissions(context.Background(), tc.app, tc.accountID)

			// THEN
			if tc.want != nil {
				require.EqualError(t, tc.want, got.Error())
			} else {
				require.NoError(t, got)
			}
		})
	}
}

func mockValidAppResourceStack() *cloudformation.StackDescription {
	return mockAppResourceStack("stack", map[string]string{
		"KMSKeyARN":               "arn:aws:kms:us-west-2:01234567890:key/0000",
		"PipelineBucket":          "tests3-bucket-us-west-2",
		"ECRRepophonetoolDASHsvc": "arn:aws:ecr:us-west-2:123:repository/phonetool-svc",
	})
}

func mockAppResourceStack(stackArn string, outputs map[string]string) *cloudformation.StackDescription {
	outputList := []awscfntypes.Output{}
	for key, val := range outputs {
		outputList = append(outputList, awscfntypes.Output{
			OutputKey:   aws.String(key),
			OutputValue: aws.String(val),
		})
	}

	return &cloudformation.StackDescription{
		StackId: aws.String(stackArn),
		Outputs: outputList,
	}
}

func mockAppRolesStack(stackArn string, parameters map[string]string) *cloudformation.StackDescription {
	parametersList := []awscfntypes.Parameter{}
	for key, val := range parameters {
		parametersList = append(parametersList, awscfntypes.Parameter{
			ParameterKey:   aws.String(key),
			ParameterValue: aws.String(val),
		})
	}

	return &cloudformation.StackDescription{
		StackId:     aws.String(stackArn),
		StackStatus: awscfntypes.StackStatusUpdateComplete,
		Parameters:  parametersList,
	}
}

func TestCloudFormation_DeleteApp(t *testing.T) {
	type contextKey string
	const key contextKey = "caller"
	ctx := context.WithValue(context.Background(), key, "delete app")
	ctrl := gomock.NewController(t)
	client := mocks.NewMockcfnClient(ctrl)
	stackSet := mocks.NewMockstackSetClient(ctrl)
	stackSet.EXPECT().DeleteAllInstances(ctx, "testApp-infrastructure").Return("1", nil)
	stackSet.EXPECT().WaitForOperation(ctx, "testApp-infrastructure", "1").Return(nil)
	stackSet.EXPECT().Delete(ctx, "testApp-infrastructure").Return(nil)
	client.EXPECT().TemplateBody(ctx, "testApp-infrastructure-roles").Return("", nil)
	client.EXPECT().Describe(ctx, "testApp-infrastructure-roles").Return(&cloudformation.StackDescription{
		StackId: aws.String("some stack"),
	}, nil)
	client.EXPECT().DeleteAndWait(gomock.Any(), "testApp-infrastructure-roles").DoAndReturn(
		func(gotCtx context.Context, _ string) error {
			require.Equal(t, "delete app", gotCtx.Value(key))
			return &cloudformation.ErrStackNotFound{}
		},
	)
	client.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).DoAndReturn(
		func(gotCtx context.Context, _ *awscfn.DescribeStackEventsInput) (*awscfn.DescribeStackEventsOutput, error) {
			require.Equal(t, "delete app", gotCtx.Value(key))
			return &awscfn.DescribeStackEventsOutput{}, nil
		},
	).AnyTimes()
	cf := CloudFormation{
		cfnClient:   client,
		appStackSet: stackSet,
		console:     new(discardFile),
	}

	err := cf.DeleteApp(ctx, "testApp")

	require.NoError(t, err)
}

func TestCloudFormation_RemoveEnvFromApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.WithValue(context.Background(), struct{}{}, "caller")
	stackSet := mocks.NewMockstackSetClient(ctrl)
	wantedErr := errors.New("list stack instances")
	stackSet.EXPECT().InstanceSummaries(ctx, "app-infrastructure", gomock.Any(), gomock.Any()).Return(nil, wantedErr)
	cf := CloudFormation{appStackSet: stackSet}
	env := &config.Environment{Name: "test", AccountID: "12345", Region: "us-west-2"}

	err := cf.RemoveEnvFromApp(ctx, &RemoveEnvFromAppOpts{
		App:          &config.Application{Name: "app", AccountID: "12345"},
		EnvToDelete:  env,
		Environments: []*config.Environment{env},
	})

	require.ErrorIs(t, err, wantedErr)
}

func TestCloudFormation_RenderStackSet(t *testing.T) {
	testDate := time.Date(2020, time.November, 23, 18, 0, 0, 0, time.UTC)
	testCases := map[string]struct {
		in   renderStackSetInput
		mock func(t *testing.T, ctrl *gomock.Controller) CloudFormation

		wantedErr error
	}{
		"should return the error if a stack set operation cannot be created": {
			in: renderStackSetInput{
				hasInstanceUpdates: true,
				createOpFn: func(context.Context) (string, error) {
					return "", errors.New("some error")
				},
				now: func() time.Time {
					return testDate
				},
			},
			mock: func(t *testing.T, ctrl *gomock.Controller) CloudFormation {
				return CloudFormation{}
			},

			wantedErr: errors.New("some error"),
		},
		"should return a wrapped error if stack set instance streamers cannot be retrieved": {
			in: renderStackSetInput{
				name:               "demo-infra",
				hasInstanceUpdates: true,
				createOpFn: func(context.Context) (string, error) {
					return "1", nil
				},
				now: func() time.Time {
					return testDate
				},
			},
			mock: func(t *testing.T, ctrl *gomock.Controller) CloudFormation {
				m := mocks.NewMockstackSetClient(ctrl)
				m.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("some error"))
				return CloudFormation{
					appStackSet: m,
				}
			},

			wantedErr: errors.New(`retrieve stack instance streamers`),
		},
		"cancel all goroutines if a streamer fails": {
			in: renderStackSetInput{
				name:               "demo-infra",
				hasInstanceUpdates: true,
				createOpFn: func(context.Context) (string, error) {
					return "1", nil
				},
				now: func() time.Time {
					return testDate
				},
			},
			mock: func(t *testing.T, ctrl *gomock.Controller) CloudFormation {
				mockStackSet := mocks.NewMockstackSetClient(ctrl)
				mockStackSet.EXPECT().InstanceSummaries(gomock.Any(), gomock.Any(), gomock.Any()).Return([]stackset.InstanceSummary{
					{
						StackID: "stackset-instance-demo-infra",
						Account: "1111",
						Region:  "us-west-2",
						Status:  "RUNNING",
					},
				}, nil)
				mockStackSet.EXPECT().DescribeOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(stackset.Operation{
					Status: "RUNNING",
				}, nil).AnyTimes()

				mockStack := mocks.NewMockcfnClient(ctrl)
				mockStack.EXPECT().DescribeStackEvents(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("some error")).AnyTimes()

				return CloudFormation{
					appStackSet: mockStackSet,
					cfnClient:   mockStack,
					regionalClient: func(_ string) cfnClient {
						return mockStack
					},
					console: mockFileWriter{
						Writer: new(strings.Builder),
					},
				}
			},

			wantedErr: errors.New(`render progress of stack set "demo-infra"`),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			client := tc.mock(t, ctrl)

			// WHEN
			err := client.renderStackSetImpl(t.Context(), tc.in)

			// THEN
			if tc.wantedErr != nil {
				require.ErrorContains(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCloudFormation_RenderStackSetStopsOnCallerCancellation(t *testing.T) {
	type contextKey string
	const key contextKey = "sentinel"
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key, "stack-set-render"))
	defer cancel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mocks.NewMockstackSetClient(ctrl)
	started := make(chan struct{})
	m.EXPECT().DescribeOperation(gomock.Any(), "demo-infra", "1").DoAndReturn(
		func(gotCtx context.Context, _, _ string) (stackset.Operation, error) {
			require.Equal(t, "stack-set-render", gotCtx.Value(key))
			close(started)
			<-gotCtx.Done()
			return stackset.Operation{}, gotCtx.Err()
		},
	)
	cf := CloudFormation{
		appStackSet: m,
		console:     mockFileWriter{Writer: new(strings.Builder)},
	}
	go func() {
		<-started
		cancel()
	}()
	startedAt := time.Now()

	err := cf.renderStackSetImpl(ctx, renderStackSetInput{
		name:     "demo-infra",
		template: "{}",
		createOpFn: func(gotCtx context.Context) (string, error) {
			require.Same(t, ctx, gotCtx)
			return "1", nil
		},
		now: time.Now,
	})

	require.ErrorIs(t, err, context.Canceled)
	require.Less(t, time.Since(startedAt), time.Second)
}
