// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package legacytools

import "testing"

const resourceKindComponent = "Component"

// resourceToolSpecs returns test specs for resource toolset
func resourceToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "apply_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"apply", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"resource"},
			testArgs: map[string]any{
				"resource": map[string]interface{}{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       resourceKindComponent,
					"metadata": map[string]interface{}{
						"name": "test-component",
					},
				},
			},
			expectedMethod: "ApplyResource",
			validateCall: func(t *testing.T, args []interface{}) {
				resource := args[0].(map[string]interface{})
				if resource["kind"] != resourceKindComponent {
					t.Errorf("Expected resource kind %q, got %v", resourceKindComponent, resource["kind"])
				}
			},
		},
		{
			name:                "get_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"get", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"kind", "resource_name"},
			optionalParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"kind":           resourceKindComponent,
				"resource_name":  testComponentName,
			},
			expectedMethod: "GetResource",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != resourceKindComponent {
					t.Errorf("Expected kind %q, got %v", resourceKindComponent, args[1])
				}
				if args[2] != testComponentName {
					t.Errorf("Expected resource name %q, got %v", testComponentName, args[2])
				}
			},
		},
		{
			name:                "delete_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"delete", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"resource"},
			testArgs: map[string]any{
				"resource": map[string]interface{}{
					"apiVersion": "openchoreo.dev/v1alpha1",
					"kind":       resourceKindComponent,
					"metadata": map[string]interface{}{
						"name": "test-component",
					},
				},
			},
			expectedMethod: "DeleteResource",
			validateCall: func(t *testing.T, args []interface{}) {
				resource := args[0].(map[string]interface{})
				if resource["kind"] != resourceKindComponent {
					t.Errorf("Expected resource kind %q, got %v", resourceKindComponent, resource["kind"])
				}
			},
		},
	}
}
