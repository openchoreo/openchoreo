// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// peToolSpecs returns test specs for platform engineering toolset
func peToolSpecs() []toolTestSpec {
	specs := peCoreSpecs()
	specs = append(specs, peClusterSpecs()...)
	return specs
}

func peCoreSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "create_environment",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			optionalParams:      []string{"display_name", "description", "data_plane_ref", "is_production", "dns_prefix"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           "new-env",
				"display_name":   "New Environment",
				"description":    "Test environment",
				"data_plane_ref": "dp1",
				"is_production":  false,
			},
			expectedMethod: "CreateEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateEnvironmentRequest
			},
		},
		{
			name:                "list_dataplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "get_dataplane",
			toolset:             "pe",
			descriptionKeywords: []string{"data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "dp_name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"dp_name":        "dp1",
			},
			expectedMethod: "GetDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "dp1" {
					t.Errorf("Expected (%s, dp1), got (%v, %v)", testNamespaceName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_dataplane",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "cluster_agent_client_ca"},
			optionalParams: []string{
				"display_name", "description", "observability_plane_ref",
			},
			testArgs: map[string]any{
				"namespace_name":          testNamespaceName,
				"name":                    "new-dp",
				"cluster_agent_client_ca": "-----BEGIN CERTIFICATE-----\ntest-ca-cert-data\n-----END CERTIFICATE-----",
			},
			expectedMethod: "CreateDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
				// args[1] is *models.CreateDataPlaneRequest
			},
		},
		{
			name:                "list_observability_planes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "observability", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListObservabilityPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
		{
			name:                "list_buildplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"list", "build", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name"},
			optionalParams:      []string{"limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
			},
			expectedMethod: "ListBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName {
					t.Errorf("Expected namespace %q, got %v", testNamespaceName, args[0])
				}
			},
		},
	}
}

func peClusterSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_cluster_dataplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "data", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "get_cluster_dataplane",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"cdp_name"},
			testArgs: map[string]any{
				"cdp_name": "cdp1",
			},
			expectedMethod: "GetClusterDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "cdp1" {
					t.Errorf("Expected cdp_name %q, got %v", "cdp1", args[0])
				}
			},
		},
		{
			name:                "create_cluster_dataplane",
			toolset:             "pe",
			descriptionKeywords: []string{"create", "cluster", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name", "plane_id", "cluster_agent_client_ca"},
			optionalParams: []string{
				"display_name", "description", "public_http_port", "public_https_port",
				"organization_http_port", "organization_https_port", "observability_plane_ref",
			},
			testArgs: map[string]any{
				"name":                    "new-cdp",
				"plane_id":                "us-west-prod",
				"cluster_agent_client_ca": "-----BEGIN CERTIFICATE-----\ntest-ca\n-----END CERTIFICATE-----",
			},
			expectedMethod: "CreateClusterDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				// args[0] is *models.CreateClusterDataPlaneRequest
			},
		},
		{
			name:                "list_cluster_buildplanes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "build", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterBuildPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
		{
			name:                "list_cluster_observability_planes",
			toolset:             "pe",
			descriptionKeywords: []string{"cluster", "observability", "plane"},
			descriptionMinLen:   10,
			optionalParams:      []string{"limit", "cursor"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListClusterObservabilityPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				// Only ListOpts argument
			},
		},
	}
}
