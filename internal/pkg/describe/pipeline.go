// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package describe

import (
	"context"

	"github.com/aproint/copilot-cli/internal/pkg/aws/sessions"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	describestack "github.com/aproint/copilot-cli/internal/pkg/describe/stack"
	"github.com/aproint/copilot-cli/internal/pkg/version"
)

// PipelineStackDescriber retrieves information about a deployed pipeline stack.
type PipelineStackDescriber struct {
	cfn stackDescriber
}

// NewPipelineStackDescriber instantiates a new pipeline stack describer
func NewPipelineStackDescriber(appName, name string, isLegacy bool) (*PipelineStackDescriber, error) {
	cfg, err := sessions.ImmutableProvider().DefaultConfig(context.Background())
	if err != nil {
		return nil, err
	}
	return &PipelineStackDescriber{
		cfn: describestack.NewStackDescriber(stack.NameForPipeline(appName, name, isLegacy), cfg),
	}, nil
}

// Version returns the CloudFormation template version associated with
// the pipeline by reading the Metadata.Version field from the template.
//
// If the Version field does not exist, then it's a legacy template and it returns an version.LegacyPipelineTemplate and nil error.
func (d *PipelineStackDescriber) Version() (string, error) {
	return stackVersion(d.cfn, version.LegacyPipelineTemplate)
}
