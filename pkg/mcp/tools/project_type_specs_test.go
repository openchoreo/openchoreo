// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testProjectTypeName = "standard-project"

// projectProjectTypeSpecs returns specs for the (Cluster)ProjectType read tools
// surfaced through the dev-facing project toolset.
func projectProjectTypeSpecs() []toolTestSpec {
	return projectTypeReadSpecs("project")
}

// peProjectTypeSpecs returns specs for the (Cluster)ProjectType tools surfaced
// through the platform-engineering toolset: reads (dual-registered) plus writes
// and the creation-schema getter.
func peProjectTypeSpecs() []toolTestSpec {
	specs := projectTypeReadSpecs("pe")
	specs = append(specs, []toolTestSpec{
		{
			name:                "get_project_type_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "project", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "create_project_type",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "project", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-project-type",
				"spec":           map[string]any{"resources": []any{}},
			},
			expectedMethod: "CreateProjectType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_project_type",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "project", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-project-type",
				"spec":           map[string]any{"resources": []any{}},
			},
			expectedMethod: "UpdateProjectType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_project_type",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "project", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-project-type",
			},
			expectedMethod: "DeleteProjectType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-project-type" {
					t.Errorf("Expected (%s, my-project-type), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
	}...)
	return specs
}

// projectTypeReadSpecs returns the three scope-collapsed read specs (list, get,
// get_schema) tagged with the given toolset. Read tools are dual-registered in
// both `project` and `pe`; the specs for each toolset differ only in the
// `toolset` field.
func projectTypeReadSpecs(toolset string) []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_project_types",
			toolset:             toolset,
			descriptionKeywords: []string{"list", "project", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListProjectTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_project_type",
			toolset:             toolset,
			descriptionKeywords: []string{"project", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectTypeName,
			},
			expectedMethod: "GetProjectType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectTypeName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_project_type_schema",
			toolset:             toolset,
			descriptionKeywords: []string{"project", "type", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testProjectTypeName,
			},
			expectedMethod: "GetProjectTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectTypeName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testProjectTypeName, args[0], args[1])
				}
			},
		},
	}
}
