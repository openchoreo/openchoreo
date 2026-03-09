// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// peToolSpecs returns test specs for platform engineering toolset
func peToolSpecs() []toolTestSpec {
	specs := peEnvironmentSpecs()
	specs = append(specs, pePipelineSpecs()...)
	specs = append(specs, peDataPlaneSpecs()...)
	specs = append(specs, peBuildPlaneSpecs()...)
	specs = append(specs, peObservabilityPlaneSpecs()...)
	specs = append(specs, peClusterSpecs()...)
	specs = append(specs, pePlatformStandardsSpecs()...)
	specs = append(specs, peDiagnosticsSpecs()...)
	return specs
}

func peEnvironmentSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_environments",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
			name:                "create_environment",
			toolset:             "pe",
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
			},
		},
		{
			name:                "update_environment",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"display_name", "description", "is_production"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "dev",
			},
			expectedMethod: "UpdateEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_environment",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "env_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"env_name":       testEnvName,
			},
			expectedMethod: "DeleteEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEnvName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testEnvName, args[0], args[1])
				}
			},
		},
	}
}

func pePipelineSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "create_deployment_pipeline",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"display_name", "description", "promotion_paths"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "new-pipeline",
			},
			expectedMethod: "CreateDeploymentPipeline",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
	}
}

func peDataPlaneSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_dataplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
			toolset:             "pe",
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
	}
}

func peBuildPlaneSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_buildplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "build", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
		{
			name:                "get_buildplane",
			toolset:             "pe",
			descriptionKeywords: []string{"build", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "bp_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"bp_name":        "bp1",
			},
			expectedMethod: "GetBuildPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "bp1" {
					t.Errorf("Expected (%s, bp1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

func peObservabilityPlaneSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_observability_planes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "observability", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
			name:                "get_observability_plane",
			toolset:             "pe",
			descriptionKeywords: []string{"observability", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "op_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"op_name":        "observability-plane-1",
			},
			expectedMethod: "GetObservabilityPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "observability-plane-1" {
					t.Errorf("Expected (%s, observability-plane-1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

func peClusterSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_dataplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "data", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_dataplane",
			toolset:             "pe",
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
			name:                "list_cluster_buildplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "build", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "list_cluster_observability_planes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "observability", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterObservabilityPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
	}
}

func pePlatformStandardsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_types",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
			toolset:             "pe",
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
			name:                "list_traits",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
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
			toolset:             "pe",
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
			name:                "list_workflows",
			toolset:             "pe",
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
			toolset:             "pe",
			descriptionKeywords: []string{"workflow", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workflow_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workflow_name":  "build-workflow",
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

func peDiagnosticsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_resource_events",
			toolset:             "pe",
			descriptionKeywords: []string{"event"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "release_binding_name", "group", "version", "kind", "resource_name"},
			testArgs: map[string]any{
				"namespace_name":       testNamespaceName,
				"release_binding_name": "binding-dev",
				"group":                "apps",
				"version":              "v1",
				"kind":                 "Deployment",
				"resource_name":        "my-app",
			},
			expectedMethod: "GetResourceEvents",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_resource_logs",
			toolset:             "pe",
			descriptionKeywords: []string{"log"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "release_binding_name", "pod_name"},
			optionalParams:      []string{"since_seconds"},
			testArgs: map[string]any{
				"namespace_name":       testNamespaceName,
				"release_binding_name": "binding-dev",
				"pod_name":             "my-app-pod-abc123",
			},
			expectedMethod: "GetResourceLogs",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
	}
}
