// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// schemaToolSpecs returns test specs for schema toolset
func schemaToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "explain_schema",
			toolset:             "schema",
			descriptionKeywords: []string{"schema", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"kind"},
			optionalParams:      []string{"path"},
			testArgs: map[string]any{
				"kind": "Component",
				"path": "spec",
			},
			expectedMethod: "ExplainSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "Component" || args[1] != "spec" {
					t.Errorf("Expected (Component, spec), got (%v, %v)", args[0], args[1])
				}
			},
		},
	}
}
