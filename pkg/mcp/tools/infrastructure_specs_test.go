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
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListEnvironments",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "get_environment",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "env_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
				"env_name": testEnvName,
			},
			expectedMethod: "GetEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != testEnvName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testOrgName, testEnvName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_dataplanes",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListDataPlanes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "get_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"data", "plane"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "dp_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
				"dp_name":  "dp1",
			},
			expectedMethod: "GetDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != "dp1" {
					t.Errorf("Expected (%s, dp1), got (%v, %v)", testOrgName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_component_types",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "component", "type"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListComponentTypes",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "get_component_type_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"component", "type", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "ct_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
				"ct_name":  "WebApplication",
			},
			expectedMethod: "GetComponentTypeSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != "WebApplication" {
					t.Errorf("Expected (%s, WebApplication), got (%v, %v)", testOrgName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_workflows",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "workflow"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListWorkflows",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "get_workflow_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"workflow", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "workflow_name"},
			testArgs: map[string]any{
				"org_name":      testOrgName,
				"workflow_name": "workflow-1",
			},
			expectedMethod: "GetWorkflowSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != "workflow-1" {
					t.Errorf("Expected (%s, workflow-1), got (%v, %v)", testOrgName, args[0], args[1])
				}
			},
		},
		{
			name:                "list_traits",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"list", "trait"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListTraits",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
		{
			name:                "get_trait_schema",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"trait", "schema"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "trait_name"},
			testArgs: map[string]any{
				"org_name":   testOrgName,
				"trait_name": "autoscaling",
			},
			expectedMethod: "GetTraitSchema",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName || args[1] != "autoscaling" {
					t.Errorf("Expected (%s, autoscaling), got (%v, %v)", testOrgName, args[0], args[1])
				}
			},
		},
		{
			name:                "create_dataplane",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "data", "plane"},
			descriptionMinLen:   10,
			requiredParams: []string{
				"org_name", "name", "kubernetes_cluster_name", "api_server_url", "ca_cert",
				"client_cert", "client_key", "public_virtual_host", "organization_virtual_host",
			},
			optionalParams: []string{
				"display_name", "description", "observer_url", "observer_username", "observer_password",
			},
			testArgs: map[string]any{
				"org_name":                  testOrgName,
				"name":                      "new-dp",
				"kubernetes_cluster_name":   "cluster1",
				"api_server_url":            "https://api.example.com",
				"ca_cert":                   "cert-data",
				"client_cert":               "client-cert-data",
				"client_key":                "client-key-data",
				"public_virtual_host":       "public.example.com",
				"organization_virtual_host": "org.example.com",
			},
			expectedMethod: "CreateDataPlane",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
				// args[1] is *models.CreateDataPlaneRequest
			},
		},
		{
			name:                "create_environment",
			toolset:             "infrastructure",
			descriptionKeywords: []string{"create", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name", "name"},
			optionalParams:      []string{"display_name", "description", "data_plane_ref", "is_production", "dns_prefix"},
			testArgs: map[string]any{
				"org_name":       testOrgName,
				"name":           "new-env",
				"display_name":   "New Environment",
				"description":    "Test environment",
				"data_plane_ref": "dp1",
				"is_production":  false,
			},
			expectedMethod: "CreateEnvironment",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
				// args[1] is *models.CreateEnvironmentRequest
			},
		},
	}
}
