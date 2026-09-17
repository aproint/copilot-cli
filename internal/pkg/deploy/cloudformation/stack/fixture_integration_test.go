//go:build integration || localintegration

// Copyright APROINT, s.r.o.
// SPDX-License-Identifier: Apache-2.0

package stack_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// fixturePath is deliberately limited to fixtures owned by the stack package.
func fixturePath(t *testing.T, path string) {
	t.Helper()
	clean := filepath.Clean(path)
	require.True(t, strings.HasPrefix(clean, "testdata"+string(filepath.Separator)), "fixture must be in stack/testdata: %s", path)
}

func updateFixtures(t *testing.T) bool {
	t.Helper()
	if os.Getenv("UPDATE_FIXTURES") != "1" {
		return false
	}
	require.Empty(t, os.Getenv("CI"), "UPDATE_FIXTURES is disabled in CI")
	return true
}

func yamlMap(t *testing.T, data []byte) map[any]any {
	t.Helper()
	var value map[any]any
	require.NoError(t, yaml.Unmarshal(data, &value))
	return value
}

// normalizeTemplate removes only generated values whose content cannot be checked reliably.
func normalizeTemplate(value map[any]any) {
	resetCustomResourceLocations(value)
	if metadata, ok := value["Metadata"].(map[string]any); ok {
		if manifest, ok := metadata["Manifest"].(string); ok {
			metadata["Manifest"] = strings.TrimSpace(manifest)
		}
	}
	resources, ok := value["Resources"].(map[string]any)
	if !ok {
		return
	}
	if action, ok := resources["DynamicDesiredCountAction"].(map[string]any); ok {
		if props, ok := action["Properties"].(map[string]any); ok {
			if _, exists := props["UpdateID"]; exists {
				props["UpdateID"] = "AVeryRandomUUID"
			}
		}
	}
}

// normalizeYAMLNode keeps CloudFormation's !Ref and other intrinsic tags intact.
func normalizeYAMLNode(node *yaml.Node) {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		normalizeYAMLNode(node.Content[0])
		return
	}
	if node.Kind != yaml.MappingNode {
		return
	}
	resources := yamlValue(node, "Resources")
	if resources == nil {
		return
	}
	for _, name := range []string{
		"EnvControllerFunction", "DynamicDesiredCountFunction", "BacklogPerTaskCalculatorFunction",
		"RulePriorityFunction", "NLBCustomDomainFunction", "NLBCertValidatorFunction",
		"CustomDomainFunction", "CertificateValidationFunction", "DNSDelegationFunction",
		"CertificateReplicatorFunction", "UniqueJSONValuesFunction", "TriggerStateMachineFunction", "BucketCleanerFunction",
	} {
		if fn := yamlValue(resources, name); fn != nil {
			if props := yamlValue(fn, "Properties"); props != nil {
				yamlDelete(props, "Code")
			}
		}
	}
	if action := yamlValue(resources, "DynamicDesiredCountAction"); action != nil {
		if props := yamlValue(action, "Properties"); props != nil {
			if id := yamlValue(props, "UpdateID"); id != nil {
				id.Value = "AVeryRandomUUID"
			}
		}
	}
}

func yamlValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlDelete(node *yaml.Node, key string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = append(node.Content[:i], node.Content[i+2:]...)
			return
		}
	}
}

func assertTemplateFixture(t *testing.T, path, rendered string) {
	t.Helper()
	fixturePath(t, path)
	actual := yamlMap(t, []byte(rendered))
	normalizeTemplate(actual)
	if updateFixtures(t) {
		existing, err := os.ReadFile(path)
		var previous map[any]any
		if err == nil {
			previous = yamlMap(t, existing)
			normalizeTemplate(previous)
		}
		if os.IsNotExist(err) || (err == nil && !stackTemplatesEqual(previous, actual)) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(rendered), &node))
			normalizeYAMLNode(&node)
			data, err := yaml.Marshal(&node)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0644))
		} else {
			require.NoError(t, err)
		}
	}
	wantedBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	wanted := yamlMap(t, wantedBytes)
	normalizeTemplate(wanted)
	compareStackTemplate(t, wanted, actual)
}

func assertEnvTemplateFixture(t *testing.T, path, rendered string) {
	t.Helper()
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(rendered), &node))
	root := node.Content[0]
	if metadata := yamlValue(root, "Metadata"); metadata != nil {
		yamlDelete(metadata, "Version")
		if manifest := yamlValue(metadata, "Manifest"); manifest != nil {
			manifest.Value = strings.TrimSpace(manifest.Value)
		}
	}
	data, err := yaml.Marshal(&node)
	require.NoError(t, err)
	assertTemplateFixture(t, path, string(data))
}

func assertParamsFixture(t *testing.T, path, rendered string) {
	t.Helper()
	fixturePath(t, path)
	var actual any
	require.NoError(t, json.Unmarshal([]byte(rendered), &actual))
	if updateFixtures(t) {
		existing, err := os.ReadFile(path)
		var previous any
		if err == nil {
			require.NoError(t, json.Unmarshal(existing, &previous))
		}
		if os.IsNotExist(err) || !reflect.DeepEqual(previous, actual) {
			data, err := json.MarshalIndent(actual, "", "  ")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, append(data, '\n'), 0644))
		} else {
			require.NoError(t, err)
		}
	}
	wantedBytes, err := os.ReadFile(path)
	require.NoError(t, err)
	var wanted any
	require.NoError(t, json.Unmarshal(wantedBytes, &wanted))
	require.Equal(t, wanted, actual, "serialized stack parameters")
}

func TestCompareStackTemplateIncludesMappings(t *testing.T) {
	wanted := yamlMap(t, []byte("Mappings:\n  Region: {us-west-2: bucket-a}\nResources: {}\n"))
	actual := yamlMap(t, []byte("Mappings:\n  Region: {us-west-2: bucket-b}\nResources: {}\n"))
	require.False(t, stackTemplatesEqual(wanted, actual), "a changed Mapping must fail comparison")
	delete(actual, "Mappings")
	require.False(t, stackTemplatesEqual(wanted, actual), "a missing Mapping must fail comparison")
}

func TestNormalizeTemplateKeepsMeaningfulProperties(t *testing.T) {
	value := yamlMap(t, []byte("Resources:\n  Service: {Properties: {SecurityGroupIds: [sg-original]}}\n  DynamicDesiredCountAction: {Properties: {UpdateID: generated}}\n"))
	normalizeTemplate(value)
	resources := value["Resources"].(map[string]any)
	require.Equal(t, []any{"sg-original"}, resources["Service"].(map[string]any)["Properties"].(map[string]any)["SecurityGroupIds"])
	require.Equal(t, "AVeryRandomUUID", resources["DynamicDesiredCountAction"].(map[string]any)["Properties"].(map[string]any)["UpdateID"])
}

func TestNormalizeYAMLNodeKeepsIntrinsicTags(t *testing.T) {
	var node yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("Resources:\n  EnvControllerFunction:\n    Properties:\n      Code: generated\n      Role: !GetAtt Role.Arn\n"), &node))
	normalizeYAMLNode(&node)
	data, err := yaml.Marshal(&node)
	require.NoError(t, err)
	require.Contains(t, string(data), "!GetAtt Role.Arn")
	require.NotContains(t, string(data), "Code:")
}
