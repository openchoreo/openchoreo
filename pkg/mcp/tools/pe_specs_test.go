// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// peToolSpecs returns test specs for platform engineering toolset
func peToolSpecs() []toolTestSpec {
	specs := peEnvironmentSpecs()
	specs = append(specs, pePipelineSpecs()...)
	specs = append(specs, peComponentReleaseSpecs()...)
	specs = append(specs, peDataPlaneSpecs()...)
	specs = append(specs, peWorkflowPlaneSpecs()...)
	specs = append(specs, peObservabilityPlaneSpecs()...)
	specs = append(specs, peClusterSpecs()...)
	specs = append(specs, peClusterPlatformStandardsSpecs()...)
	specs = append(specs, pePlatformStandardsSpecs()...)
	specs = append(specs, peDiagnosticsSpecs()...)
	specs = append(specs, peAuthzSpecs()...)
	return specs
}

func peComponentReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_releases",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListComponentReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_component_release",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "CreateComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName || args[2] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testComponentName, testReleaseName,
						args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_component_release",
			toolset:             "pe",
			descriptionKeywords: []string{"release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testReleaseName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_component_release_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"release", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentReleaseSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName || args[2] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testComponentName, testReleaseName, args[0], args[1], args[2])
				}
			},
		},
	}
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
			requiredParams:      []string{"namespace_name", "name", "data_plane_ref"},
			optionalParams: []string{
				"display_name", "description", "data_plane_ref_kind", "is_production",
			},
			testArgs: map[string]any{
				"namespace_name":      testNamespaceName,
				"name":                "new-env",
				"display_name":        "New Environment",
				"description":         "Test environment",
				"data_plane_ref":      "dp1",
				"data_plane_ref_kind": "DataPlane",
				"is_production":       false,
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
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEnvName,
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
		{
			name:                "update_deployment_pipeline",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"display_name", "description", "promotion_paths"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-pipeline",
			},
			expectedMethod: "UpdateDeploymentPipeline",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_deployment_pipeline",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-pipeline",
			},
			expectedMethod: "DeleteDeploymentPipeline",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-pipeline" {
					t.Errorf("Expected (%s, my-pipeline), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

func peDataPlaneSpecs() []toolTestSpec {
	return makeScopedListGetSpecs(
		"pe", "list_dataplanes", "get_dataplane",
		[]string{"list", "data", "plane"}, []string{"data", "plane"},
		"name", "dp1", "ListDataPlanes", "GetDataPlane",
	)
}

func peWorkflowPlaneSpecs() []toolTestSpec {
	return makeScopedListGetSpecs(
		"pe", "list_workflowplanes", "get_workflowplane",
		[]string{"list", "workflow", "plane"}, []string{"workflow", "plane"},
		"name", "wp1", "ListWorkflowPlanes", "GetWorkflowPlane",
	)
}

func peObservabilityPlaneSpecs() []toolTestSpec {
	return makeScopedListGetSpecs(
		"pe", "list_observability_planes", "get_observability_plane",
		[]string{"list", "observability", "plane"}, []string{"observability", "plane"},
		"name", "observability-plane-1", "ListObservabilityPlanes", "GetObservabilityPlane",
	)
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
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": "cdp1",
			},
			expectedMethod: "GetClusterDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "cdp1" {
					t.Errorf("Expected name %q, got %v", "cdp1", args[0])
				}
			},
		},
		{
			name:                "list_cluster_workflowplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "workflow", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterWorkflowPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_workflowplane",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "workflow", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": "cwp1",
			},
			expectedMethod: "GetClusterWorkflowPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "cwp1" {
					t.Errorf("Expected name %q, got %v", "cwp1", args[0])
				}
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
		{
			name:                "get_cluster_observability_plane",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "observability", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": "cop1",
			},
			expectedMethod: "GetClusterObservabilityPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "cop1" {
					t.Errorf("Expected name %q, got %v", "cop1", args[0])
				}
			},
		},
	}
}

func peClusterPlatformStandardsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_component_types",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "component", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterComponentTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testGoServiceName,
			},
			expectedMethod: "GetClusterComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testGoServiceName {
					t.Errorf("Expected name %q, got %v", testGoServiceName, args[0])
				}
			},
		},
		{
			name:                "get_cluster_component_type_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "component", "type", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testGoServiceName,
			},
			expectedMethod: "GetClusterComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testGoServiceName {
					t.Errorf("Expected name %q, got %v", testGoServiceName, args[0])
				}
			},
		},
		{
			name:                "list_cluster_traits",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "trait"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterTraits",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testAutoscalerName,
			},
			expectedMethod: "GetClusterTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testAutoscalerName {
					t.Errorf("Expected name %q, got %v", testAutoscalerName, args[0])
				}
			},
		},
		{
			name:                "get_cluster_trait_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "trait", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testAutoscalerName,
			},
			expectedMethod: "GetClusterTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testAutoscalerName {
					t.Errorf("Expected name %q, got %v", testAutoscalerName, args[0])
				}
			},
		},
		// Write operations (cluster-scoped)
		{
			name:                "create_cluster_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "cluster", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testGoServiceName,
				"spec": map[string]any{"workloadType": "deployment", "resources": []any{}},
			},
			expectedMethod: "CreateClusterComponentType",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "update_cluster_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "cluster", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testGoServiceName,
				"spec": map[string]any{"workloadType": "deployment", "resources": []any{}},
			},
			expectedMethod: "UpdateClusterComponentType",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "delete_cluster_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "cluster", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testGoServiceName,
			},
			expectedMethod: "DeleteClusterComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testGoServiceName {
					t.Errorf("Expected name %q, got %v", testGoServiceName, args[0])
				}
			},
		},
		{
			name:                "create_cluster_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "cluster", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testAutoscalerName,
				"spec": map[string]any{},
			},
			expectedMethod: "CreateClusterTrait",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "update_cluster_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "cluster", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testAutoscalerName,
				"spec": map[string]any{},
			},
			expectedMethod: "UpdateClusterTrait",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "delete_cluster_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "cluster", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testAutoscalerName,
			},
			expectedMethod: "DeleteClusterTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testAutoscalerName {
					t.Errorf("Expected name %q, got %v", testAutoscalerName, args[0])
				}
			},
		},
		{
			name:                "create_cluster_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "cluster", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testBuildWorkflow,
				"spec": map[string]any{"runTemplate": map[string]any{}},
			},
			expectedMethod: "CreateClusterWorkflow",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "update_cluster_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "cluster", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"name": testBuildWorkflow,
				"spec": map[string]any{"runTemplate": map[string]any{}},
			},
			expectedMethod: "UpdateClusterWorkflow",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
		{
			name:                "delete_cluster_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "cluster", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"name": testBuildWorkflow,
			},
			expectedMethod: "DeleteClusterWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testBuildWorkflow {
					t.Errorf("Expected name %q, got %v", testBuildWorkflow, args[0])
				}
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
			requiredParams:      []string{},
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
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
			name:                "get_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"component", "type", "spec"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testWebAppType,
			},
			expectedMethod: "GetComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWebAppType {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWebAppType, args[0], args[1])
				}
			},
		},
		{
			name:                "get_component_type_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"component", "type", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testWebAppType,
			},
			expectedMethod: "GetComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testWebAppType {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testWebAppType, args[0], args[1])
				}
			},
		},
		{
			name:                "list_traits",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{},
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
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
			name:                "get_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"trait", "spec"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testAutoscalingTrait,
			},
			expectedMethod: "GetTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testAutoscalingTrait {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testAutoscalingTrait, args[0], args[1])
				}
			},
		},
		{
			name:                "get_trait_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"trait", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testAutoscalingTrait,
			},
			expectedMethod: "GetTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testAutoscalingTrait {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testAutoscalingTrait, args[0], args[1])
				}
			},
		},
		{
			name:                "list_workflows",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{},
			optionalParams:      []string{"scope", "namespace_name", "limit", "cursor"},
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
			name:                "get_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"workflow", "spec"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testBuildWorkflow,
			},
			expectedMethod: "GetWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testBuildWorkflow {
					t.Errorf("Expected (%s, build-workflow), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workflow_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"workflow", "schema"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testBuildWorkflow,
			},
			expectedMethod: "GetWorkflowSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testBuildWorkflow {
					t.Errorf("Expected (%s, build-workflow), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		// Creation schema tools (static, no handler call)
		{
			name:                "get_component_type_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "component", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "get_cluster_component_type_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "cluster", "component", "type"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
		},
		{
			name:                "get_trait_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "trait"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "get_cluster_trait_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "cluster", "trait"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
		},
		{
			name:                "get_workflow_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "workflow"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope"},
			testArgs:            map[string]any{},
		},
		{
			name:                "get_cluster_workflow_creation_schema",
			toolset:             "pe",
			descriptionKeywords: []string{"schema", "creating", "cluster", "workflow"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
		},
		// Write operations (namespace-scoped)
		{
			name:                "create_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-component-type",
				"spec":           map[string]any{"workloadType": "deployment", "resources": []any{}},
			},
			expectedMethod: "CreateComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-component-type",
				"spec":           map[string]any{"workloadType": "deployment", "resources": []any{}},
			},
			expectedMethod: "UpdateComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_component_type",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "component", "type"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-component-type",
			},
			expectedMethod: "DeleteComponentType",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-component-type" {
					t.Errorf("Expected (%s, my-component-type), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-trait",
				"spec":           map[string]any{},
			},
			expectedMethod: "CreateTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-trait",
				"spec":           map[string]any{},
			},
			expectedMethod: "UpdateTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_trait",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "trait"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "my-trait",
			},
			expectedMethod: "DeleteTrait",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-trait" {
					t.Errorf("Expected (%s, my-trait), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testBuildWorkflow,
				"spec":           map[string]any{"runTemplate": map[string]any{}},
			},
			expectedMethod: "CreateWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"update", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "spec"},
			optionalParams:      []string{"scope", "namespace_name", "display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testBuildWorkflow,
				"spec":           map[string]any{"runTemplate": map[string]any{}},
			},
			expectedMethod: "UpdateWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "delete_workflow",
			toolset:             "pe",
			descriptionKeywords: []string{"delete", "workflow"},
			descriptionMinLen:   10,
			optionalParams:      []string{"scope", "namespace_name"},
			requiredParams:      []string{"name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testBuildWorkflow,
			},
			expectedMethod: "DeleteWorkflow",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testBuildWorkflow {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testBuildWorkflow, args[0], args[1])
				}
			},
		},
	}
}

func peDiagnosticsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_resource_tree",
			toolset:             "pe",
			descriptionKeywords: []string{"rendered", "resource"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "release_binding_name"},
			testArgs: map[string]any{
				"namespace_name":       testNamespaceName,
				"release_binding_name": "binding-dev",
			},
			expectedMethod: "GetResourceTree",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
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

// ---------------------------------------------------------------------------
// Coverage tests for closures in pe.go that are overwritten when both
// Component/Build/Deployment toolsets and PE toolset are registered together.
// These tests register only the relevant toolset so the non-PE closures execute.
// ---------------------------------------------------------------------------

// TestComponentToolsetClosuresInPEFile exercises the component toolset handler closures
// defined in pe.go (RegisterListComponentTypes, RegisterListTraits, etc.) that are normally
// overwritten when the PE toolset is also registered.
func TestComponentToolsetClosuresInPEFile(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{ComponentToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()

	tests := []struct {
		toolName       string
		args           map[string]any
		expectedMethod string
	}{
		{"list_component_types", map[string]any{"namespace_name": testNamespaceName}, "ListComponentTypes"},
		{
			"get_component_type_schema",
			map[string]any{"namespace_name": testNamespaceName, "name": testWebAppType},
			"GetComponentTypeSchema",
		},
		{"list_traits", map[string]any{"namespace_name": testNamespaceName}, "ListTraits"},
		{
			"get_trait_schema",
			map[string]any{"namespace_name": testNamespaceName, "name": testAutoscalingTrait},
			"GetTraitSchema",
		},
		{"list_cluster_component_types", map[string]any{}, "ListClusterComponentTypes"},
		{"get_cluster_component_type", map[string]any{"name": testGoServiceName}, "GetClusterComponentType"},
		{"get_cluster_component_type_schema", map[string]any{"name": testGoServiceName}, "GetClusterComponentTypeSchema"},
		{"list_cluster_traits", map[string]any{}, "ListClusterTraits"},
		{"get_cluster_trait", map[string]any{"name": testAutoscalerName}, "GetClusterTrait"},
		{"get_cluster_trait_schema", map[string]any{"name": testAutoscalerName}, "GetClusterTraitSchema"},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			mockHandler.calls = make(map[string][]interface{})
			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool %q: %v", tt.toolName, err)
			}
			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}
			if _, ok := mockHandler.calls[tt.expectedMethod]; !ok {
				t.Errorf("Expected ComponentToolset method %q to be called, got calls: %v",
					tt.expectedMethod, mockHandler.calls)
			}
		})
	}
}

// TestBuildToolsetClosuresInPEFile exercises the build toolset handler closures
// (RegisterListWorkflows, RegisterGetWorkflowSchema) in pe.go that are normally
// overwritten when the PE toolset is also registered.
func TestBuildToolsetClosuresInPEFile(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{BuildToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()

	tests := []struct {
		toolName       string
		args           map[string]any
		expectedMethod string
	}{
		{"list_workflows", map[string]any{"namespace_name": testNamespaceName}, "ListWorkflows"},
		{
			"get_workflow_schema",
			map[string]any{"namespace_name": testNamespaceName, "name": testBuildWorkflow},
			"GetWorkflowSchema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			mockHandler.calls = make(map[string][]interface{})
			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.toolName,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("Failed to call tool %q: %v", tt.toolName, err)
			}
			if len(result.Content) == 0 {
				t.Fatal("Expected non-empty result content")
			}
			if _, ok := mockHandler.calls[tt.expectedMethod]; !ok {
				t.Errorf("Expected BuildToolset method %q to be called, got calls: %v",
					tt.expectedMethod, mockHandler.calls)
			}
		})
	}
}

// TestDeploymentToolsetListEnvironmentsInPEFile exercises the RegisterListEnvironments
// closure in pe.go that uses DeploymentToolset, normally overwritten by
// RegisterPEListEnvironments when the PE toolset is also registered.
func TestDeploymentToolsetListEnvironmentsInPEFile(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{DeploymentToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_environments",
		Arguments: map[string]any{"namespace_name": testNamespaceName},
	})
	if err != nil {
		t.Fatalf("Failed to call list_environments: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["ListEnvironments"]; !ok {
		t.Errorf("Expected DeploymentToolset.ListEnvironments to be called, got calls: %v", mockHandler.calls)
	}
}

// TestCreateEnvironmentMinimalArgs covers the code path in RegisterCreateEnvironment
// where optional fields (display_name, description) are absent.
func TestCreateEnvironmentMinimalArgs(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "create_environment",
		Arguments: map[string]any{"namespace_name": testNamespaceName, "name": "minimal-env", "data_plane_ref": "default"},
	})
	if err != nil {
		t.Fatalf("Failed to call create_environment: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["CreateEnvironment"]; !ok {
		t.Errorf("Expected CreateEnvironment to be called, got: %v", mockHandler.calls)
	}
}

// TestCreateDeploymentPipelineWithPromotionPaths covers the promotion_paths branch
// in RegisterCreateDeploymentPipeline where promotion paths are provided.
func TestCreateDeploymentPipelineWithPromotionPaths(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_deployment_pipeline",
		Arguments: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           "my-pipeline",
			"display_name":   "My Pipeline",
			"description":    "A test pipeline",
			"promotion_paths": []map[string]any{
				{
					"source_environment_ref": "dev",
					"target_environment_refs": []map[string]any{
						{"name": "staging"},
						{"name": "production"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to call create_deployment_pipeline: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["CreateDeploymentPipeline"]; !ok {
		t.Errorf("Expected CreateDeploymentPipeline to be called, got: %v", mockHandler.calls)
	}
}

// TestUpdateEnvironmentWithOptionalFields covers the non-empty display_name/description
// branches in RegisterUpdateEnvironment.
func TestUpdateEnvironmentWithOptionalFields(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_environment",
		Arguments: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           "dev",
			"display_name":   "Development",
			"description":    "Updated description",
		},
	})
	if err != nil {
		t.Fatalf("Failed to call update_environment: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["UpdateEnvironment"]; !ok {
		t.Errorf("Expected UpdateEnvironment to be called, got: %v", mockHandler.calls)
	}
}

// TestCreateEnvironmentWithClusterDataPlane covers the ClusterDataPlane kind branch
// in RegisterCreateEnvironment (data_plane_ref_kind == "ClusterDataPlane").
func TestCreateEnvironmentWithClusterDataPlane(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "create_environment",
		Arguments: map[string]any{
			"namespace_name":      testNamespaceName,
			"name":                "prod-env",
			"data_plane_ref":      "cdp1",
			"data_plane_ref_kind": "ClusterDataPlane",
			"is_production":       true,
		},
	})
	if err != nil {
		t.Fatalf("Failed to call create_environment: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["CreateEnvironment"]; !ok {
		t.Errorf("Expected CreateEnvironment to be called, got: %v", mockHandler.calls)
	}
}

// TestUpdateDeploymentPipelineWithPromotionPaths covers the promotion_paths branch
// in RegisterUpdateDeploymentPipeline.
func TestUpdateDeploymentPipelineWithPromotionPaths(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_deployment_pipeline",
		Arguments: map[string]any{
			"namespace_name": testNamespaceName,
			"name":           "my-pipeline",
			"display_name":   "Updated Pipeline",
			"description":    "Updated description",
			"promotion_paths": []map[string]any{
				{
					"source_environment_ref": "dev",
					"target_environment_refs": []map[string]any{
						{"name": "prod"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Failed to call update_deployment_pipeline: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["UpdateDeploymentPipeline"]; !ok {
		t.Errorf("Expected UpdateDeploymentPipeline to be called, got: %v", mockHandler.calls)
	}
}

// TestGetResourceLogsWithSinceSeconds covers the since_seconds optional parameter
// in RegisterGetResourceLogs.
func TestGetResourceLogsWithSinceSeconds(t *testing.T) {
	mockHandler := NewMockCoreToolsetHandler()
	toolsets := &Toolsets{PEToolset: mockHandler}
	clientSession := setupTestServerWithToolset(t, toolsets)
	defer clientSession.Close()

	ctx := context.Background()
	mockHandler.calls = make(map[string][]interface{})

	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_resource_logs",
		Arguments: map[string]any{
			"namespace_name":       testNamespaceName,
			"release_binding_name": "binding-dev",
			"pod_name":             "my-app-pod-abc123",
			"since_seconds":        int64(300),
		},
	})
	if err != nil {
		t.Fatalf("Failed to call get_resource_logs: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("Expected non-empty result content")
	}
	if _, ok := mockHandler.calls["GetResourceLogs"]; !ok {
		t.Errorf("Expected GetResourceLogs to be called, got: %v", mockHandler.calls)
	}
}
