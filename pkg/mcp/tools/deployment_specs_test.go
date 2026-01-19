// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// deploymentToolSpecs returns test specs for deployment toolset
func deploymentToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_deployment_pipeline",
			toolset:             "deployment",
			descriptionKeywords: []string{"deployment", "pipeline"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "project_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"project_name":   testProjectName,
			},
			expectedMethod: "GetProjectDeploymentPipeline",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testProjectName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testProjectName, args[0], args[1])
				}
			},
		},
	}
}
