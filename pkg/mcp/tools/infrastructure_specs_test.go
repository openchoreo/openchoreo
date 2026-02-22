// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// infrastructureToolSpecs returns test specs for infrastructure toolset
func infrastructureToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_environments",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListEnvironments",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_environment",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "env_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"env_name":       testEnvName,
			},
			expectedMethod: "GetEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEnvName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testEnvName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_dataplanes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "dp_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"dp_name":        "dp1",
			},
			expectedMethod: "GetDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "dp1" {
					t.Errorf("Expected (%s, dp1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_component_types",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListComponentTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_component_type_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"component", "type", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "ct_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"ct_name":        "WebApplication",
			},
			expectedMethod: "GetComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "WebApplication" {
					t.Errorf("Expected (%s, WebApplication), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_workflows",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
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
			toolset:             "infrastructure",
			descriptionKeywords: []string{"workflow", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workflow_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workflow_name":  "workflow-1",
			},
			expectedMethod: "GetWorkflowSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workflow-1" {
					t.Errorf("Expected (%s, workflow-1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_workflow_run",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workflow_name"},
			optionalParams:      []string{"parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workflow_name":  "workflow-1",
				"parameters": map[string]any{
					"repository": map[string]any{
						"url": "https://github.com/example/repo",
					},
				},
			},
			expectedMethod: "CreateWorkflowRun",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateWorkflowRunRequest
			},
		},
		{
			name:                "list_workflow_runs",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"project_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListWorkflowRuns",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_workflow_run",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       "workflow-run-1",
			},
			expectedMethod: "GetWorkflowRun",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workflow-run-1" {
					t.Errorf("Expected (%s, workflow-run-1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workflow_run_logs",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"workflow", "run", "logs"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			optionalParams:      []string{"step", "since_seconds"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       "workflow-run-1",
				"step":           "build",
				"since_seconds":  int64(300),
			},
			expectedMethod: "GetWorkflowRunLogs",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workflow-run-1" || args[2] != "build" {
					t.Errorf("Expected (%s, workflow-run-1, build), got (%v, %v, %v)",
						testNamespaceName, args[0], args[1], args[2])
				}
				if args[3] == nil {
					t.Errorf("Expected since_seconds pointer, got nil")
				}
			},
		},
		{
			name:                "get_workflow_run_events",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"workflow", "run", "events"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "run_name"},
			optionalParams:      []string{"step"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"run_name":       "workflow-run-1",
				"step":           "build",
			},
			expectedMethod: "GetWorkflowRunEvents",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workflow-run-1" || args[2] != "build" {
					t.Errorf("Expected (%s, workflow-run-1, build), got (%v, %v, %v)",
						testNamespaceName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "list_traits",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListTraits",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_trait_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"trait", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "trait_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"trait_name":     "autoscaling",
			},
			expectedMethod: "GetTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "autoscaling" {
					t.Errorf("Expected (%s, autoscaling), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"namespace_name", "name", "cluster_agent_client_ca", "public_virtual_host", "namespace_virtual_host",
			},
			optionalParams: []string{
				"display_name", "description", "observability_plane_ref",
			},
			testArgs: map[string]any{
				"namespace_name":          testNamespaceName,
				"name":                    "new-dp",
				"cluster_agent_client_ca": "-----BEGIN CERTIFICATE-----\ntest-ca-cert-data\n-----END CERTIFICATE-----",
				"public_virtual_host":     "public.example.com",
				"namespace_virtual_host":  "org.example.com",
			},
			expectedMethod: "CreateDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateDataPlaneRequest
			},
		},
		{
			name:                "create_environment",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"display_name", "description", "data_plane_ref", "is_production", "dns_prefix"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "new-env",
				"display_name":   "New Environment",
				"description":    "Test environment",
				"data_plane_ref": "dp1",
				"is_production":  false,
			},
			expectedMethod: "CreateEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateEnvironmentRequest
			},
		},
		{
			name:                "list_observability_planes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "observability", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListObservabilityPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "list_cluster_dataplanes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "data", "plane"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments to validate
			},
		},
		{
			name:                "get_cluster_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"cdp_name"},
			testArgs: map[string]any{
				"cdp_name": "cdp1",
			},
			expectedMethod: "GetClusterDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "cdp1" {
					t.Errorf("Expected cdp_name %q, got %v", "cdp1", args[0])
				}
			},
		},
		{
			name:                "create_cluster_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "cluster", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"name", "plane_id", "cluster_agent_client_ca", "public_virtual_host", "organization_virtual_host",
			},
			optionalParams: []string{
				"display_name", "description", "public_http_port", "public_https_port",
				"organization_http_port", "organization_https_port", "observability_plane_ref",
			},
			testArgs: map[string]any{
				"name":                      "new-cdp",
				"plane_id":                  "us-west-prod",
				"cluster_agent_client_ca":   "-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----",
				"public_virtual_host":       "public.example.com",
				"organization_virtual_host": "org.example.com",
			},
			expectedMethod: "CreateClusterDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				// args[0] is *models.CreateClusterDataPlaneRequest
			},
		},
		{
			name:                "list_cluster_buildplanes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "build", "plane"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments to validate
			},
		},
		{
			name:                "list_cluster_observability_planes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "observability", "plane"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterObservabilityPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments to validate
			},
		},
		{
			name:                "list_cluster_component_types",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "component", "type"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterComponentTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments to validate
			},
		},
		{
			name:                "get_cluster_component_type",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"cct_name"},
			testArgs: map[string]any{
				"cct_name": "go-service",
			},
			expectedMethod: "GetClusterComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "go-service" {
					t.Errorf("Expected cct_name %q, got %v", "go-service", args[0])
				}
			},
		},
		{
			name:                "get_cluster_component_type_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "component", "type", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"cct_name"},
			testArgs: map[string]any{
				"cct_name": "go-service",
			},
			expectedMethod: "GetClusterComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "go-service" {
					t.Errorf("Expected cct_name %q, got %v", "go-service", args[0])
				}
			},
		},
		{
			name:                "list_cluster_traits",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "trait"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterTraits",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments to validate
			},
		},
		{
			name:                "get_cluster_trait",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"ct_name"},
			testArgs: map[string]any{
				"ct_name": "autoscaler",
			},
			expectedMethod: "GetClusterTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "autoscaler" {
					t.Errorf("Expected ct_name %q, got %v", "autoscaler", args[0])
				}
			},
		},
		{
			name:                "get_cluster_trait_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"cluster", "trait", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"ct_name"},
			testArgs: map[string]any{
				"ct_name": "autoscaler",
			},
			expectedMethod: "GetClusterTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "autoscaler" {
					t.Errorf("Expected ct_name %q, got %v", "autoscaler", args[0])
				}
			},
		},
	}
}
