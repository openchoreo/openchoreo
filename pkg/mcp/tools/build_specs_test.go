// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// buildToolSpecs returns test specs for build toolset
func buildToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 12)
	specs = append(specs, buildWorkflowRunSpecs()...)
	specs = append(specs, buildWorkflowSpecs()...)
	specs = append(specs, buildClusterWorkflowSpecs()...)
	return specs
}

func buildWorkflowRunSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "trigger_workflow_run",
			toolset:             "build",
			descriptionKeywords: []string{"trigger", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"commit"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"commit":         "abc1234",
			},
			expectedMethod: "TriggerWorkflowRun",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "abc1234" {
					t.Errorf("Expected (%s, %s, %s, abc1234), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "create_workflow_run",
			toolset:             "build",
			descriptionKeywords: []string{"create", "workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "build-workflow",
			},
			expectedMethod: "CreateWorkflowRun",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "build-workflow" {
					t.Errorf("Expected (%s, build-workflow), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_workflow_runs",
			toolset:             "build",
			descriptionKeywords: []string{"list", "workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"project_name", "component_name", "limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListWorkflowRuns",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_workflow_run",
			toolset:             "build",
			descriptionKeywords: []string{"workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       testWorkflowRunName,
			},
			expectedMethod: "GetWorkflowRun",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWorkflowRunName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWorkflowRunName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workflow_run_status",
			toolset:             "build",
			descriptionKeywords: []string{"workflow", "run", "status"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       testWorkflowRunName,
			},
			expectedMethod: "GetWorkflowRunStatus",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWorkflowRunName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWorkflowRunName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workflow_run_logs",
			toolset:             "build",
			descriptionKeywords: []string{"workflow", "run", "logs"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			optionalParams:      []string{"task", "since_seconds"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       testWorkflowRunName,
			},
			expectedMethod: "GetWorkflowRunLogs",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWorkflowRunName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWorkflowRunName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workflow_run_events",
			toolset:             "build",
			descriptionKeywords: []string{"workflow", "run", "events"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			optionalParams:      []string{"task"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       testWorkflowRunName,
			},
			expectedMethod: "GetWorkflowRunEvents",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWorkflowRunName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWorkflowRunName, args[0], args[1])
				}
			},
		},
	}
}

func buildWorkflowSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_workflows",
			toolset:             "build",
			descriptionKeywords: []string{"list", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListWorkflows",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_workflow_schema",
			toolset:             "build",
			descriptionKeywords: []string{"workflow", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "build-workflow",
			},
			expectedMethod: "GetWorkflowSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "build-workflow" {
					t.Errorf("Expected (%s, build-workflow), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

func buildClusterWorkflowSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_workflows",
			toolset:             "build",
			descriptionKeywords: []string{"cluster", "workflow"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterWorkflows",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_workflow",
			toolset:             "build",
			descriptionKeywords: []string{"cluster", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": "build-go",
			},
			expectedMethod: "GetClusterWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "build-go" {
					t.Errorf("Expected cwf_name %q, got %v", "build-go", args[0])
				}
			},
		},
		{
			name:                "get_cluster_workflow_schema",
			toolset:             "build",
			descriptionKeywords: []string{"cluster", "workflow", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": "build-go",
			},
			expectedMethod: "GetClusterWorkflowSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "build-go" {
					t.Errorf("Expected cwf_name %q, got %v", "build-go", args[0])
				}
			},
		},
	}
}
