// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const (
	testReleaseName = "release-1"
	testBindingName = "binding-1"
)

// componentToolSpecs returns test specs for component toolset (definition & configuration)
func componentToolSpecs() []toolTestSpec {
	specs := make([]toolTestSpec, 0, 20)
	specs = append(specs, componentBasicSpecs()...)
	specs = append(specs, componentWorkloadSpecs()...)
	specs = append(specs, componentSchemaSpecs()...)
	specs = append(specs, componentPlatformStandardsSpecs()...)
	specs = append(specs, componentBindingSpecs()...)
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

// componentWorkloadSpecs returns workload operation specs
// Note: component releases and release bindings are registered by the PE/deployment toolsets,
// not the component toolset; those specs live in pe_specs_test.go and deployment_specs_test.go.
func componentWorkloadSpecs() []toolTestSpec {
	return nil
}

// componentBindingSpecs returns component binding operation specs
func componentBindingSpecs() []toolTestSpec {
	return []toolTestSpec{
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

// componentPlatformStandardsSpecs returns specs for platform standards accessible from the component toolset
func componentPlatformStandardsSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_component_types",
			toolset:             "component",
			descriptionKeywords: []string{"list", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListComponentTypes",
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
		{
			name:                "list_traits",
			toolset:             "component",
			descriptionKeywords: []string{"list", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName},
			expectedMethod:      "ListTraits",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_trait_schema",
			toolset:             "component",
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
			testArgs:            map[string]any{"cct_name": "go-service"},
			expectedMethod:      "GetClusterComponentType",
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
			testArgs:            map[string]any{"cct_name": "go-service"},
			expectedMethod:      "GetClusterComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "go-service" {
					t.Errorf("Expected cct_name %q, got %v", "go-service", args[0])
				}
			},
		},
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
			testArgs:            map[string]any{"ct_name": "autoscaler"},
			expectedMethod:      "GetClusterTrait",
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
			testArgs:            map[string]any{"ct_name": "autoscaler"},
			expectedMethod:      "GetClusterTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "autoscaler" {
					t.Errorf("Expected ct_name %q, got %v", "autoscaler", args[0])
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
	}
}
