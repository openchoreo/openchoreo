// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// buildToolSpecs returns test specs for build toolset
func buildToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_build_observer_url",
			toolset:             "build",
			descriptionKeywords: []string{"observability", "build"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetBuildObserverURL",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "list_build_templates",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build", "template"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListBuildTemplates",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "trigger_build",
			toolset:             "build",
			descriptionKeywords: []string{"trigger", "build"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "commit"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"commit":         "abc123",
			},
			expectedMethod: "TriggerBuild",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "abc123" {
					t.Errorf("Expected (%s, %s, %s, abc123), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "list_builds",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListBuilds",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "list_buildplanes",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
	}
}
