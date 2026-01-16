// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// projectToolSpecs returns test specs for project toolset
func projectToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_projects",
			toolset:             "project",
			descriptionKeywords: []string{"list", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListProjects",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected org name %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_project",
			toolset:             "project",
			descriptionKeywords: []string{"project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			testArgs: map[string]any{
				"namespace_name":     testNamespaceName,
				"project_name": testProjectName,
			},
			expectedMethod: "GetProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected org name %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != testProjectName {
					t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
				}
			},
		},
		{
			name:                "create_project",
			toolset:             "project",
			descriptionKeywords: []string{"create", "project"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"description"},
			testArgs: map[string]any{
				"namespace_name":    testNamespaceName,
				"name":        "new-project",
				"description": "test project",
			},
			expectedMethod: "CreateProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected org name %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateProjectRequest
			},
		},
	}
}
