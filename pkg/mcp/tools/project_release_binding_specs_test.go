// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testProjectReleaseBindingName = "my-project-dev"

// deploymentProjectReleaseBindingSpecs returns specs for ProjectReleaseBinding
// tools surfaced through the deployment toolset (mirrors ResourceReleaseBinding).
func deploymentProjectReleaseBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_project_release_bindings",
			toolset:             "deployment",
			descriptionKeywords: []string{"list", "project", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "ListProjectReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_project_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"project", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseBindingName,
			},
			expectedMethod: "GetProjectReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectReleaseBindingName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_project_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"create", "project", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "project_name", "environment"},
			optionalParams:      []string{"project_release", "environment_configs"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseBindingName,
				"project_name":   testProjectName,
				"environment":    "development",
			},
			expectedMethod: "CreateProjectReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_project_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"update", "project", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"project_release", "environment_configs"},
			testArgs: map[string]any{
				"namespace_name":  testNamespaceName,
				"name":            testProjectReleaseBindingName,
				"project_release": testProjectReleaseName,
			},
			expectedMethod: "UpdateProjectReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_project_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "project", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseBindingName,
			},
			expectedMethod: "DeleteProjectReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectReleaseBindingName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectReleaseBindingName, args[0], args[1])
				}
			},
		},
	}
}
