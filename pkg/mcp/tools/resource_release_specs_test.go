// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testResourceReleaseName = "analytics-shared-db-abc123"

// peResourceReleaseSpecs returns ResourceRelease list/create/get specs
// (PE-controlled administrative operations).
func peResourceReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_resource_releases",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "resource", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "resource_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"resource_name":  testResourceName,
			},
			expectedMethod: "ListResourceReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_resource_release",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "resource", "release"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"namespace_name", "name", "project_name", "resource_name", "type_name", "type_spec",
			},
			optionalParams: []string{"type_kind", "parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseName,
				"project_name":   testProjectName,
				"resource_name":  testResourceName,
				"type_name":      "postgres",
				"type_spec":      map[string]any{"resources": []any{}},
			},
			expectedMethod: "CreateResourceRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_resource_release",
			toolset:             "pe",
			descriptionKeywords: []string{"resource", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseName,
			},
			expectedMethod: "GetResourceRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceReleaseName, args[0], args[1])
				}
			},
		},
	}
}

// deploymentResourceReleaseSpecs returns the dev-side delete spec (mirrors
// delete_component_release on the deployment toolset).
func deploymentResourceReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "delete_resource_release",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "resource", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseName,
			},
			expectedMethod: "DeleteResourceRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceReleaseName, args[0], args[1])
				}
			},
		},
	}
}
