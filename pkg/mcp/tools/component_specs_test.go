// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testReleaseName = "release-1"

// componentToolSpecs returns test specs for component toolset
func componentToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 5)
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
	specs = append(specs, componentClusterWorkflowSpecs()...)
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
			requiredParams:      []string{"namespace_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				if args[1] != testComponentName {
					t.Errorf("Expected component name %q, got %v", testComponentName, args[1])
				}
			},
		},
		{
			name:                "list_workloads",
			toolset:             "component",
			descriptionKeywords: []string{"workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListWorkloads",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_workload",
			toolset:             "component",
			descriptionKeywords: []string{"workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workload_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workload_name":  "workload1",
			},
			expectedMethod: "GetWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workload1" {
					t.Errorf("Expected (%s, workload1), got (%v, %v)",
						testNamespaceName, args[0], args[1])
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
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"auto_deploy", "parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "PatchComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
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
			toolset:             "component",
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
			toolset:             "component",
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
			name:                "list_release_bindings",
			toolset:             "component",
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
			toolset:             "component",
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
			requiredParams:      []string{"namespace_name", "binding_name"},
			optionalParams: []string{
				"release_name", "environment", "component_type_environment_configs",
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
			name:                "deploy_release",
			toolset:             "component",
			descriptionKeywords: []string{"deploy", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "DeployRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "promote_component",
			toolset:             "component",
			descriptionKeywords: []string{"promote", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "source_env", "target_env"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"source_env":     "dev",
				"target_env":     "staging",
			},
			expectedMethod: "PromoteComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_workload",
			toolset:             "component",
			descriptionKeywords: []string{"create", "workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "workload_spec"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"workload_spec":  map[string]interface{}{"container": map[string]interface{}{"image": "example.com/test:latest"}},
			},
			expectedMethod: "CreateWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "update_workload",
			toolset:             "component",
			descriptionKeywords: []string{"update", "workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "workload_name", "workload_spec"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"workload_name":  "workload-1",
				"workload_spec":  map[string]interface{}{"container": map[string]interface{}{"image": "example.com/test:v2"}},
			},
			expectedMethod: "UpdateWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "workload-1" {
					t.Errorf("Expected (%s, workload-1), got (%v, %v)",
						testNamespaceName, args[0], args[1])
				}
			},
		},
	}
}

// componentSchemaSpecs returns component schema operation specs
func componentSchemaSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_workload_schema",
			toolset:             "component",
			descriptionKeywords: []string{"workload", "schema"},
			descriptionMinLen:   10,
			testArgs:            map[string]any{},
			expectedMethod:      "GetWorkloadSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				// No arguments expected
			},
		},
		{
			name:                "get_component_schema",
			toolset:             "component",
			descriptionKeywords: []string{"schema", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponentSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "update_release_binding_state",
			toolset:             "component",
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
		{
			name:                "get_component_release_schema",
			toolset:             "component",
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

// clusterResourceTrioSpecs generates the standard list/get/get-schema trio of tool specs
// for a cluster-scoped resource (e.g., ClusterTrait, ClusterWorkflow).
type clusterResourceTrioConfig struct {
	resourceKeyword string // e.g., "trait", "workflow"
	listToolName    string // e.g., "list_cluster_traits"
	getToolName     string // e.g., "get_cluster_trait"
	schemaToolName  string // e.g., "get_cluster_trait_schema"
	paramName       string // e.g., "ct_name", "cwf_name"
	testValue       string // e.g., "autoscaler", "build-go"
	listMethod      string // e.g., "ListClusterTraits"
	getMethod       string // e.g., "GetClusterTrait"
	schemaMethod    string // e.g., "GetClusterTraitSchema"
}

func clusterResourceTrioSpecs(cfg clusterResourceTrioConfig) []toolTestSpec {
	return []toolTestSpec{
		{
			name:                cfg.listToolName,
			toolset:             "component",
			descriptionKeywords: []string{"cluster", cfg.resourceKeyword},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      cfg.listMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                cfg.getToolName,
			toolset:             "component",
			descriptionKeywords: []string{"cluster", cfg.resourceKeyword},
			descriptionMinLen:   10,
			requiredParams:      []string{cfg.paramName},
			testArgs:            map[string]any{cfg.paramName: cfg.testValue},
			expectedMethod:      cfg.getMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != cfg.testValue {
					t.Errorf("Expected %s %q, got %v", cfg.paramName, cfg.testValue, args[0])
				}
			},
		},
		{
			name:                cfg.schemaToolName,
			toolset:             "component",
			descriptionKeywords: []string{"cluster", cfg.resourceKeyword, "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{cfg.paramName},
			testArgs:            map[string]any{cfg.paramName: cfg.testValue},
			expectedMethod:      cfg.schemaMethod,
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != cfg.testValue {
					t.Errorf("Expected %s %q, got %v", cfg.paramName, cfg.testValue, args[0])
				}
			},
		},
	}
}

// componentClusterTraitSpecs returns cluster trait operation specs
func componentClusterTraitSpecs() []toolTestSpec {
	return clusterResourceTrioSpecs(clusterResourceTrioConfig{
		resourceKeyword: "trait",
		listToolName:    "list_cluster_traits",
		getToolName:     "get_cluster_trait",
		schemaToolName:  "get_cluster_trait_schema",
		paramName:       "ct_name",
		testValue:       "autoscaler",
		listMethod:      "ListClusterTraits",
		getMethod:       "GetClusterTrait",
		schemaMethod:    "GetClusterTraitSchema",
	})
}

func componentClusterWorkflowSpecs() []toolTestSpec {
	return clusterResourceTrioSpecs(clusterResourceTrioConfig{
		resourceKeyword: "workflow",
		listToolName:    "list_cluster_workflows",
		getToolName:     "get_cluster_workflow",
		schemaToolName:  "get_cluster_workflow_schema",
		paramName:       "cwf_name",
		testValue:       "build-go",
		listMethod:      "ListClusterWorkflows",
		getMethod:       "GetClusterWorkflow",
		schemaMethod:    "GetClusterWorkflowSchema",
	})
}
