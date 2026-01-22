// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// namespaceToolSpecs returns test specs for namespace toolset
func namespaceToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_namespaces",
			toolset:             "namespace",
			descriptionKeywords: []string{"namespace"},
			descriptionMinLen:   10,
			requiredParams:      []string{},
			optionalParams:      []string{},
			testArgs:            map[string]any{},
			expectedMethod:      "ListNamespaces",
			validateCall: func(t *testing.T, args []interface{}) {
				// ListNamespaces takes no arguments
				if len(args) != 0 {
					t.Errorf("Expected no arguments for ListNamespaces, got %d", len(args))
				}
			},
		},
		{
			name:                "get_namespace",
			toolset:             "namespace",
			descriptionKeywords: []string{"namespace"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			optionalParams:      []string{},
			testArgs:            map[string]any{"name": "test-namespace"},
			expectedMethod:      "GetNamespace",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "test-namespace" {
					t.Errorf("Expected namespace 'test-namespace', got %v", args[0])
				}
			},
		},
		{
			name:                "list_secret_references",
			toolset:             "namespace",
			descriptionKeywords: []string{"list", "secret", "reference"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
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
