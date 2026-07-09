// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/identity"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/require"
)

func TestStore_ListApplications(t *testing.T) {
	ctx := context.WithValue(context.Background(), "operation", "list-applications")
	testApplication := Application{Name: "chicken", Version: "1.0"}
	testApplicationString, err := marshal(testApplication)
	require.NoError(t, err, "Marshal application should not fail")

	cowApplication := Application{Name: "cow", Version: "1.0"}
	cowApplicationString, err := marshal(cowApplication)
	require.NoError(t, err, "Marshal application should not fail")

	lastPageInPaginatedResp := false

	testCases := map[string]struct {
		mockGetParametersByPath func(t *testing.T, _ context.Context, param *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error)

		wantedApplicationNames []string
		wantedErr              error
	}{
		"with multiple existing applications": {
			mockGetParametersByPath: func(t *testing.T, gotCtx context.Context, param *ssm.GetParametersByPathInput) (output *ssm.GetParametersByPathOutput, e error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, rootApplicationPath, *param.Path)
				return &ssm.GetParametersByPathOutput{
					Parameters: []types.Parameter{
						{
							Name:  aws.String("/copilot/applications/chicken"),
							Value: aws.String(testApplicationString),
						},
						{
							Name:  aws.String("/copilot/applications/cow"),
							Value: aws.String(cowApplicationString),
						},
					},
				}, nil
			},

			wantedApplicationNames: []string{"chicken", "cow"},
			wantedErr:              nil,
		},
		"with malformed json": {
			mockGetParametersByPath: func(t *testing.T, _ context.Context, param *ssm.GetParametersByPathInput) (output *ssm.GetParametersByPathOutput, e error) {
				require.Equal(t, rootApplicationPath, *param.Path)
				return &ssm.GetParametersByPathOutput{
					Parameters: []types.Parameter{
						{
							Name:  aws.String("/copilot/applications/chicken"),
							Value: aws.String("oops"),
						},
					},
				}, nil
			},
			wantedErr: fmt.Errorf("read application configuration: invalid character 'o' looking for beginning of value"),
		},
		"with SSM error": {
			mockGetParametersByPath: func(t *testing.T, _ context.Context, param *ssm.GetParametersByPathInput) (output *ssm.GetParametersByPathOutput, e error) {
				require.Equal(t, rootApplicationPath, *param.Path)
				return nil, fmt.Errorf("broken")
			},

			wantedApplicationNames: nil,
			wantedErr:              fmt.Errorf("list applications: broken"),
		},
		"with paginated response": {
			mockGetParametersByPath: func(t *testing.T, _ context.Context, param *ssm.GetParametersByPathInput) (output *ssm.GetParametersByPathOutput, e error) {
				require.Equal(t, rootApplicationPath, *param.Path)

				// this closure references the `lastPageInPaginatedResp` variable
				// in order to determine the content of the response.
				if !lastPageInPaginatedResp {
					lastPageInPaginatedResp = true
					return &ssm.GetParametersByPathOutput{
						Parameters: []types.Parameter{
							{
								Name:  aws.String("/copilot/applications/chicken"),
								Value: aws.String(testApplicationString),
							},
						},
						NextToken: aws.String("more"),
					}, nil
				}
				return &ssm.GetParametersByPathOutput{
					Parameters: []types.Parameter{
						{
							Name:  aws.String("/copilot/applications/cow"),
							Value: aws.String(cowApplicationString),
						},
					},
				}, nil
			},

			wantedApplicationNames: []string{"chicken", "cow"},
			wantedErr:              nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			lastPageInPaginatedResp = false
			store := &Store{
				ssm: &mockSSM{
					t:                       t,
					mockGetParametersByPath: tc.mockGetParametersByPath,
				},
			}

			// WHEN
			apps, err := store.ListApplications(ctx)

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				var names []string
				for _, a := range apps {
					names = append(names, a.Name)
				}
				require.ElementsMatch(t, tc.wantedApplicationNames, names)

			}
		})
	}
}

func TestStore_GetApplication(t *testing.T) {
	ctx := context.WithValue(context.Background(), "operation", "get-application")
	testApplication := Application{Name: "chicken", AccountID: "1234", Version: "1.0"}
	testApplicationString, err := marshal(testApplication)
	testApplicationPath := fmt.Sprintf(fmtApplicationPath, testApplication.Name)
	require.NoError(t, err, "Marshal application should not fail")

	testCases := map[string]struct {
		mockGetParameter       func(t *testing.T, _ context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error)
		mockIdentityServiceGet func(context.Context) (identity.Caller, error)
		wantedApplication      Application
		wantedErr              error
	}{
		"with existing application": {
			mockGetParameter: func(t *testing.T, gotCtx context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, testApplicationPath, *param.Name)
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:  aws.String(testApplicationPath),
						Value: aws.String(testApplicationString),
					},
				}, nil
			},

			wantedApplication: testApplication,
			wantedErr:         nil,
		},
		"with no existing application": {
			mockGetParameter: func(t *testing.T, _ context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
				require.Equal(t, testApplicationPath, *param.Name)
				return nil, &types.ParameterNotFound{}
			},
			mockIdentityServiceGet: func(gotCtx context.Context) (identity.Caller, error) {
				require.Equal(t, ctx, gotCtx)
				return identity.Caller{
					Account: "12345",
				}, nil
			},
			wantedErr: &ErrNoSuchApplication{
				ApplicationName: "chicken",
				AccountID:       "12345",
				Region:          "us-west-2",
			},
		},
		"with no existing application and failed STS call": {
			mockGetParameter: func(t *testing.T, _ context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
				require.Equal(t, testApplicationPath, *param.Name)
				return nil, &types.ParameterNotFound{}
			},
			mockIdentityServiceGet: func(_ context.Context) (identity.Caller, error) {
				return identity.Caller{}, fmt.Errorf("Error")
			},
			wantedErr: &ErrNoSuchApplication{
				ApplicationName: "chicken",
				AccountID:       "unknown",
				Region:          "us-west-2",
			},
		},
		"with malformed json": {
			mockGetParameter: func(t *testing.T, _ context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
				require.Equal(t, testApplicationPath, *param.Name)
				return &ssm.GetParameterOutput{
					Parameter: &types.Parameter{
						Name:  aws.String(testApplicationPath),
						Value: aws.String("oops"),
					},
				}, nil
			},

			wantedErr: fmt.Errorf("read configuration for application chicken: invalid character 'o' looking for beginning of value"),
		},
		"with SSM error": {
			mockGetParameter: func(t *testing.T, _ context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error) {
				require.Equal(t, testApplicationPath, *param.Name)
				return nil, fmt.Errorf("broken")
			},
			wantedErr: fmt.Errorf("get application chicken: broken"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			store := &Store{
				ssm: &mockSSM{
					t:                t,
					mockGetParameter: tc.mockGetParameter,
				},
				sts: mockIdentityService{
					mockIdentityServiceGet: tc.mockIdentityServiceGet,
				},
				appRegion: "us-west-2",
			}

			// WHEN
			app, err := store.GetApplication(ctx, "chicken")

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.Equal(t, tc.wantedApplication, *app)
			}
		})
	}
}

func TestStore_CreateApplication(t *testing.T) {
	ctx := context.WithValue(context.Background(), "operation", "create-application")
	tagForApplicationParam := []types.Tag{
		{
			Key:   aws.String("copilot-application"),
			Value: aws.String("phonetool"),
		},
	}
	testCases := map[string]struct {
		inApplication *Application

		mockPutParameter func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
		wantedErr        error
	}{
		"with no existing application": {
			inApplication: &Application{Name: "phonetool", AccountID: "1234", Domain: "phonetool.com", DomainHostedZoneID: "mockHostedZoneID", Tags: map[string]string{"owner": "boss"}},
			mockPutParameter: func(t *testing.T, gotCtx context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, fmt.Sprintf(fmtApplicationPath, "phonetool"), *param.Name)
				require.Equal(t, fmt.Sprintf(`{"name":"phonetool","account":"1234","domain":"phonetool.com","domainHostedZoneID":"mockHostedZoneID","version":"%s","tags":{"owner":"boss"}}`, schemaVersion), *param.Value)
				require.Equal(t, tagForApplicationParam, param.Tags)
				return &ssm.PutParameterOutput{
					Version: 1,
				}, nil
			},
			wantedErr: nil,
		},
		"with existing application": {
			inApplication: &Application{Name: "phonetool", AccountID: "1234"},
			mockPutParameter: func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
				require.Equal(t, tagForApplicationParam, param.Tags)
				return nil, &types.ParameterAlreadyExists{}
			},
			wantedErr: nil,
		},
		"with SSM error": {
			inApplication: &Application{Name: "phonetool", AccountID: "1234"},
			mockPutParameter: func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
				require.Equal(t, tagForApplicationParam, param.Tags)
				return nil, fmt.Errorf("broken")
			},
			wantedErr: fmt.Errorf("create application phonetool: broken"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			store := &Store{
				ssm: &mockSSM{
					t:                t,
					mockPutParameter: tc.mockPutParameter,
				},
			}

			// WHEN
			err := store.CreateApplication(ctx, tc.inApplication)

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestStore_UpdateApplication(t *testing.T) {
	testCases := map[string]struct {
		inApplication *Application

		mockPutParameter func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
		wantedErr        error
	}{
		"success": {
			inApplication: &Application{Name: "phonetool", AccountID: "1234", Domain: "phonetool.com", DomainHostedZoneID: "mockHostedZoneID", Tags: map[string]string{"owner": "boss"}},
			mockPutParameter: func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
				require.Equal(t, fmt.Sprintf(fmtApplicationPath, "phonetool"), *param.Name)
				require.Equal(t, fmt.Sprintf(`{"name":"phonetool","account":"1234","domain":"phonetool.com","domainHostedZoneID":"mockHostedZoneID","version":"%s","tags":{"owner":"boss"}}`, schemaVersion), *param.Value)

				return &ssm.PutParameterOutput{
					Version: 1,
				}, nil
			},
			wantedErr: nil,
		},
		"with SSM error": {
			inApplication: &Application{Name: "phonetool", AccountID: "1234"},
			mockPutParameter: func(t *testing.T, _ context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
				return nil, fmt.Errorf("broken")
			},
			wantedErr: fmt.Errorf("update application phonetool: broken"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			store := &Store{
				ssm: &mockSSM{
					t:                t,
					mockPutParameter: tc.mockPutParameter,
				},
			}

			// WHEN
			err := store.UpdateApplication(context.Background(), tc.inApplication)

			// THEN
			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDeleteApplication(t *testing.T) {
	ctx := context.WithValue(context.Background(), "operation", "delete-application")
	mockApplicationName := "mockApplicationName"
	mockError := errors.New("mockError")

	tests := map[string]struct {
		mockDeleteParameter func(t *testing.T, _ context.Context, in *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error)

		want error
	}{
		"should return nil given success": {
			mockDeleteParameter: func(t *testing.T, gotCtx context.Context, in *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error) {
				require.Equal(t, ctx, gotCtx)
				require.Equal(t, fmt.Sprintf(fmtApplicationPath, mockApplicationName), *in.Name)

				return &ssm.DeleteParameterOutput{}, nil
			},
			want: nil,
		},
		"should return nil given paramter not found error code": {
			mockDeleteParameter: func(t *testing.T, _ context.Context, in *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error) {
				require.Equal(t, fmt.Sprintf(fmtApplicationPath, mockApplicationName), *in.Name)

				return nil, &types.ParameterNotFound{}
			},
			want: nil,
		},
		"should return unhandled non-awserr": {
			mockDeleteParameter: func(t *testing.T, _ context.Context, in *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error) {
				require.Equal(t, fmt.Sprintf(fmtApplicationPath, mockApplicationName), *in.Name)

				return nil, mockError
			},
			want: mockError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := &Store{
				ssm: &mockSSM{
					t:                   t,
					mockDeleteParameter: test.mockDeleteParameter,
				},
			}

			got := store.DeleteApplication(ctx, mockApplicationName)

			require.Equal(t, test.want, got)
		})
	}
}
