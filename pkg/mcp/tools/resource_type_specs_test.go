// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testResourceTypeName = "postgres"

// resourceResourceTypeSpecs returns specs for the (Cluster)ResourceType read tools
// surfaced through the dev-facing resource toolset.
func resourceResourceTypeSpecs() []toolTestSpec {
	return resourceTypeReadSpecs("resource")
}

// peResourceTypeSpecs returns specs for the (Cluster)ResourceType tools surfaced
// through the platform-engineering toolset: reads (dual-registered) plus writes
// and the creation-schema getter.
func peResourceTypeSpecs() []toolTestSpec {
	specs := resourceTypeReadSpecs("pe")
	specs = append(specs, []toolTestSpec{
		{
			name:                "get_resource_type_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "resource", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "create_resource_type",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "resource", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-resource-type",
				"spec":           map[string]any{"resources": []any{}},
			},
			expectedMethod: "CreateResourceType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_resource_type",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "resource", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-resource-type",
				"spec":           map[string]any{"resources": []any{}},
			},
			expectedMethod: "UpdateResourceType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_resource_type",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "resource", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-resource-type",
			},
			expectedMethod: "DeleteResourceType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-resource-type" {
					t.Errorf("Expected (%s, my-resource-type), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
	}...)
	return specs
}

// resourceTypeReadSpecs returns the three scope-collapsed read specs (list, get,
// get_schema) tagged with the given toolset. Read tools are dual-registered in
// both `resource` and `pe`; the specs for each toolset differ only in the
// `toolset` field.
func resourceTypeReadSpecs(toolset string) []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_resource_types",
			toolset:             toolset,
			descriptionKeywords: []string{"list", "resource", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListResourceTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_resource_type",
			toolset:             toolset,
			descriptionKeywords: []string{"resource", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceTypeName,
			},
			expectedMethod: "GetResourceType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceTypeName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_resource_type_schema",
			toolset:             toolset,
			descriptionKeywords: []string{"resource", "type", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceTypeName,
			},
			expectedMethod: "GetResourceTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testResourceTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testResourceTypeName, args[0], args[1])
				}
			},
		},
	}
}
