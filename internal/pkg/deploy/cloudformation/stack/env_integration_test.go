//go:build integration || localintegration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aproint/copilot-cli/internal/pkg/manifest"

	"gopkg.in/yaml.v3"

	"github.com/aproint/copilot-cli/internal/pkg/deploy"
	"github.com/aproint/copilot-cli/internal/pkg/deploy/cloudformation/stack"
	"github.com/stretchr/testify/require"
)

func TestEnvStack_Template(t *testing.T) {
	testCases := map[string]struct {
		input          *stack.EnvConfig
		wantedFileName string
	}{
		"generate template with embedded manifest file with container insights and cloudfront imported bucket and advanced access logs": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
# Create the public ALB with certificates attached.
cdn:
  certificate: viewer-cert
  static_assets:
    location: cf-s3-ecs-demo-bucket.s3.us-west-2.amazonaws.com
    alias: example.com
    path: static/*
http:
  public:
    ingress:
      cdn: true
      source_ips:
        - 1.1.1.1
        - 2.2.2.2
    access_logs:
      bucket_name: accesslogsbucket
      prefix: accesslogsbucketprefix
    certificates:
      - cert-1
      - cert-2
  private:
    security_groups:
      ingress:
        from_vpc: true
observability:
  container_insights: true # Enable container insights.`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					CIDRPrefixListIDs:    []string{"pl-mockid"},
					PublicALBSourceIPs:   []string{"1.1.1.1", "2.2.2.2"},
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			wantedFileName: "template-with-cloudfront-observability.yml",
		},
		"generate template with default access logs": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
http:
  public:
    access_logs: true`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			wantedFileName: "template-with-default-access-log-config.yml",
		},
		"generate template with embedded manifest file with custom security groups rules added by the customer": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
# Create the public ALB with certificates attached.
http:
  public:
    certificates:
      - cert-1
      - cert-2
observability:
  container_insights: true # Enable container insights.
network:
  vpc:
    security_group:
      ingress:
        - ip_protocol: tcp
          ports: 10
          cidr: 0.0.0.0
        - ip_protocol: tcp
          ports: 1-10
          cidr: 0.0.0.0
      egress:
        - ip_protocol: tcp
          ports: 0-65535
          cidr: 0.0.0.0`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),

			wantedFileName: "template-with-custom-security-group.yml",
		},
		"generate template with embedded manifest file with imported certificates and SSL Policy and empty security groups rules added by the customer": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
# Create the public ALB with certificates attached.
http:
  public:
    certificates:
      - cert-1
      - cert-2
    ssl_policy: ELBSecurityPolicy-FS-1-1-2019-08
observability:
  container_insights: true # Enable container insights.
security_group:
  ingress:
  egress:`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),

			wantedFileName: "template-with-imported-certs-sslpolicy-custom-empty-security-group.yml",
		},
		"generate template with custom resources": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			wantedFileName: "template-with-basic-manifest.yml",
		},
		"generate template with default vpc and flowlogs is on": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
network:
  vpc:
    flow_logs:
     retention: 60`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			wantedFileName: "template-with-defaultvpc-flowlogs.yml",
		},
		"generate template with imported vpc and flowlogs is on": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
network:
  vpc:
    id: 'vpc-12345'
    subnets:
      public:
        - id: 'subnet-11111'
        - id: 'subnet-22222'
      private:
        - id: 'subnet-33333'
        - id: 'subnet-44444'
    flow_logs: on`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			wantedFileName: "template-with-importedvpc-flowlogs.yml",
		},
		"imported vpc with selected private ALB subnets": {
			input: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
network:
  vpc:
    id: vpc-12345
    subnets:
      public:
        - id: subnet-public-a
        - id: subnet-public-b
      private:
        - id: subnet-private-a
        - id: subnet-private-b
        - id: subnet-private-c
http:
  private:
    subnets:
      - subnet-private-a
      - subnet-private-c
    certificates:
      - arn:aws:acm:us-west-2:111122223333:certificate/private-a
      - arn:aws:acm:us-west-2:111122223333:certificate/private-b
    ssl_policy: ELBSecurityPolicy-FS-1-2-Res-2019-08
    ingress:
      vpc: true`
				var mft manifest.Environment
				require.NoError(t, yaml.Unmarshal([]byte(rawMft), &mft))
				require.NoError(t, mft.Validate())
				return &stack.EnvConfig{
					Version: "1.x",
					App:     deploy.AppInformation{AccountPrincipalARN: "arn:aws:iam::000000000:root", Name: "demo"},
					Name:    "test", ArtifactBucketARN: "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft, RawMft: rawMft,
				}
			}(),
			wantedFileName: "template-with-importedvpc-private-alb.yml",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// GIVEN
			// WHEN
			envStack, err := stack.NewEnvStackConfig(tc.input)
			require.NoError(t, err)
			actual, err := envStack.Template()
			require.NoError(t, err, "serialize template")
			assertEnvTemplateFixture(t, filepath.Join("testdata", "environments", tc.wantedFileName), actual)
			params, err := envStack.SerializedParameters()
			require.NoError(t, err)
			assertParamsFixture(t, filepath.Join("testdata", "environments", strings.TrimSuffix(tc.wantedFileName, ".yml")+".params.json"), params)
		})
	}
}

func TestEnvStack_Regression(t *testing.T) {
	testCases := map[string]struct {
		originalManifest *stack.EnvConfig
		newManifest      *stack.EnvConfig
	}{
		"should produce the same template after migrating load balancer ingress fields": {
			originalManifest: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
# Create the public ALB with certificates attached.
cdn:
  certificate: viewer-cert
http:
  public:
    security_groups:
      ingress:
        restrict_to:
          cdn: true
    access_logs:
      bucket_name: accesslogsbucket
      prefix: accesslogsbucketprefix
    certificates:
      - cert-1
      - cert-2
  private:
    security_groups:
      ingress:
        from_vpc: true
observability:
  container_insights: true # Enable container insights.`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					CIDRPrefixListIDs:    []string{"pl-mockid"},
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
			newManifest: func() *stack.EnvConfig {
				rawMft := `name: test
type: Environment
# Create the public ALB with certificates attached.
cdn:
  certificate: viewer-cert
http:
  public:
    ingress:
      cdn: true
    access_logs:
      bucket_name: accesslogsbucket
      prefix: accesslogsbucketprefix
    certificates:
      - cert-1
      - cert-2
  private:
    ingress:
      vpc: true
observability:
  container_insights: true # Enable container insights.`
				var mft manifest.Environment
				err := yaml.Unmarshal([]byte(rawMft), &mft)
				require.NoError(t, err)
				return &stack.EnvConfig{
					Version: "1.x",
					App: deploy.AppInformation{
						AccountPrincipalARN: "arn:aws:iam::000000000:root",
						Name:                "demo",
					},
					Name:                 "test",
					CIDRPrefixListIDs:    []string{"pl-mockid"},
					ArtifactBucketARN:    "arn:aws:s3:::mockbucket",
					ArtifactBucketKeyARN: "arn:aws:kms:us-west-2:000000000:key/1234abcd-12ab-34cd-56ef-1234567890ab",
					Mft:                  &mft,
					RawMft:               rawMft,
				}
			}(),
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			// WHEN
			originalStack, err := stack.NewEnvStackConfig(tc.originalManifest)
			require.NoError(t, err)
			originalTmpl, err := originalStack.Template()
			require.NoError(t, err, "should serialize the template given the original environment manifest")
			originalObj := make(map[any]any)
			require.NoError(t, yaml.Unmarshal([]byte(originalTmpl), originalObj))

			newStack, err := stack.NewEnvStackConfig(tc.newManifest)
			require.NoError(t, err)
			newTmpl, err := newStack.Template()
			require.NoError(t, err, "should serialize the template given a migrated environment manifest")
			newObj := make(map[any]any)
			require.NoError(t, yaml.Unmarshal([]byte(newTmpl), newObj))

			// Delete because manifest could be different.
			delete(originalObj["Metadata"].(map[string]any), "Manifest")
			delete(newObj["Metadata"].(map[string]any), "Manifest")

			resetCustomResourceLocations(originalObj)
			resetCustomResourceLocations(newObj)
			compareStackTemplate(t, originalObj, newObj)
		})
	}
}

func compareStackTemplate(t *testing.T, wantedObj, actualObj map[any]any) {
	t.Helper()
	if !stackTemplatesEqual(wantedObj, actualObj) {
		require.Equal(t, wantedObj, actualObj, "complete CloudFormation template")
	}
}

func stackTemplatesEqual(wantedObj, actualObj map[any]any) bool {
	return reflect.DeepEqual(wantedObj, actualObj)
}

func resetCustomResourceLocations(template map[any]any) {
	resources := template["Resources"].(map[string]any)
	functions := []string{
		"EnvControllerFunction", "DynamicDesiredCountFunction", "BacklogPerTaskCalculatorFunction",
		"RulePriorityFunction", "NLBCustomDomainFunction", "NLBCertValidatorFunction",
		"CustomDomainFunction", "CertificateValidationFunction", "DNSDelegationFunction",
		"CertificateReplicatorFunction", "UniqueJSONValuesFunction", "TriggerStateMachineFunction", "BucketCleanerFunction",
	}
	for _, fnName := range functions {
		resource, ok := resources[fnName]
		if !ok {
			continue
		}
		fn := resource.(map[string]any)
		props := fn["Properties"].(map[string]any)
		delete(props, "Code")
	}
}
