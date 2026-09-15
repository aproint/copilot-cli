// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package sessions

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/aproint/copilot-cli/internal/pkg/version"
	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	v2config "github.com/aws/aws-sdk-go-v2/config"
	v2credentials "github.com/aws/aws-sdk-go-v2/credentials"
	v2stscreds "github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
)

type configLoader func(context.Context, ...func(*v2config.LoadOptions) error) (awsv2.Config, error)

// DefaultConfig returns an SDK v2 config configured against the default AWS profile.
// DefaultConfig assumes that a region must be present with the config, otherwise it returns an error.
func (p *Provider) DefaultConfig(ctx context.Context) (awsv2.Config, error) {
	cfg, err := p.defaultConfig(ctx)
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
	return p.loader()(ctx, p.loadOptions(v2config.WithRegion(region))...)
}

// ConfigFromProfile returns an SDK v2 config configured against the input profile name.
func (p *Provider) ConfigFromProfile(ctx context.Context, name string) (awsv2.Config, error) {
	cfg, err := p.loader()(ctx, p.loadOptions(v2config.WithSharedConfigProfile(name))...)
	if err != nil {
		return awsv2.Config{}, err
	}
	if cfg.Region == "" {
		return awsv2.Config{}, &errMissingRegion{}
	}
	if _, err := p.credentials(ctx, cfg); err != nil {
		if isCredRetrievalErr(err) {
			return awsv2.Config{}, &errCredRetrieval{profile: name, parentErr: err}
		}
		return awsv2.Config{}, err
	}
	return cfg, nil
}

// ConfigFromRole returns an SDK v2 config configured against the input role and region.
func (p *Provider) ConfigFromRole(ctx context.Context, roleARN string, region string) (awsv2.Config, error) {
	cfg, err := p.defaultConfig(ctx)
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
		HTTPClient:  newHTTPClient(),
		Retryer:     newRetryer,
		APIOptions:  p.apiOptions(),
	}, nil
}

func (p *Provider) defaultConfig(ctx context.Context) (awsv2.Config, error) {
	if p.hasCachedDefaultConfig {
		return p.cachedDefaultConfig, nil
	}

	cfg, err := p.loader()(ctx, p.loadOptions()...)
	if err != nil {
		return awsv2.Config{}, err
	}
	if _, err = p.credentials(ctx, cfg); err != nil {
		if isCredRetrievalErr(err) {
			return awsv2.Config{}, &errCredRetrieval{parentErr: err}
		}
		return awsv2.Config{}, err
	}

	p.cachedDefaultConfig = cfg
	p.hasCachedDefaultConfig = true
	return cfg, nil
}

func (p *Provider) loadOptions(additional ...func(*v2config.LoadOptions) error) []func(*v2config.LoadOptions) error {
	opts := []func(*v2config.LoadOptions) error{
		v2config.WithHTTPClient(newHTTPClient()),
		v2config.WithRetryer(newRetryer),
		v2config.WithAssumeRoleCredentialOptions(func(o *v2stscreds.AssumeRoleOptions) {
			o.TokenProvider = v2stscreds.StdinTokenProvider
		}),
		v2config.WithAPIOptions(p.apiOptions()),
	}
	return append(opts, additional...)
}

func (p *Provider) loader() configLoader {
	if p.loadConfig != nil {
		return p.loadConfig
	}
	return v2config.LoadDefaultConfig
}

func (p *Provider) credentials(ctx context.Context, cfg awsv2.Config) (awsv2.Credentials, error) {
	if p.validateCredentials != nil {
		return p.validateCredentials(ctx, cfg)
	}
	return Credentials(ctx, cfg)
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: clientTimeout,
	}
}

func newRetryer() awsv2.Retryer {
	return retry.NewStandard(func(o *retry.StandardOptions) {
		o.MaxAttempts = maxRetriesOnRecoverableFailures
	})
}

func (p *Provider) apiOptions() []func(*middleware.Stack) error {
	return []func(*middleware.Stack) error{
		addCopilotUserAgent(p),
	}
}

// AreCredentialsFromEnvVars returns true if the config's credentials provider is environment variables, false otherwise.
// An error is returned if the credentials are invalid or the request times out.
func AreCredentialsFromEnvVars(ctx context.Context, cfg awsv2.Config) (bool, error) {
	v, err := Credentials(ctx, cfg)
	if err != nil {
		return false, err
	}
	return v.Source == v2config.CredentialsSourceName, nil
}

// Credentials returns the credential values from an AWS SDK config.
func Credentials(ctx context.Context, cfg awsv2.Config) (awsv2.Credentials, error) {
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

func addCopilotUserAgent(provider *Provider) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		if err := awsmiddleware.AddUserAgentKeyValue(userAgentProductName, version.Version)(stack); err != nil {
			return err
		}
		for _, extra := range provider.userAgentExtras {
			if err := awsmiddleware.AddUserAgentKeyValue(userAgentCommandKey, extra)(stack); err != nil {
				return err
			}
		}
		return nil
	}
}
