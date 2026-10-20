// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const testReleaseLineName = "next"

func releaseLineSpecs() []toolTestSpec {
	lineArg := func(t *testing.T, args []interface{}) *gen.ReleaseLine {
		t.Helper()
		line, ok := args[2].(*gen.ReleaseLine)
		if !ok {
			t.Fatalf("Expected *gen.ReleaseLine, got %T", args[2])
		}
		return line
	}
	return []toolTestSpec{
		{
			name:                "list_release_lines",
			toolset:             "component",
			descriptionKeywords: []string{"list", "release line"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
			},
			expectedMethod: "ListReleaseLines",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testComponentName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_release_line",
			toolset:             "component",
			descriptionKeywords: []string{"release line"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "release_line_name"},
			testArgs: map[string]any{
				"namespace_name":    testNamespaceName,
				"component_name":    testComponentName,
				"release_line_name": testReleaseLineName,
			},
			expectedMethod: "GetReleaseLine",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName || args[2] != testReleaseLineName {
					t.Errorf("Expected (%s, %s, %s), got %v", testNamespaceName, testComponentName, testReleaseLineName, args)
				}
			},
		},
		{
			name:                "create_release_line",
			toolset:             "component",
			descriptionKeywords: []string{"create", "release line"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "name"},
			optionalParams:      []string{"description", "auto_deploy", "parameters", "traits", "workflow_parameters"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"name":           testReleaseLineName,
				"description":    "Builds the next branch",
				"auto_deploy":    false,
				"parameters":     map[string]any{"logLevel": "warn"},
				"traits": []any{
					map[string]any{"kind": "ClusterTrait", "name": "health-probes", "instanceName": "health"},
				},
				"workflow_parameters": map[string]any{
					"repository": map[string]any{"revision": map[string]any{"branch": "next"}},
				},
			},
			expectedMethod: "CreateReleaseLine",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testComponentName, args[0], args[1])
				}
				line := lineArg(t, args)
				if line.Metadata.Name != testReleaseLineName {
					t.Errorf("Expected name %s, got %s", testReleaseLineName, line.Metadata.Name)
				}
				annotations := line.Metadata.Annotations
				if annotations == nil || (*annotations)["openchoreo.dev/description"] != "Builds the next branch" {
					t.Errorf("Expected description annotation, got %v", line.Metadata.Annotations)
				}
				spec := line.Spec
				if spec == nil || spec.Owner.ComponentName != testComponentName {
					t.Fatalf("Expected owner component %s, got %+v", testComponentName, spec)
				}
				if spec.AutoDeploy == nil || *spec.AutoDeploy {
					t.Errorf("Expected autoDeploy false, got %v", spec.AutoDeploy)
				}
				if spec.Parameters == nil || (*spec.Parameters)["logLevel"] != "warn" {
					t.Errorf("Expected parameters.logLevel warn, got %v", spec.Parameters)
				}
				if spec.Traits == nil || len(*spec.Traits) != 1 || (*spec.Traits)[0].InstanceName != "health" {
					t.Errorf("Expected one trait with instance health, got %v", spec.Traits)
				}
				if spec.Workflow == nil || spec.Workflow.Parameters == nil {
					t.Errorf("Expected workflow parameters, got %v", spec.Workflow)
				}
			},
		},
		{
			name:                "update_release_line",
			toolset:             "component",
			descriptionKeywords: []string{"release line"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "release_line_name"},
			optionalParams:      []string{"description", "auto_deploy", "parameters", "traits", "workflow_parameters"},
			testArgs: map[string]any{
				"namespace_name":    testNamespaceName,
				"component_name":    testComponentName,
				"release_line_name": testReleaseLineName,
				"parameters":        map[string]any{"logLevel": "debug"},
			},
			expectedMethod: "UpdateReleaseLine",
			validateCall: func(t *testing.T, args []interface{}) {
				line := lineArg(t, args)
				if line.Metadata.Name != testReleaseLineName {
					t.Errorf("Expected name %s, got %s", testReleaseLineName, line.Metadata.Name)
				}
				if line.Spec == nil || line.Spec.AutoDeploy != nil || line.Spec.Workflow != nil {
					t.Errorf("Expected only parameters to be set, got %+v", line.Spec)
				}
			},
		},
		{
			name:                "delete_release_line",
			toolset:             "component",
			descriptionKeywords: []string{"delete", "release line"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name", "release_line_name"},
			testArgs: map[string]any{
				"namespace_name":    testNamespaceName,
				"component_name":    testComponentName,
				"release_line_name": testReleaseLineName,
			},
			expectedMethod: "DeleteReleaseLine",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[2] != testReleaseLineName {
					t.Errorf("Expected release line %s, got %v", testReleaseLineName, args[2])
				}
			},
		},
	}
}
