//go:build integration || localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// Copyright APROINT, s.r.o. in modifications to this fork.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/aws/elbv2"
	"github.com/aproint/copilot-cli/internal/pkg/config"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/aproint/copilot-cli/internal/pkg/manifest"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func TestBackendService_TemplateAndParamsGeneration(t *testing.T) {
	const (
		appName = "my-app"
	)
	envName := "my-env"

	testDir := filepath.Join("testdata", "workloads", "backend")

	tests := map[string]struct {
		ManifestPath        string
		TemplatePath        string
		ParamsPath          string
		EnvImportedCertARNs []string
		ImportedALB         *elbv2.LoadBalancer
	}{
		"simple": {
			ManifestPath: filepath.Join(testDir, "simple-manifest.yml"),
			TemplatePath: filepath.Join(testDir, "simple-template.yml"),
			ParamsPath:   filepath.Join(testDir, "simple-params.json"),
		},
		"simple without port config": {
			ManifestPath: filepath.Join(testDir, "simple-manifest-without-port-config.yml"),
			TemplatePath: filepath.Join(testDir, "simple-template-without-port-config.yml"),
			ParamsPath:   filepath.Join(testDir, "simple-params-without-port-config.json"),
		},
		"http only path configured": {
			ManifestPath: filepath.Join(testDir, "http-only-path-manifest.yml"),
			TemplatePath: filepath.Join(testDir, "http-only-path-template.yml"),
			ParamsPath:   filepath.Join(testDir, "http-only-path-params.json"),
		},
		"http full config": {
			ManifestPath: filepath.Join(testDir, "http-full-config-manifest.yml"),
			TemplatePath: filepath.Join(testDir, "http-full-config-template.yml"),
			ParamsPath:   filepath.Join(testDir, "http-full-config-params.json"),
		},
		"https path and alias configured": {
			ManifestPath:        filepath.Join(testDir, "https-path-alias-manifest.yml"),
			TemplatePath:        filepath.Join(testDir, "https-path-alias-template.yml"),
			ParamsPath:          filepath.Join(testDir, "https-path-alias-params.json"),
			EnvImportedCertARNs: []string{"exampleComCertARN"},
		},
		"http with autoscaling by requests configured": {
			ManifestPath: filepath.Join(testDir, "http-autoscaling-manifest.yml"),
			TemplatePath: filepath.Join(testDir, "http-autoscaling-template.yml"),
			ParamsPath:   filepath.Join(testDir, "http-autoscaling-params.json"),
		},
		"imported internal ALB with request autoscaling": {
			ManifestPath:        filepath.Join(testDir, "imported-internal-alb-manifest.yml"),
			TemplatePath:        filepath.Join(testDir, "imported-internal-alb-template.yml"),
			ParamsPath:          filepath.Join(testDir, "imported-internal-alb-params.json"),
			EnvImportedCertARNs: []string{"arn:aws:acm:us-west-2:111122223333:certificate/private"},
			ImportedALB: &elbv2.LoadBalancer{
				ARN:  "arn:aws:elasticloadbalancing:us-west-2:111122223333:loadbalancer/app/internal-shared/abc123",
				Name: "internal-shared", DNSName: "internal-shared.us-west-2.elb.amazonaws.com", HostedZoneID: "Z123456",
				SecurityGroups: []string{"sg-shared-alb"},
				Listeners: []elbv2.Listener{
					{ARN: "arn:aws:elasticloadbalancing:us-west-2:111122223333:listener/app/internal-shared/abc123/http", Port: 80, Protocol: "HTTP"},
					{ARN: "arn:aws:elasticloadbalancing:us-west-2:111122223333:listener/app/internal-shared/abc123/https", Port: 443, Protocol: "HTTPS"},
				},
			},
		},
	}

	// run tests
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// parse files
			manifestBytes, err := os.ReadFile(tc.ManifestPath)
			require.NoError(t, err)

			dynamicMft, err := manifest.UnmarshalWorkload([]byte(manifestBytes))
			require.NoError(t, err)
			require.NoError(t, dynamicMft.Validate())
			mft := dynamicMft.Manifest()

			envConfig := &manifest.Environment{
				Workload: manifest.Workload{
					Name: &envName,
				},
			}
			envConfig.HTTPConfig.Private.Certificates = tc.EnvImportedCertARNs
			var opts []stack.BackendServiceOption
			if tc.ImportedALB != nil {
				opts = append(opts, stack.WithImportedInternalALB(tc.ImportedALB))
			}
			serializer, err := stack.NewBackendService(stack.BackendServiceConfig{
				App: &config.Application{
					Name: appName,
				},
				EnvManifest:        envConfig,
				ArtifactBucketName: "bucket",
				ArtifactKey:        "arn:aws:kms:us-west-2:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
				Manifest:           mft.(*manifest.BackendService),
				RuntimeConfig: stack.RuntimeConfig{
					ServiceDiscoveryEndpoint: fmt.Sprintf("%s.%s.local", envName, appName),
					EnvVersion:               "v1.42.0",
					Version:                  "v1.29.0",
				},
			}, opts...)
			require.NoError(t, err)

			// validate generated template
			tmpl, err := serializer.Template()
			require.NoError(t, err)
			assertTemplateFixture(t, tc.TemplatePath, tmpl)

			// validate generated params
			params, err := serializer.SerializedParameters()
			require.NoError(t, err)
			assertParamsFixture(t, tc.ParamsPath, params)
		})
	}
}
