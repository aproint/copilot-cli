// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package sessions

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/version"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	v2config "github.com/aws/aws-sdk-go-v2/config"
	v2credentials "github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
)

type mockV2CredentialsProvider struct {
	value awsv2.Credentials
	err   error
}

func (m mockV2CredentialsProvider) Retrieve(context.Context) (awsv2.Credentials, error) {
	if m.err != nil {
		return awsv2.Credentials{}, m.err
	}
	return m.value, nil
}

type stubV2Loader struct {
	cfg   awsv2.Config
	err   error
	calls int
	opts  v2config.LoadOptions
}

func (s *stubV2Loader) load(_ context.Context, optFns ...func(*v2config.LoadOptions) error) (awsv2.Config, error) {
	s.calls++
	var opts v2config.LoadOptions
	for _, fn := range optFns {
		if err := fn(&opts); err != nil {
			return awsv2.Config{}, err
		}
	}
	s.opts = opts
	if s.err != nil {
		return awsv2.Config{}, s.err
	}
	return s.cfg, nil
}

type stubV2Validator struct {
	value awsv2.Credentials
	err   error
	calls int
}

func (s *stubV2Validator) ValidateV2Credentials(context.Context, awsv2.Config) (awsv2.Credentials, error) {
	s.calls++
	if s.err != nil {
		return awsv2.Credentials{}, s.err
	}
	return s.value, nil
}

func TestProvider_DefaultConfig(t *testing.T) {
	t.Run("returns a cached config with v2 defaults applied", func(t *testing.T) {
		loader := &stubV2Loader{
			cfg: awsv2.Config{
				Region:      "us-west-2",
				Credentials: mockV2CredentialsProvider{},
			},
		}
		validator := &stubV2Validator{}
		provider := &Provider{
			userAgentExtras:   []string{"svc deploy"},
			loadV2Config:      loader.load,
			configV2Validator: validator,
		}

		cfg, err := provider.DefaultConfig(context.Background())
		require.NoError(t, err)
		require.Equal(t, "us-west-2", cfg.Region)

		cached, err := provider.DefaultConfig(context.Background())
		require.NoError(t, err)
		require.Equal(t, cfg, cached)
		require.Equal(t, 1, loader.calls)
		require.Equal(t, 1, validator.calls)

		httpClient, ok := loader.opts.HTTPClient.(*http.Client)
		require.True(t, ok)
		require.Equal(t, clientTimeout, httpClient.Timeout)
		require.NotNil(t, loader.opts.AssumeRoleCredentialOptions)
		require.Len(t, loader.opts.APIOptions, 1)
		require.Equal(t, maxRetriesOnRecoverableFailures, loader.opts.Retryer().MaxAttempts())
	})

	t.Run("returns an error if region is missing", func(t *testing.T) {
		loader := &stubV2Loader{
			cfg: awsv2.Config{
				Credentials: mockV2CredentialsProvider{},
			},
		}
		provider := &Provider{
			loadV2Config:      loader.load,
			configV2Validator: &stubV2Validator{},
		}

		cfg, err := provider.DefaultConfig(context.Background())
		require.EqualError(t, err, "missing region configuration")
		require.Equal(t, awsv2.Config{}, cfg)
	})

	t.Run("wraps credential retrieval errors", func(t *testing.T) {
		loader := &stubV2Loader{
			cfg: awsv2.Config{
				Region:      "us-west-2",
				Credentials: mockV2CredentialsProvider{},
			},
		}
		provider := &Provider{
			loadV2Config:      loader.load,
			configV2Validator: &stubV2Validator{err: context.DeadlineExceeded},
		}

		cfg, err := provider.DefaultConfig(context.Background())
		require.EqualError(t, err, "context deadline exceeded")
		require.Equal(t, awsv2.Config{}, cfg)
		var retrievalErr *errCredRetrieval
		require.ErrorAs(t, err, &retrievalErr)
	})
}

func TestProvider_DefaultConfigWithRegion(t *testing.T) {
	loader := &stubV2Loader{
		cfg: awsv2.Config{
			Region:      "us-west-2",
			Credentials: mockV2CredentialsProvider{},
		},
	}
	provider := &Provider{loadV2Config: loader.load}

	cfg, err := provider.DefaultConfigWithRegion(context.Background(), "us-west-2")
	require.NoError(t, err)
	require.Equal(t, "us-west-2", cfg.Region)
	require.Equal(t, "us-west-2", loader.opts.Region)
}

func TestProvider_ConfigFromProfile(t *testing.T) {
	t.Run("loads config with profile and validates credentials", func(t *testing.T) {
		loader := &stubV2Loader{
			cfg: awsv2.Config{
				Region:      "us-west-2",
				Credentials: mockV2CredentialsProvider{},
			},
		}
		validator := &stubV2Validator{}
		provider := &Provider{
			loadV2Config:      loader.load,
			configV2Validator: validator,
		}

		cfg, err := provider.ConfigFromProfile(context.Background(), "prod")
		require.NoError(t, err)
		require.Equal(t, "us-west-2", cfg.Region)
		require.Equal(t, "prod", loader.opts.SharedConfigProfile)
		require.Equal(t, 1, validator.calls)
	})

	t.Run("wraps credential retrieval errors with profile context", func(t *testing.T) {
		loader := &stubV2Loader{
			cfg: awsv2.Config{
				Region:      "us-west-2",
				Credentials: mockV2CredentialsProvider{},
			},
		}
		provider := &Provider{
			loadV2Config:      loader.load,
			configV2Validator: &stubV2Validator{err: errors.New("NoCredentialProviders: no valid providers in chain")},
		}

		cfg, err := provider.ConfigFromProfile(context.Background(), "prod")
		require.EqualError(t, err, "NoCredentialProviders: no valid providers in chain")
		require.Equal(t, awsv2.Config{}, cfg)
		var retrievalErr *errCredRetrieval
		require.ErrorAs(t, err, &retrievalErr)
		require.Equal(t, "prod", retrievalErr.profile)
	})
}

func TestProvider_ConfigFromStaticCreds(t *testing.T) {
	provider := &Provider{}

	cfg, err := provider.ConfigFromStaticCreds("access", "secret", "token")
	require.NoError(t, err)

	httpClient, ok := cfg.HTTPClient.(*http.Client)
	require.True(t, ok)
	require.Equal(t, clientTimeout, httpClient.Timeout)
	require.Equal(t, maxRetriesOnRecoverableFailures, cfg.Retryer().MaxAttempts())
	require.Len(t, cfg.APIOptions, 1)

	creds, err := V2Creds(context.Background(), cfg)
	require.NoError(t, err)
	require.Equal(t, awsv2.Credentials{
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		SessionToken:    "token",
		Source:          v2credentials.StaticCredentialsName,
	}, creds)
}

func TestV2Creds(t *testing.T) {
	t.Run("returns values if provider is valid", func(t *testing.T) {
		cfg := awsv2.Config{
			Credentials: mockV2CredentialsProvider{
				value: awsv2.Credentials{
					AccessKeyID:     "abc",
					SecretAccessKey: "def",
				},
			},
		}

		creds, err := V2Creds(context.Background(), cfg)
		require.NoError(t, err)
		require.Equal(t, awsv2.Credentials{
			AccessKeyID:     "abc",
			SecretAccessKey: "def",
		}, creds)
	})

	t.Run("returns a wrapped error if fetching credentials fails", func(t *testing.T) {
		cfg := awsv2.Config{
			Credentials: mockV2CredentialsProvider{err: errors.New("some error")},
		}

		_, err := V2Creds(context.Background(), cfg)
		require.EqualError(t, err, "get credentials of config: some error")
	})

	t.Run("returns an error if provider is missing", func(t *testing.T) {
		_, err := V2Creds(context.Background(), awsv2.Config{})
		require.EqualError(t, err, "get credentials of config: missing credentials provider")
	})
}

func TestAreV2CredsFromEnvVars(t *testing.T) {
	testCases := map[string]struct {
		source string
		want   bool
	}{
		"returns true if credentials come from environment variables": {
			source: v2config.CredentialsSourceName,
			want:   true,
		},
		"returns false if credentials do not come from environment variables": {
			source: v2credentials.StaticCredentialsName,
			want:   false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			cfg := awsv2.Config{
				Credentials: mockV2CredentialsProvider{
					value: awsv2.Credentials{Source: tc.source},
				},
			}

			ok, err := AreV2CredsFromEnvVars(context.Background(), cfg)
			require.NoError(t, err)
			require.Equal(t, tc.want, ok)
		})
	}
}

func TestAddCopilotUserAgent(t *testing.T) {
	provider := &Provider{}
	testCases := map[string]struct {
		existing string
	}{
		"sets the header if missing": {},
		"appends to existing user agent": {
			existing: "aws-sdk-go-v2/1.0.0",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := renderCopilotUserAgent(t, provider, tc.existing)
			require.Contains(t, got, "aws-sdk-go-v2/")
			require.Contains(t, got, userAgentProductName+"/"+version.Version)
			if tc.existing != "" {
				require.Contains(t, got, tc.existing)
			}
		})
	}
}

func TestAddCopilotUserAgentUsesLatestProviderExtras(t *testing.T) {
	provider := &Provider{userAgentExtras: []string{"svc deploy"}}

	provider.UserAgentExtras("override cdk")
	got := renderCopilotUserAgent(t, provider, "")

	require.Contains(t, got, userAgentProductName+"/"+version.Version)
	require.Contains(t, got, userAgentCommandKey+"/svc-deploy")
	require.Contains(t, got, userAgentCommandKey+"/override-cdk")
}

func renderCopilotUserAgent(t *testing.T, provider *Provider, existing string) string {
	t.Helper()

	stack := middleware.NewStack("testStack", smithyhttp.NewStackRequest)
	req := &smithyhttp.Request{Request: &http.Request{Header: http.Header{}}}
	if existing != "" {
		req.Header.Set("User-Agent", existing)
	}
	err := stack.Build.Add(middleware.BuildMiddlewareFunc("setRequest", func(ctx context.Context, _ middleware.BuildInput, handler middleware.BuildHandler) (
		out middleware.BuildOutput, metadata middleware.Metadata, err error,
	) {
		return handler.HandleBuild(ctx, middleware.BuildInput{Request: req})
	}), middleware.After)
	require.NoError(t, err)

	err = addCopilotUserAgent(provider)(stack)
	require.NoError(t, err)
	_, _, err = middleware.DecorateHandler(middleware.HandlerFunc(func(_ context.Context, input interface{}) (
		output interface{}, metadata middleware.Metadata, err error,
	) {
		return input, metadata, nil
	}), stack).Handle(context.Background(), nil)
	require.NoError(t, err)

	return req.Header.Get("User-Agent")
}
