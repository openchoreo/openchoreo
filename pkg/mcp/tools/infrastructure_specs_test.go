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
			name:                "get_deployment_pipeline",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "pipeline_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"pipeline_name":  "default-pipeline",
			},
			expectedMethod: "GetDeploymentPipeline",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "default-pipeline" {
					t.Errorf("Expected (%s, default-pipeline), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_deployment_pipelines",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListDeploymentPipelines",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_observer_url",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"observer", "url"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "env_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"env_name":       testEnvName,
			},
			expectedMethod: "GetObserverURL",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEnvName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testEnvName, args[0], args[1])
				}
			},
		},
	}
}
