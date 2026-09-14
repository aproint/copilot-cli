// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package selector

import (
	"context"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/term/selector/mocks"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestCredsSelect_Creds(t *testing.T) {
	type contextKey string
	callerCtx := context.WithValue(context.Background(), contextKey("caller"), "credential-selector")
	testCases := map[string]struct {
		inMsg  string
		inHelp string
		given  func(ctrl *gomock.Controller) *CredsSelect

		wantedErr error
	}{
		"should create a session from a named profile": {
			inMsg:  "message",
			inHelp: "help",
			given: func(ctrl *gomock.Controller) *CredsSelect {
				profile := mocks.NewMockNames(ctrl)
				profile.EXPECT().Names().Return([]string{"test", "prod"})

				prompter := mocks.NewMockPrompter(ctrl)
				prompter.EXPECT().SelectOne("message", "help", []string{
					"Enter temporary credentials",
					"[profile test]",
					"[profile prod]",
				}, gomock.Any()).Return("[profile prod]", nil)

				provider := mocks.NewMockSessionProvider(ctrl)
				provider.EXPECT().ConfigFromProfile(callerCtx, "prod").Return(aws.Config{}, nil)

				return &CredsSelect{
					Prompt:  prompter,
					Profile: profile,
					Session: provider,
				}
			},
		},
		"should create a session from temporary credentials with masked prompt": {
			given: func(ctrl *gomock.Controller) *CredsSelect {
				profile := mocks.NewMockNames(ctrl)
				profile.EXPECT().Names().Return(nil)

				prompter := mocks.NewMockPrompter(ctrl)
				prompter.EXPECT().SelectOne(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("Enter temporary credentials", nil)

				provider := mocks.NewMockSessionProvider(ctrl)
				provider.EXPECT().DefaultConfig(gomock.Any()).Return(aws.Config{
					Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(
						"11111",
						"22222",
						"33333",
					)),
				}, nil)

				prompter.EXPECT().Get("What's your AWS Access Key ID?", "", gomock.Any(), gomock.Any()).
					Return("****************1111", nil)
				prompter.EXPECT().Get("What's your AWS Secret Access Key?", "", gomock.Any(), gomock.Any()).
					Return("****************2222", nil)
				prompter.EXPECT().Get("What's your AWS Session Token?", "", nil, gomock.Any()).
					Return("****************3333", nil)

				provider.EXPECT().ConfigFromStaticCreds("11111", "22222", "33333").
					Return(aws.Config{}, nil)

				return &CredsSelect{
					Prompt:  prompter,
					Profile: profile,
					Session: provider,
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			sel := tc.given(ctrl)

			_, err := sel.Creds(callerCtx, tc.inMsg, tc.inHelp)

			if tc.wantedErr != nil {
				require.EqualError(t, err, tc.wantedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCredsSelect_Creds_CanceledContextPreventsSessionLoading(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	prompter := mocks.NewMockPrompter(ctrl)
	prompter.EXPECT().SelectOne(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	provider := mocks.NewMockSessionProvider(ctrl)
	provider.EXPECT().DefaultConfig(gomock.Any()).Times(0)
	provider.EXPECT().ConfigFromProfile(gomock.Any(), gomock.Any()).Times(0)
	selector := &CredsSelect{
		Prompt:  prompter,
		Profile: mocks.NewMockNames(ctrl),
		Session: provider,
	}

	_, err := selector.Creds(ctx, "message", "help")

	require.ErrorIs(t, err, context.Canceled)
}
