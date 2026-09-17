//go:build integration || localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/addon"
	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/aproint/copilot-cli/internal/pkg/workspace"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestRDWS_Template(t *testing.T) {
	const manifestFileName = "rdws-manifest.yml"
	testCases := map[string]struct {
		envName      string
		svcStackPath string
		manifestPath string
		paramsPath   string
	}{
		"test env": {
			envName:      "test",
			svcStackPath: "rdws-test.stack.yml",
			manifestPath: manifestFileName,
			paramsPath:   "rdws-test.params.json",
		},
		"prod env": {
			envName:      "prod",
			svcStackPath: "rdws-prod.stack.yml",
			manifestPath: manifestFileName,
			paramsPath:   "rdws-prod.params.json",
		},
		"private ingress with supplied VPC endpoint": {
			envName:      "test",
			svcStackPath: "rdws-private.stack.yml",
			manifestPath: "rdws-private-manifest.yml",
			paramsPath:   "rdws-private.params.json",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			manifestBytes, err := os.ReadFile(filepath.Join("testdata", "workloads", tc.manifestPath))
			require.NoError(t, err, "read manifest file")
			mft, err := manifest.UnmarshalWorkload(manifestBytes)
			require.NoError(t, err, "unmarshal manifest file")
			envMft, err := mft.ApplyEnv(tc.envName)
			require.NoError(t, err, "apply test env to manifest")
			err = envMft.Validate()
			require.NoError(t, err)
			err = envMft.Load(context.Background(), aws.Config{})
			require.NoError(t, err)
			content := envMft.Manifest()

			v, ok := content.(*manifest.RequestDrivenWebService)
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

			// Read actual stack template.
			serializer, err := stack.NewRequestDrivenWebService(stack.RequestDrivenWebServiceConfig{
				App: deploy.AppInformation{
					Name: appName,
				},
				Env:                tc.envName,
				Manifest:           v,
				ArtifactBucketName: "bucket",
				RuntimeConfig: stack.RuntimeConfig{
					AccountID:  "123456789123",
					Region:     "us-west-2",
					EnvVersion: "v1.42.0",
					Version:    "v1.29.0",
				},
			})
			require.NoError(t, err, "create rdws serializer")
			actualTemplate, err := serializer.Template()
			require.NoError(t, err, "get cloudformation template for rdws")
			assertTemplateFixture(t, filepath.Join("testdata", "workloads", tc.svcStackPath), actualTemplate)
			params, err := serializer.SerializedParameters()
			require.NoError(t, err)
			assertParamsFixture(t, filepath.Join("testdata", "workloads", tc.paramsPath), params)
		})
	}
}
