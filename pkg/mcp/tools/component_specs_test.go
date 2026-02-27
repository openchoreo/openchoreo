// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

const testReleaseName = "release-1"

// componentToolSpecs returns test specs for component toolset
func componentToolSpecs() []toolTestSpec {
	specs := []toolTestSpec{}
	specs = append(specs, componentBasicSpecs()...)
	specs = append(specs, componentReleaseSpecs()...)
	specs = append(specs, componentBindingSpecs()...)
	specs = append(specs, componentSchemaSpecs()...)
	specs = append(specs, componentTypeSpecs()...)
	specs = append(specs, componentTraitSpecs()...)
	specs = append(specs, componentWorkflowRunSpecs()...)
	specs = append(specs, componentWorkflowSpecs()...)
	specs = append(specs, componentClusterComponentTypeSpecs()...)
	specs = append(specs, componentClusterTraitSpecs()...)
	return specs
}

// componentBasicSpecs returns basic component operation specs
func componentBasicSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_components",
			toolset:             "component",
			descriptionKeywords: []string{"list", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "ListComponents",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != testProjectName {
					t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
				}
			},
		},
		{
			name:                "get_component",
			toolset:             "component",
			descriptionKeywords: []string{"component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"additional_resources"},
			testArgs: map[string]any{
				"namespace_name":       testNamespaceName,
				"project_name":         testProjectName,
				"component_name":       testComponentName,
				"additional_resources": []interface{}{"deployments", "services"},
			},
			expectedMethod: "GetComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != testProjectName {
					t.Errorf("Expected project name %q, got %v", testProjectName, args[1])
				}
				if args[2] != testComponentName {
					t.Errorf("Expected component name %q, got %v", testComponentName, args[2])
				}
				resources := args[3].([]string)
				expected := []string{"deployments", "services"}
				if diff := cmp.Diff(expected, resources); diff != "" {
					t.Errorf("additional_resources mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name:                "get_component_workloads",
			toolset:             "component",
			descriptionKeywords: []string{"workload", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponentWorkloads",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_component_workload",
			toolset:             "component",
			descriptionKeywords: []string{"workload", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "workload_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"workload_name":  "workload1",
			},
			expectedMethod: "GetComponentWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "workload1" {
					t.Errorf("Expected (%s, %s, %s, workload1), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "create_component",
			toolset:             "component",
			descriptionKeywords: []string{"create", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "name", "componentType"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"name":           "new-component",
				"componentType":  "WebApplication",
			},
			expectedMethod: "CreateComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
		{
			name:                "patch_component",
			toolset:             "component",
			descriptionKeywords: []string{"patch", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"display_name", "description", "auto_deploy", "parameters", "workflow"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"display_name":   "My Component",
			},
			expectedMethod: "PatchComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
	}
}

// componentReleaseSpecs returns component release operation specs
func componentReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_releases",
			toolset:             "component",
			descriptionKeywords: []string{"list", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListComponentReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "create_component_release",
			toolset:             "component",
			descriptionKeywords: []string{"create", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "CreateComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "get_component_release",
			toolset:             "component",
			descriptionKeywords: []string{"release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "list_release_bindings",
			toolset:             "component",
			descriptionKeywords: []string{"release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"environments", "limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"environments":   []interface{}{"dev", "staging"},
			},
			expectedMethod: "ListReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
				envs := args[3].([]string)
				expected := []string{"dev", "staging"}
				if diff := cmp.Diff(expected, envs); diff != "" {
					t.Errorf("environments mismatch (-want +got):\n%s", diff)
				}
			},
		},
		{
			name:                "get_release_binding",
			toolset:             "component",
			descriptionKeywords: []string{"release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "binding_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"binding_name":   "binding-dev",
			},
			expectedMethod: "GetReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "binding-dev" {
					t.Errorf("Expected (%s, %s, %s, binding-dev), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
	}
}

// componentBindingSpecs returns component binding operation specs
func componentBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "patch_release_binding",
			toolset:             "component",
			descriptionKeywords: []string{"patch", "release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "binding_name"},
			optionalParams: []string{
				"release_name", "environment", "component_type_env_overrides",
				"trait_overrides", "workload_overrides",
			},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"binding_name":   "binding-1",
				"release_name":   testReleaseName,
			},
			expectedMethod: "PatchReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "binding-1" {
					t.Errorf("Expected (%s, %s, %s, binding-1), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "deploy_release",
			toolset:             "component",
			descriptionKeywords: []string{"deploy", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "DeployRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "promote_component",
			toolset:             "component",
			descriptionKeywords: []string{"promote", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "source_env", "target_env"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"source_env":     "dev",
				"target_env":     "staging",
			},
			expectedMethod: "PromoteComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "create_workload",
			toolset:             "component",
			descriptionKeywords: []string{"create", "workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "workload_spec"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"workload_spec":  map[string]interface{}{"container": map[string]interface{}{"image": "example.com/test:latest"}},
			},
			expectedMethod: "CreateWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
	}
}

// componentSchemaSpecs returns component schema operation specs
func componentSchemaSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_component_schema",
			toolset:             "component",
			descriptionKeywords: []string{"schema", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponentSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_environment_release",
			toolset:             "component",
			descriptionKeywords: []string{"release", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "environment_name"},
			testArgs: map[string]any{
				"namespace_name":   testNamespaceName,
				"project_name":     testProjectName,
				"component_name":   testComponentName,
				"environment_name": testEnvName,
			},
			expectedMethod: "GetEnvironmentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testEnvName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, testEnvName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "update_release_binding_state",
			toolset:             "component",
			descriptionKeywords: []string{"state", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name", "binding_name", "release_state"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"binding_name":   "binding-1",
				"release_state":  "Active",
			},
			expectedMethod: "UpdateReleaseBindingState",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "binding-1" {
					t.Errorf("Expected (%s, %s, %s, binding-1), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "get_component_release_schema",
			toolset:             "component",
			descriptionKeywords: []string{"release", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name", "component_name"},
			optionalParams:      []string{"release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentReleaseSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testNamespaceName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "trigger_workflow_run",
			toolset:             "component",
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
	}
}

// componentTypeSpecs returns component type operation specs
func componentTypeSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_types",
			toolset:             "component",
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
			toolset:             "component",
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
	}
}

// listAndGetSchemaSpecs generates specs for list and get-schema operations on a named entity type.
// entityKind is the human-readable kind (e.g. "trait"), entityNameParam is the request param name
// (e.g. "trait_name"), testEntityName is the value used in test args, listMethod and getSchemaMethod
// are the expected mock method names.
func listAndGetSchemaSpecs(
	entityKind, entityNameParam, testEntityName, listMethod, getSchemaMethod string,
) []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_" + entityKind + "s",
			toolset:             "component",
			descriptionKeywords: []string{"list", entityKind},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: listMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_" + entityKind + "_schema",
			toolset:             "component",
			descriptionKeywords: []string{entityKind, "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", entityNameParam},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				entityNameParam:  testEntityName,
			},
			expectedMethod: getSchemaMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEntityName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testEntityName, args[0], args[1])
				}
			},
		},
	}
}

// componentTraitSpecs returns trait operation specs
func componentTraitSpecs() []toolTestSpec {
	return listAndGetSchemaSpecs("trait", "trait_name", "autoscaling", "ListTraits", "GetTraitSchema")
}

// componentWorkflowRunSpecs returns workflow run operation specs
func componentWorkflowRunSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "create_workflow_run",
			toolset:             "component",
			descriptionKeywords: []string{"create", "workflow", "run"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workflow_name"},
			optionalParams:      []string{"parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workflow_name":  "build-workflow",
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
			toolset:             "component",
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
			toolset:             "component",
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
	}
}

// componentWorkflowSpecs returns workflow operation specs
func componentWorkflowSpecs() []toolTestSpec {
	return listAndGetSchemaSpecs("workflow", "workflow_name", "build-workflow", "ListWorkflows", "GetWorkflowSchema")
}

// componentClusterComponentTypeSpecs returns cluster component type operation specs
func componentClusterComponentTypeSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_component_types",
			toolset:             "component",
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
			toolset:             "component",
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
			toolset:             "component",
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
	}
}

// componentClusterTraitSpecs returns cluster trait operation specs
func componentClusterTraitSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_traits",
			toolset:             "component",
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
			toolset:             "component",
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
			toolset:             "component",
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
