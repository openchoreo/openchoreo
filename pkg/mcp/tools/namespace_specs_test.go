// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// namespaceToolSpecs returns test specs for namespace toolset
func namespaceToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_namespaces",
			toolset:             "namespace",
			descriptionKeywords: []string{"namespace"},
			descriptionMinLen:   10,
			requiredParams:      []string{},
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListNamespaces",
			validateCall: func(t *testing.T, args []interface{}) {
				// ListNamespaces takes only ListOpts
				if len(args) != 1 {
					t.Errorf("Expected 1 argument (ListOpts) for ListNamespaces, got %d", len(args))
				}
			},
		},
		{
			name:                "create_namespace",
			toolset:             "namespace",
			descriptionKeywords: []string{"create", "namespace"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name":         "new-namespace",
				"display_name": "New Namespace",
				"description":  "test namespace",
			},
			expectedMethod: "CreateNamespace",
			validateCall: func(t *testing.T, args []interface{}) {
				t.Helper()
				if len(args) != 1 {
					t.Errorf("Expected 1 argument for CreateNamespace, got %d", len(args))
					return
				}
				req, ok := args[0].(*gen.CreateNamespaceJSONRequestBody)
				if !ok {
					t.Errorf("Expected args[0] to be *gen.CreateNamespaceJSONRequestBody, got %T", args[0])
					return
				}
				if req.Metadata.Name != "new-namespace" {
					t.Errorf("Expected Metadata.Name %q, got %q", "new-namespace", req.Metadata.Name)
				}
			},
		},
		{
			name:                "list_secret_references",
			toolset:             "namespace",
			descriptionKeywords: []string{"list", "secret", "reference"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListSecretReferences",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
	}
}
