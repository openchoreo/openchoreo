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
			requiredParams:      []string{"org_name", "project_name"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
			},
			expectedMethod: "ListComponents",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
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
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			optionalParams:      []string{"additional_resources"},
			testArgs: map[string]any{
				"org_name":             testOrgName,
				"project_name":         testProjectName,
				"component_name":       testComponentName,
				"additional_resources": []interface{}{"deployments", "services"},
			},
			expectedMethod: "GetComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
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
			name:                "get_component_binding",
			toolset:             "component",
			descriptionKeywords: []string{"component", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "environment"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"environment":    testEnvName,
			},
			expectedMethod: "GetComponentBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testEnvName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, testEnvName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "get_component_observer_url",
			toolset:             "component",
			descriptionKeywords: []string{"observability", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "environment_name"},
			testArgs: map[string]any{
				"org_name":         testOrgName,
				"project_name":     testProjectName,
				"component_name":   testComponentName,
				"environment_name": testEnvName,
			},
			expectedMethod: "GetComponentObserverURL",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testEnvName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, testEnvName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "get_component_workloads",
			toolset:             "component",
			descriptionKeywords: []string{"workload", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponentWorkloads",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "create_component",
			toolset:             "component",
			descriptionKeywords: []string{"create", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "name", "type"},
			optionalParams:      []string{"display_name", "description"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"name":         "new-component",
				"type":         "WebApplication",
			},
			expectedMethod: "CreateComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testOrgName, testProjectName, args[0], args[1])
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
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListComponentReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "create_component_release",
			toolset:             "component",
			descriptionKeywords: []string{"create", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			optionalParams:      []string{"release_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "CreateComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "get_component_release",
			toolset:             "component",
			descriptionKeywords: []string{"release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "list_release_bindings",
			toolset:             "component",
			descriptionKeywords: []string{"release", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			optionalParams:      []string{"environments"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"environments":   []interface{}{"dev", "staging"},
			},
			expectedMethod: "ListReleaseBindings",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
				envs := args[3].([]string)
				expected := []string{"dev", "staging"}
				if diff := cmp.Diff(expected, envs); diff != "" {
					t.Errorf("environments mismatch (-want +got):\n%s", diff)
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
			requiredParams:      []string{"org_name", "project_name", "component_name", "binding_name"},
			optionalParams: []string{
				"release_name", "environment", "component_type_env_overrides",
				"trait_overrides", "configuration_overrides",
			},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"binding_name":   "binding-1",
				"release_name":   testReleaseName,
			},
			expectedMethod: "PatchReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName || args[3] != "binding-1" {
					t.Errorf("Expected (%s, %s, %s, binding-1), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2], args[3])
				}
			},
		},
		{
			name:                "deploy_release",
			toolset:             "component",
			descriptionKeywords: []string{"deploy", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "DeployRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "promote_component",
			toolset:             "component",
			descriptionKeywords: []string{"promote", "component"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "source_env", "target_env"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"source_env":     "dev",
				"target_env":     "staging",
			},
			expectedMethod: "PromoteComponent",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "create_workload",
			toolset:             "component",
			descriptionKeywords: []string{"create", "workload"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "workload_spec"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"workload_spec":  map[string]interface{}{"containers": []interface{}{}},
			},
			expectedMethod: "CreateWorkload",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "update_component_binding",
			toolset:             "component",
			descriptionKeywords: []string{"update", "component", "binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "binding_name", "release_state"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"binding_name":   "binding-1",
				"release_state":  "Active",
			},
			expectedMethod: "UpdateComponentBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != "binding-1" {
					t.Errorf("Expected (%s, %s, %s, binding-1), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName,
						args[0], args[1], args[2], args[3])
				}
				// args[4] is *models.UpdateBindingRequest
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
			requiredParams:      []string{"org_name", "project_name", "component_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
			},
			expectedMethod: "GetComponentSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName || args[2] != testComponentName {
					t.Errorf("Expected (%s, %s, %s), got (%v, %v, %v)",
						testOrgName, testProjectName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_component_release_schema",
			toolset:             "component",
			descriptionKeywords: []string{"schema", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "project_name", "component_name", "release_name"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"project_name":   testProjectName,
				"component_name": testComponentName,
				"release_name":   testReleaseName,
			},
			expectedMethod: "GetComponentReleaseSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testProjectName ||
					args[2] != testComponentName || args[3] != testReleaseName {
					t.Errorf("Expected (%s, %s, %s, %s), got (%v, %v, %v, %v)",
						testOrgName, testProjectName, testComponentName, testReleaseName,
						args[0], args[1], args[2], args[3])
				}
			},
		},
	}
}
