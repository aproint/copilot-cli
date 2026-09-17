//go:build integration || localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/config"

	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"gopkg.in/yaml.v3"

	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/stretchr/testify/require"
)

// TestGHPipeline_Template ensures that the CloudFormation template generated for a pipeline matches our pre-defined template.
func TestGHPipeline_Template(t *testing.T) {
	var build deploy.Build
	build.Init(nil, "copilot/pipelines/phonetool-pipeline/")
	var stage deploy.PipelineStage
	stage.Init(&config.Environment{
		App:              "phonetool",
		Name:             "test",
		Region:           "us-west-2",
		AccountID:        "1111",
		ExecutionRoleARN: "arn:aws:iam::1111:role/phonetool-test-CFNExecutionRole",
		ManagerRoleARN:   "arn:aws:iam::1111:role/phonetool-test-EnvManagerRole",
	}, &manifest.PipelineStage{
		Name:             "test",
		RequiresApproval: true,
		PreDeployments: map[string]*manifest.PrePostDeployment{
			"preAction1": {
				BuildspecPath: "./buildspec.yml",
			},
			"preAction2": {
				BuildspecPath: "copilot/pipelines/buildspec.yml",
				DependsOn:     []string{"preAction1"},
			},
		},
		PostDeployments: map[string]*manifest.PrePostDeployment{
			"postAction1": {
				BuildspecPath: ".anotherPath/buildspec.yml",
			},
			"postAction2": {
				BuildspecPath: ".somePath/buildspec.yml",
				DependsOn:     []string{"postAction1"},
			},
			"postAction3": {
				BuildspecPath: ".someOtherPath/buildspec.yml",
			},
		},
	}, []string{"api", "frontend"})
	ps := stack.NewPipelineStackConfig(&deploy.CreatePipelineInput{
		AppName: "phonetool",
		Name:    "phonetool-pipeline",
		Source: &deploy.GitHubSource{
			ProviderName:  manifest.GithubProviderName,
			RepositoryURL: "https://github.com/aws/phonetool",
			Branch:        "mainline",
		},
		Build:  &build,
		Stages: []deploy.PipelineStage{stage},
		ArtifactBuckets: []deploy.ArtifactBucket{
			{
				BucketName: "fancy-bucket",
				KeyArn:     "arn:aws:kms:us-west-2:1111:key/abcd",
			},
		},
		AdditionalTags: nil,
		Version:        "v1.28.0",
	})

	actual, err := ps.Template()
	require.NoError(t, err, "template should have rendered successfully")
	actualInBytes := []byte(actual)
	m1 := make(map[interface{}]interface{})
	require.NoError(t, yaml.Unmarshal(actualInBytes, m1))

	wanted, err := os.ReadFile(filepath.Join("testdata", "pipeline", "gh_template.yaml"))
	require.NoError(t, err, "should be able to read expected template file")
	wantedInBytes := []byte(wanted)
	m2 := make(map[interface{}]interface{})
	require.NoError(t, yaml.Unmarshal(wantedInBytes, m2))

	require.Equal(t, m2, m1)
}

func TestGHPipeline_CrossRegionTemplate(t *testing.T) {
	var build deploy.Build
	require.NoError(t, build.Init(nil, "copilot/pipelines/phonetool-pipeline/"))
	stages := make([]deploy.PipelineStage, 2)
	stages[0].Init(&config.Environment{
		App: "phonetool", Name: "test", Region: "us-west-2", AccountID: "111122223333",
		ExecutionRoleARN: "arn:aws:iam::111122223333:role/test-CFNExecutionRole",
		ManagerRoleARN:   "arn:aws:iam::111122223333:role/test-EnvManagerRole",
	}, &manifest.PipelineStage{
		Name: "test", RequiresApproval: true, TestCommands: []string{"go test ./..."},
		PreDeployments:  map[string]*manifest.PrePostDeployment{"prepare": {BuildspecPath: "ci/prepare.yml"}},
		PostDeployments: map[string]*manifest.PrePostDeployment{"smoke": {BuildspecPath: "ci/smoke.yml"}},
	}, []string{"api"})
	stages[1].Init(&config.Environment{
		App: "phonetool", Name: "prod", Region: "us-east-1", AccountID: "444455556666",
		ExecutionRoleARN: "arn:aws:iam::444455556666:role/prod-CFNExecutionRole",
		ManagerRoleARN:   "arn:aws:iam::444455556666:role/prod-EnvManagerRole",
	}, &manifest.PipelineStage{
		Name: "prod", RequiresApproval: true,
		PreDeployments:  map[string]*manifest.PrePostDeployment{"check": {BuildspecPath: "ci/check.yml"}},
		PostDeployments: map[string]*manifest.PrePostDeployment{"notify": {BuildspecPath: "ci/notify.yml"}},
	}, []string{"api"})
	ps := stack.NewPipelineStackConfig(&deploy.CreatePipelineInput{
		AppName: "phonetool", Name: "phonetool-pipeline",
		Source: &deploy.GitHubSource{
			ProviderName:  manifest.GithubProviderName,
			RepositoryURL: "https://github.com/aws/phonetool", Branch: "mainline",
			ConnectionARN: "arn:aws:codestar-connections:us-west-2:111122223333:connection/12345678-1234-1234-1234-123456789012",
		},
		Build: &build, Stages: stages,
		ArtifactBuckets: []deploy.ArtifactBucket{
			{BucketName: "phonetool-artifacts-west", KeyArn: "arn:aws:kms:us-west-2:111122223333:key/west"},
			{BucketName: "phonetool-artifacts-east", KeyArn: "arn:aws:kms:us-east-1:444455556666:key/east"},
		},
		Version: "v1.28.0",
	})
	actual, err := ps.Template()
	require.NoError(t, err)
	assertTemplateFixture(t, filepath.Join("testdata", "pipeline", "gh_cross_region_template.yaml"), actual)
}
