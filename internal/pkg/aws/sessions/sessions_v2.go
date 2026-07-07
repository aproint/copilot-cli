// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package sessions

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/aproint/copilot-cli/internal/pkg/version"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	v2config "github.com/aws/aws-sdk-go-v2/config"
	v2credentials "github.com/aws/aws-sdk-go-v2/credentials"
	v2stscreds "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

type v2ConfigLoader func(context.Context, ...func(*v2config.LoadOptions) error) (awsv2.Config, error)

// DefaultConfig returns an SDK v2 config configured against the default AWS profile.
// DefaultConfig assumes that a region must be present with the config, otherwise it returns an error.
func (p *Provider) DefaultConfig(ctx context.Context) (awsv2.Config, error) {
	cfg, err := p.defaultV2Config(ctx)
	if err != nil {
		return awsv2.Config{}, err
	}
	if cfg.Region == "" {
		return awsv2.Config{}, &errMissingRegion{}
	}
	return cfg, nil
}

// DefaultConfigWithRegion returns an SDK v2 config configured against the default AWS profile and the input region.
func (p *Provider) DefaultConfigWithRegion(ctx context.Context, region string) (awsv2.Config, error) {
	return p.v2Loader()(ctx, p.v2LoadOptions(v2config.WithRegion(region))...)
}

// ConfigFromProfile returns an SDK v2 config configured against the input profile name.
func (p *Provider) ConfigFromProfile(ctx context.Context, name string) (awsv2.Config, error) {
	cfg, err := p.v2Loader()(ctx, p.v2LoadOptions(v2config.WithSharedConfigProfile(name))...)
	if err != nil {
		return awsv2.Config{}, err
	}
	if cfg.Region == "" {
		return awsv2.Config{}, &errMissingRegion{}
	}
	if _, err := p.v2Validator().ValidateV2Credentials(ctx, cfg); err != nil {
		if isCredRetrievalErr(err) {
			return awsv2.Config{}, &errCredRetrieval{profile: name, parentErr: err}
		}
		return awsv2.Config{}, err
	}
	return cfg, nil
}

// ConfigFromRole returns an SDK v2 config configured against the input role and region.
func (p *Provider) ConfigFromRole(ctx context.Context, roleARN string, region string) (awsv2.Config, error) {
	cfg, err := p.defaultV2Config(ctx)
	if err != nil {
		return awsv2.Config{}, fmt.Errorf("create default config: %w", err)
	}

	cfg.Region = region
	cfg.Credentials = awsv2.NewCredentialsCache(v2stscreds.NewAssumeRoleProvider(
		sts.NewFromConfig(cfg),
		roleARN,
		func(o *v2stscreds.AssumeRoleOptions) {
			o.TokenProvider = v2stscreds.StdinTokenProvider
		},
	))
	return cfg, nil
}

// ConfigFromStaticCreds returns an SDK v2 config from static credentials.
func (p *Provider) ConfigFromStaticCreds(accessKeyID, secretAccessKey, sessionToken string) (awsv2.Config, error) {
	return awsv2.Config{
		Credentials: awsv2.NewCredentialsCache(v2credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, sessionToken)),
		HTTPClient:  newV2HTTPClient(),
		Retryer:     newV2Retryer,
		APIOptions:  p.v2APIOptions(),
	}, nil
}

func (p *Provider) defaultV2Config(ctx context.Context) (awsv2.Config, error) {
	if p.hasDefaultConfigV2 {
		return p.defaultConfigV2, nil
	}

	cfg, err := p.v2Loader()(ctx, p.v2LoadOptions()...)
	if err != nil {
		return awsv2.Config{}, err
	}
	if _, err = p.v2Validator().ValidateV2Credentials(ctx, cfg); err != nil {
		if isCredRetrievalErr(err) {
			return awsv2.Config{}, &errCredRetrieval{parentErr: err}
		}
		return awsv2.Config{}, err
	}

	p.defaultConfigV2 = cfg
	p.hasDefaultConfigV2 = true
	return cfg, nil
}

func (p *Provider) v2LoadOptions(additional ...func(*v2config.LoadOptions) error) []func(*v2config.LoadOptions) error {
	opts := []func(*v2config.LoadOptions) error{
		v2config.WithHTTPClient(newV2HTTPClient()),
		v2config.WithRetryer(newV2Retryer),
		v2config.WithAssumeRoleCredentialOptions(func(o *v2stscreds.AssumeRoleOptions) {
			o.TokenProvider = v2stscreds.StdinTokenProvider
		}),
		v2config.WithAPIOptions(p.v2APIOptions()),
	}
	return append(opts, additional...)
}

func (p *Provider) v2Loader() v2ConfigLoader {
	if p.loadV2Config != nil {
		return p.loadV2Config
	}
	return v2config.LoadDefaultConfig
}

func (p *Provider) v2Validator() v2ConfigValidator {
	if p.configV2Validator != nil {
		return p.configV2Validator
	}
	return &v2Validator{}
}

func newV2HTTPClient() *http.Client {
	return &http.Client{
		Timeout: clientTimeout,
	}
}

func newV2Retryer() awsv2.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = maxRetriesOnRecoverableFailures
	})
}

func (p *Provider) v2APIOptions() []func(*middleware.Stack) error {
	return []func(*middleware.Stack) error{
		addCopilotUserAgent(p),
	}
}

func (p *Provider) userAgentValue() string {
	extras := append([]string{runtime.GOOS}, p.userAgentExtras...)
	return fmt.Sprintf("%s/%s (%s)", userAgentProductName, version.Version, strings.Join(extras, "; "))
}

// AreV2CredsFromEnvVars returns true if the config's credentials provider is environment variables, false otherwise.
// An error is returned if the credentials are invalid or the request times out.
func AreV2CredsFromEnvVars(ctx context.Context, cfg awsv2.Config) (bool, error) {
	v, err := V2Creds(ctx, cfg)
	if err != nil {
		return false, err
	}
	return v.Source == v2config.CredentialsSourceName, nil
}

// V2Creds returns the credential values from an SDK v2 config.
func V2Creds(ctx context.Context, cfg awsv2.Config) (awsv2.Credentials, error) {
	ctx, cancel := context.WithTimeout(ctx, credsTimeout)
	defer cancel()

	if cfg.Credentials == nil {
		return awsv2.Credentials{}, errors.New("get credentials of config: missing credentials provider")
	}
	v, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return awsv2.Credentials{}, fmt.Errorf("get credentials of config: %w", err)
	}
	return v, nil
}

type v2Validator struct{}

func (v *v2Validator) ValidateV2Credentials(ctx context.Context, cfg awsv2.Config) (awsv2.Credentials, error) {
	return V2Creds(ctx, cfg)
}

type copilotUserAgent struct {
	provider *Provider
}

func addCopilotUserAgent(provider *Provider) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(&copilotUserAgent{provider: provider}, middleware.After)
	}
}

func (m *copilotUserAgent) ID() string {
	return "CopilotUserAgent"
}

func (m *copilotUserAgent) HandleBuild(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (
	out middleware.BuildOutput, metadata middleware.Metadata, err error,
) {
	req, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return out, metadata, fmt.Errorf("unknown transport type %T", in.Request)
	}

	value := m.provider.userAgentValue()
	if existing := req.Header.Get("User-Agent"); existing != "" {
		req.Header.Set("User-Agent", fmt.Sprintf("%s %s", existing, value))
	} else {
		req.Header.Set("User-Agent", value)
	}
	return next.HandleBuild(ctx, in)
}
