// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testEndpointTypeName = "http-route"

// endpointEndpointTypeSpecs returns specs for the (Cluster)EndpointType read tools
// surfaced through the dev-facing endpoint toolset.
func endpointEndpointTypeSpecs() []toolTestSpec {
	return endpointTypeReadSpecs("endpoint")
}

// peEndpointTypeSpecs returns specs for the (Cluster)EndpointType tools surfaced
// through the platform-engineering toolset: reads (dual-registered) plus writes
// and the creation-schema getter.
func peEndpointTypeSpecs() []toolTestSpec {
	specs := endpointTypeReadSpecs("pe")
	specs = append(specs, []toolTestSpec{
		{
			name:                "get_endpoint_type_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "endpoint", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "create_endpoint_type",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "endpoint", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-endpoint-type",
				"spec":           map[string]any{"endpoints": []any{}},
			},
			expectedMethod: "CreateEndpointType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_endpoint_type",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "endpoint", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-endpoint-type",
				"spec":           map[string]any{"endpoints": []any{}},
			},
			expectedMethod: "UpdateEndpointType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_endpoint_type",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "endpoint", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-endpoint-type",
			},
			expectedMethod: "DeleteEndpointType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-endpoint-type" {
					t.Errorf("Expected (%s, my-endpoint-type), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
	}...)
	return specs
}

// endpointTypeReadSpecs returns the three scope-collapsed read specs (list, get,
// get_schema) tagged with the given toolset. Read tools are dual-registered in
// both `endpoint` and `pe`; the specs for each toolset differ only in the
// `toolset` field.
func endpointTypeReadSpecs(toolset string) []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_endpoint_types",
			toolset:             toolset,
			descriptionKeywords: []string{"list", "endpoint", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListEndpointTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_endpoint_type",
			toolset:             toolset,
			descriptionKeywords: []string{"endpoint", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointTypeName,
			},
			expectedMethod: "GetEndpointType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointTypeName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_endpoint_type_schema",
			toolset:             toolset,
			descriptionKeywords: []string{"endpoint", "type", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointTypeName,
			},
			expectedMethod: "GetEndpointTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointTypeName, args[0], args[1])
				}
			},
		},
	}
}
