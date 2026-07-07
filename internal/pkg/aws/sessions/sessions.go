// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package sessions provides functions that return AWS SDK configs to use in the AWS SDK.
package sessions

import (
	"context"
	"sync"
	"time"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
)

// Timeout settings.
const (
	maxRetriesOnRecoverableFailures = 8 // Default provided by SDK is 3 which means requests are retried up to only 2 seconds.
	credsTimeout                    = 10 * time.Second
	clientTimeout                   = 30 * time.Second
)

// User-Agent settings.
const (
	userAgentProductName = "aws-copilot"
)

// Provider provides methods to create AWS SDK configs.
// Once a config is created, it's cached locally so that the same config is not re-created.
type Provider struct {
	defaultConfigV2    awsv2.Config
	hasDefaultConfigV2 bool

	// Metadata associated with the provider.
	userAgentExtras   []string
	loadV2Config      v2ConfigLoader
	configV2Validator v2ConfigValidator
}

type v2ConfigValidator interface {
	ValidateV2Credentials(context.Context, awsv2.Config) (awsv2.Credentials, error)
}

var instance *Provider
var once sync.Once

// ImmutableProvider returns an immutable session Provider with the options applied.
func ImmutableProvider(options ...func(*Provider)) *Provider {
	once.Do(func() {
		instance = &Provider{}
		for _, option := range options {
			option(instance)
		}
	})
	return instance
}

// UserAgentExtras augments a session provider with additional User-Agent extras.
func UserAgentExtras(extras ...string) func(*Provider) {
	return func(p *Provider) {
		p.userAgentExtras = append(p.userAgentExtras, extras...)
	}
}

// UserAgentExtras adds additional User-Agent extras to cached configs and any new configs.
func (p *Provider) UserAgentExtras(extras ...string) {
	p.userAgentExtras = append(p.userAgentExtras, extras...)
}
