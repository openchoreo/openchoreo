// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testProjectReleaseName = "my-project-abc123"

// peProjectReleaseSpecs returns ProjectRelease list/create/get specs
// (PE-controlled administrative operations).
func peProjectReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_project_releases",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "project", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "ListProjectReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_project_release",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "project", "release"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"namespace_name", "name", "project_name", "type_name", "type_spec",
			},
			optionalParams: []string{"type_kind", "parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseName,
				"project_name":   testProjectName,
				"type_name":      "standard-project",
				"type_spec":      map[string]any{"resources": []any{}},
			},
			expectedMethod: "CreateProjectRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_project_release",
			toolset:             "pe",
			descriptionKeywords: []string{"project", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseName,
			},
			expectedMethod: "GetProjectRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectReleaseName, args[0], args[1])
				}
			},
		},
	}
}

// deploymentProjectReleaseSpecs returns the dev-side delete spec (mirrors
// delete_resource_release on the deployment toolset).
func deploymentProjectReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "delete_project_release",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "project", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectReleaseName,
			},
			expectedMethod: "DeleteProjectRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectReleaseName, args[0], args[1])
				}
			},
		},
	}
}
