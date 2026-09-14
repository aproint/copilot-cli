// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// Package identity provides a client to make API requests to AWS Security Token Service.
package identity

import (
	"context"
	"fmt"

	awsv2 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type api interface {
	GetCallerIdentity(ctx context.Context, input *sts.GetCallerIdentityInput, opts ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// STS wraps the internal sts client.
type STS struct {
	client api
}

// New returns a STS configured with the input SDK v2 config.
func New(cfg awsv2.Config) STS {
	return STS{
		client: sts.NewFromConfig(cfg),
	}
}

// Caller holds information about a calling entity.
type Caller struct {
	RootUserARN string
	Account     string
	UserID      string
}

// Get returns the Caller associated with the Client's session.
func (s STS) Get(ctx context.Context) (Caller, error) {
	out, err := s.client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return Caller{}, fmt.Errorf("get caller identity: %w", err)
	}
	parsedARN, err := arn.Parse(awsv2.ToString(out.Arn))
	if err != nil {
		return Caller{}, fmt.Errorf("parse caller arn: %w", err)
	}

	return Caller{
		RootUserARN: fmt.Sprintf("arn:%s:iam::%s:root", parsedARN.Partition, awsv2.ToString(out.Account)),
		Account:     awsv2.ToString(out.Account),
		UserID:      awsv2.ToString(out.UserId),
	}, nil
}
