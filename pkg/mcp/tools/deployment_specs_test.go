// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// deploymentToolSpecs returns test specs for deployment toolset
func deploymentToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 13)
	specs = append(specs, deploymentReleaseSpecs()...)
	specs = append(specs, deploymentBindingSpecs()...)
	specs = append(specs, deploymentPipelineSpecs()...)
	specs = append(specs, deploymentEnvironmentSpecs()...)
	return specs
}

func deploymentReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_releases",
			toolset:             "deployment",
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
			toolset:             "deployment",
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
			toolset:             "deployment",
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
			toolset:             "deployment",
			descriptionKeywords: []string{"release", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"release_name"},
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

func deploymentBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_release_bindings",
			toolset:             "deployment",
			descriptionKeywords: []string{"release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "binding_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"binding_name":   "binding-dev",
			},
			expectedMethod: "GetReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "binding-dev" {
					t.Errorf("Expected (%s, binding-dev), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "patch_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"patch", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "binding_name"},
			optionalParams: []string{
				"release_name", "environment", "component_type_env_overrides",
				"trait_overrides", "workload_overrides",
			},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"binding_name":   "binding-1",
				"release_name":   testReleaseName,
			},
			expectedMethod: "PatchReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "binding-1" {
					t.Errorf("Expected (%s, binding-1), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "update_release_binding_state",
			toolset:             "deployment",
			descriptionKeywords: []string{"state", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "binding_name", "release_state"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"binding_name":   "binding-1",
				"release_state":  "Active",
			},
			expectedMethod: "UpdateReleaseBindingState",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "binding-1" {
					t.Errorf("Expected (%s, binding-1), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

func deploymentPipelineSpecs() []toolTestSpec {
	return makeNamespacedListGetSpecs(
		"deployment", "list_deployment_pipelines", "get_deployment_pipeline",
		[]string{"list", "deployment", "pipeline"}, []string{"deployment", "pipeline"},
		"pipeline_name", "default-pipeline", "ListDeploymentPipelines", "GetDeploymentPipeline",
	)
}

func deploymentEnvironmentSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_environments",
			toolset:             "deployment",
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
	}
}
