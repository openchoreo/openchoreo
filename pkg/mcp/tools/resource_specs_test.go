// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testResourceName = "analytics-shared-db"

// resourceToolSpecs returns the dev-facing resource toolset specs: Resource CRUD
// plus the scope-collapsed read tools over (Cluster)ResourceType. Mirrors the
// component toolset's mix of primary-CRD CRUD + type reads.
func resourceToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 8)
	specs = append(specs, resourceCRUDSpecs()...)
	specs = append(specs, resourceResourceTypeSpecs()...)
	return specs
}

// resourceCRUDSpecs returns test specs for the Resource CRUD surface.
func resourceCRUDSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_resources",
			toolset:             "resource",
			descriptionKeywords: []string{"list", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "ListResources",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceName,
			},
			expectedMethod: "GetResource",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"create", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "name", "type_name"},
			optionalParams:      []string{"type_kind", "parameters", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"name":           testResourceName,
				"type_name":      "postgres",
			},
			expectedMethod: "CreateResource",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "update_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"update", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"parameters", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceName,
				"parameters":     map[string]any{"version": "8.0"},
			},
			expectedMethod: "UpdateResource",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_resource",
			toolset:             "resource",
			descriptionKeywords: []string{"delete", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceName,
			},
			expectedMethod: "DeleteResource",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceName, args[0], args[1])
				}
			},
		},
	}
}
