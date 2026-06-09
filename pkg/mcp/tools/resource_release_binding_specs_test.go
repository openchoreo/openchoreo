// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testResourceReleaseBindingName = "analytics-shared-db-dev"

// deploymentResourceReleaseBindingSpecs returns specs for ResourceReleaseBinding
// tools surfaced through the deployment toolset (mirrors ReleaseBinding).
func deploymentResourceReleaseBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_resource_release_bindings",
			toolset:             "deployment",
			descriptionKeywords: []string{"list", "resource", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "resource_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"resource_name":  testResourceName,
			},
			expectedMethod: "ListResourceReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_resource_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"resource", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseBindingName,
			},
			expectedMethod: "GetResourceReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceReleaseBindingName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_resource_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"create", "resource", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "project_name", "resource_name", "environment"},
			optionalParams:      []string{"resource_release", "retain_policy", "resource_type_environment_configs"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseBindingName,
				"project_name":   testProjectName,
				"resource_name":  testResourceName,
				"environment":    "development",
			},
			expectedMethod: "CreateResourceReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_resource_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"update", "resource", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"resource_release", "retain_policy", "resource_type_environment_configs"},
			testArgs: map[string]any{
				"namespace_name":   testNamespaceName,
				"name":             testResourceReleaseBindingName,
				"resource_release": testResourceReleaseName,
			},
			expectedMethod: "UpdateResourceReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_resource_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "resource", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseBindingName,
			},
			expectedMethod: "DeleteResourceReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceReleaseBindingName, args[0], args[1])
				}
			},
		},
	}
}
