// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// deploymentToolSpecs returns test specs for deployment toolset.
// Note: component release operations (list/create/get/schema) are registered by the PE toolset,
// not the deployment toolset; those specs live in pe_specs_test.go.
func deploymentToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 9)
	specs = append(specs, deploymentBindingSpecs()...)
	specs = append(specs, deploymentPipelineSpecs()...)
	specs = append(specs, deploymentEnvironmentSpecs()...)
	return specs
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
			name:                "create_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"create", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "environment", "release_name"},
			optionalParams: []string{
				"component_type_environment_configs",
				"trait_environment_configs", "workload_overrides",
			},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"environment":    "dev",
				"release_name":   testReleaseName,
			},
			expectedMethod: "CreateReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %s, got %v",
						testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "update_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"update", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "binding_name"},
			optionalParams: []string{
				"release_name", "release_state",
				"component_type_environment_configs",
				"trait_environment_configs", "workload_overrides",
			},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"binding_name":   "binding-1",
				"release_name":   testReleaseName,
			},
			expectedMethod: "UpdateReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "binding-1" {
					t.Errorf("Expected (%s, binding-1), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "delete_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "binding_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"binding_name":   "binding-1",
			},
			expectedMethod: "DeleteReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "binding-1" {
					t.Errorf("Expected (%s, binding-1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "delete_component_release",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "DeleteComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testReleaseName, args[0], args[1])
				}
			},
		},
	}
}

func deploymentPipelineSpecs() []toolTestSpec {
	return makeNamespacedListGetSpecs(
		"deployment", "list_deployment_pipelines", "get_deployment_pipeline",
		[]string{"list", "deployment", "pipeline"}, []string{"deployment", "pipeline"},
		"name", "default-pipeline", "ListDeploymentPipelines", "GetDeploymentPipeline",
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
