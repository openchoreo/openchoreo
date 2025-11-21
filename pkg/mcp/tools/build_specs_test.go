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
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetBuildObserverURL",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "list_build_templates",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build", "template"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListBuildTemplates",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "trigger_build",
			toolset:             "build",
			descriptionKeywords: []string{"trigger", "build"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "commit"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"commit":         "abc123",
			},
			expectedMethod: "TriggerBuild",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "abc123" {
					t.Errorf("Expected (%s, %s, %s, abc123), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "list_builds",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListBuilds",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "list_buildplanes",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
	}
}
