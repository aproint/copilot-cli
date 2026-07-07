// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"github.com/aproint/copilot-cli/internal/pkg/aws/identity"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aws/aws-sdk-go-v2/aws"
)

func newSSMConfigStoreFromConfig(cfg aws.Config) *config.Store {
	return config.NewSSMStore(identity.New(cfg), config.NewSSMClient(cfg), cfg.Region)
}
