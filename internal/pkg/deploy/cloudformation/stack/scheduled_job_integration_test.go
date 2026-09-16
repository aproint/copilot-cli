//go:build integration || localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/addon"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

const (
	jobManifestPath   = "job-manifest.yml"
	jobStackPath      = "job-test.stack.yml"
	jobParamsPath     = "job-test.params.json"
	envControllerPath = "custom-resources/env-controller.js"
)

func TestScheduledJob_Template(t *testing.T) {
	testScheduledJobTemplate(t, jobManifestPath, jobStackPath, jobParamsPath)
}

func TestScheduledJob_ExplicitPlacement(t *testing.T) {
	testScheduledJobTemplate(t, "job-explicit-placement-manifest.yml", "job-explicit-placement.stack.yml", "job-explicit-placement.params.json")
}

func testScheduledJobTemplate(t *testing.T, manifestPath, stackPath, paramsPath string) {
	path := filepath.Join("testdata", "workloads", manifestPath)
	manifestBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	mft, err := manifest.UnmarshalWorkload(manifestBytes)
	require.NoError(t, err)
	envMft, err := mft.ApplyEnv(envName)
	require.NoError(t, err)
	err = envMft.Validate()
	require.NoError(t, err)
	err = envMft.Load(context.Background(), aws.Config{})
	require.NoError(t, err)
	content := envMft.Manifest()

	v, ok := content.(*manifest.ScheduledJob)
	require.True(t, ok)

	// Create in-memory mock file system.
	wd, err := os.Getwd()
	require.NoError(t, err)
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll(fmt.Sprintf("%s/copilot", wd), 0755)
	_ = afero.WriteFile(fs, fmt.Sprintf("%s/copilot/.workspace", wd), []byte(fmt.Sprintf("---\napplication: %s", "DavidsApp")), 0644)
	require.NoError(t, err)

	ws, err := workspace.Use(fs)
	require.NoError(t, err)

	_, err = addon.ParseFromWorkload(aws.ToString(v.Name), ws)
	var notFound *addon.ErrAddonsNotFound
	require.ErrorAs(t, err, &notFound)

	serializer, err := stack.NewScheduledJob(stack.ScheduledJobConfig{
		App: &config.Application{
			Name: appName,
		},
		Env:                envName,
		Manifest:           v,
		ArtifactBucketName: "bucket",
		ArtifactKey:        "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
		RuntimeConfig: stack.RuntimeConfig{
			ServiceDiscoveryEndpoint: "test.my-app.local",
			AccountID:                "123456789123",
			Region:                   "us-west-2",
			EnvVersion:               "v1.42.0",
			Version:                  "v1.29.0",
		},
	})

	tpl, err := serializer.Template()
	require.NoError(t, err, "template should render")
	t.Run("CF Template should be equal", func(t *testing.T) {
		assertTemplateFixture(t, filepath.Join("testdata", "workloads", stackPath), tpl)
	})

	t.Run("Parameter values should render properly", func(t *testing.T) {
		actualParams, err := serializer.SerializedParameters()
		require.NoError(t, err)

		assertParamsFixture(t, filepath.Join("testdata", "workloads", paramsPath), actualParams)
	})

}
