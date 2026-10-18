// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testEndpointReleaseBindingName = "orders-api-development"

// deploymentEndpointReleaseBindingSpecs returns specs for EndpointReleaseBinding
// tools surfaced through the deployment toolset (mirrors ReleaseBinding).
func deploymentEndpointReleaseBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_endpoint_release_bindings",
			toolset:             "deployment",
			descriptionKeywords: []string{"list", "endpoint", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "endpoint_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"endpoint_name":  testEndpointName,
			},
			expectedMethod: "ListEndpointReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_endpoint_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"endpoint", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointReleaseBindingName,
			},
			expectedMethod: "GetEndpointReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointReleaseBindingName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_endpoint_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"create", "endpoint", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "project_name", "endpoint_name", "environment"},
			optionalParams:      []string{"endpoint_release", "endpoint_type_environment_configs"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointReleaseBindingName,
				"project_name":   testProjectName,
				"endpoint_name":  testEndpointName,
				"environment":    "development",
			},
			expectedMethod: "CreateEndpointReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_endpoint_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"update", "endpoint", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"endpoint_release", "endpoint_type_environment_configs"},
			testArgs: map[string]any{
				"namespace_name":   testNamespaceName,
				"name":             testEndpointReleaseBindingName,
				"endpoint_release": testEndpointReleaseName,
			},
			expectedMethod: "UpdateEndpointReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_endpoint_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "endpoint", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointReleaseBindingName,
			},
			expectedMethod: "DeleteEndpointReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointReleaseBindingName, args[0], args[1])
				}
			},
		},
	}
}
