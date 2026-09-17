// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
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
	ctx context.Context
	cfn stackDescriber
}

// NewPipelineStackDescriber instantiates a pipeline stack describer using ctx.
func NewPipelineStackDescriber(ctx context.Context, appName, name string, isLegacy bool) (*PipelineStackDescriber, error) {
	cfg, err := sessions.ImmutableProvider().DefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &PipelineStackDescriber{
		ctx: ctx,
		cfn: describestack.NewStackDescriber(stack.NameForPipeline(appName, name, isLegacy), cfg),
	}, nil
}

// Version returns the CloudFormation template version associated with
// the pipeline by reading the Metadata.Version field from the template.
//
// If the Version field does not exist, then it's a legacy template and it returns an version.LegacyPipelineTemplate and nil error.
func (d *PipelineStackDescriber) Version() (string, error) {
	return stackVersion(d.ctx, d.cfn, version.LegacyPipelineTemplate)
}
