// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/identity"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type mockSSM struct {
	t                       *testing.T
	mockPutParameter        func(t *testing.T, ctx context.Context, param *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
	mockGetParametersByPath func(t *testing.T, ctx context.Context, param *ssm.GetParametersByPathInput) (*ssm.GetParametersByPathOutput, error)
	mockGetParameter        func(t *testing.T, ctx context.Context, param *ssm.GetParameterInput) (*ssm.GetParameterOutput, error)
	mockDeleteParameter     func(t *testing.T, ctx context.Context, param *ssm.DeleteParameterInput) (*ssm.DeleteParameterOutput, error)
}

func (m *mockSSM) PutParameter(ctx context.Context, in *ssm.PutParameterInput, opts ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	return m.mockPutParameter(m.t, ctx, in)
}

func (m *mockSSM) GetParametersByPath(ctx context.Context, in *ssm.GetParametersByPathInput, opts ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	return m.mockGetParametersByPath(m.t, ctx, in)
}

func (m *mockSSM) GetParameter(ctx context.Context, in *ssm.GetParameterInput, opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return m.mockGetParameter(m.t, ctx, in)
}

func (m *mockSSM) DeleteParameter(ctx context.Context, in *ssm.DeleteParameterInput, opts ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error) {
	return m.mockDeleteParameter(m.t, ctx, in)
}

type mockIdentityService struct {
	mockIdentityServiceGet func(context.Context) (identity.Caller, error)
}

func (m mockIdentityService) Get(ctx context.Context) (identity.Caller, error) {
	return m.mockIdentityServiceGet(ctx)
}
