// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"
)

const testEndpointReleaseName = "orders-api-abc123"

// peEndpointReleaseSpecs returns EndpointRelease list/get specs. There is no
// create tool: releases are cut by the Endpoint controller.
func peEndpointReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_endpoint_releases",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "endpoint", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "endpoint_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"endpoint_name":  testEndpointName,
			},
			expectedMethod: "ListEndpointReleases",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointName, args[0], args[1])
				}
			},
		},
		{
			name:                "get_endpoint_release",
			toolset:             "pe",
			descriptionKeywords: []string{"endpoint", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointReleaseName,
			},
			expectedMethod: "GetEndpointRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointReleaseName, args[0], args[1])
				}
			},
		},
	}
}

// deploymentEndpointReleaseSpecs returns the dev-side delete spec (mirrors
// delete_component_release on the deployment toolset).
func deploymentEndpointReleaseSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "delete_endpoint_release",
			toolset:             "deployment",
			descriptionKeywords: []string{"delete", "endpoint", "release"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointReleaseName,
			},
			expectedMethod: "DeleteEndpointRelease",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointReleaseName {
					t.Errorf("Expected (%s, %s), got (%v, %v)",
						testNamespaceName, testEndpointReleaseName, args[0], args[1])
				}
			},
		},
	}
}
