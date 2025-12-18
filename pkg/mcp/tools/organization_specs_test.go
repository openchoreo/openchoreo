// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

// organizationToolSpecs returns test specs for organization toolset
func organizationToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_organizations",
			toolset:             "organization",
			descriptionKeywords: []string{"organization"},
			descriptionMinLen:   10,
			requiredParams:      []string{},
			optionalParams:      []string{},
			testArgs:            map[string]any{},
			expectedMethod:      "ListOrganizations",
			validateCall: func(t *testing.T, args []interface{}) {
				// ListOrganizations takes no arguments
				if len(args) != 0 {
					t.Errorf("Expected no arguments for ListOrganizations, got %d", len(args))
				}
			},
		},
		{
			name:                "get_organization",
			toolset:             "organization",
			descriptionKeywords: []string{"organization"},
			descriptionMinLen:   10,
			requiredParams:      []string{"name"},
			optionalParams:      []string{},
			testArgs:            map[string]any{"name": "test-org"},
			expectedMethod:      "GetOrganization",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != "test-org" {
					t.Errorf("Expected org name 'test-org', got %v", args[0])
				}
			},
		},
		{
			name:                "list_secret_references",
			toolset:             "organization",
			descriptionKeywords: []string{"list", "secret", "reference"},
			descriptionMinLen:   10,
			requiredParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name": testOrgName,
			},
			expectedMethod: "ListSecretReferences",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testOrgName {
					t.Errorf("Expected org name %q, got %v", testOrgName, args[0])
				}
			},
		},
	}
}
