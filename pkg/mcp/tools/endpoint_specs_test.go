// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const testEndpointName = "orders-api"

// endpointToolSpecs returns the dev-facing endpoint toolset specs: Endpoint CRUD
// plus the scope-collapsed read tools over (Cluster)EndpointType. Mirrors the
// resource toolset.
func endpointToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 8)
	specs = append(specs, endpointCRUDSpecs()...)
	specs = append(specs, endpointEndpointTypeSpecs()...)
	return specs
}

// endpointCRUDSpecs returns test specs for the Endpoint CRUD surface.
func endpointCRUDSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_endpoints",
			toolset:             "endpoint",
			descriptionKeywords: []string{"list", "endpoint"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "ListEndpoints",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_endpoint",
			toolset:             "endpoint",
			descriptionKeywords: []string{"endpoint"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
			},
			expectedMethod: "GetEndpoint",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_endpoint",
			toolset:             "endpoint",
			descriptionKeywords: []string{"create", "endpoint"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"namespace_name", "project_name", "name", "type_name", "visibility", "backend_refs",
			},
			optionalParams: []string{"type_kind", "parameters", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"name":           testEndpointName,
				"type_name":      testEndpointTypeName,
				"visibility":     []any{"external"},
				"backend_refs": []any{
					map[string]any{"name": "primary", "component": "orders", "endpoint": "http", "weight": 100},
				},
			},
			expectedMethod: "CreateEndpoint",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "update_endpoint",
			toolset:             "endpoint",
			descriptionKeywords: []string{"update", "endpoint"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"visibility", "backend_refs", "parameters", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
				"visibility":     []any{"internal", "external"},
				"backend_refs": []any{
					map[string]any{"name": "stable", "component": "greeter", "endpoint": "http", "weight": 100},
					map[string]any{
						"name": "canary", "component": "greeter-v2", "endpoint": "http", "weight": 0,
						"match": map[string]any{"headers": []any{map[string]any{"name": "x-canary", "value": "true"}}},
					},
					map[string]any{
						"name": "shadow", "component": "greeter-v3", "endpoint": "http", "weight": 0,
						"mirror": map[string]any{"percent": 10},
					},
				},
			},
			expectedMethod: "UpdateEndpoint",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				body, ok := args[1].(*gen.UpdateEndpointJSONRequestBody)
				if !ok || body.Spec == nil || len(body.Spec.BackendRefs) != 3 {
					t.Fatalf("Expected three backend refs in the body, got %#v", args[1])
				}
				refs := body.Spec.BackendRefs
				if refs[1].Match == nil || (*refs[1].Match.Headers)[0].Name != "x-canary" {
					t.Errorf("Expected the canary's match to be kept, got %#v", refs[1].Match)
				}
				if refs[2].Mirror == nil || refs[2].Mirror.Percent != 10 {
					t.Errorf("Expected the shadow's mirror to be kept, got %#v", refs[2].Mirror)
				}
			},
		},
		{
			name:                "delete_endpoint",
			toolset:             "endpoint",
			descriptionKeywords: []string{"delete", "endpoint"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
			},
			expectedMethod: "DeleteEndpoint",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointName, args[0], args[1])
				}
			},
		},
	}
}
