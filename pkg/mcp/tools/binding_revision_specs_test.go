// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"strings"
	"testing"
)

// testReleaseBindingName is the binding the rollback specs target;
// testResourceReleaseBindingName comes from resource_release_binding_specs_test.go.
const testReleaseBindingName = "my-component-development"

// deploymentBindingRevisionSpecs returns specs for the revision history and
// rollback tools of ReleaseBindings and ResourceReleaseBindings.
func deploymentBindingRevisionSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_release_binding_revisions",
			toolset:             "deployment",
			descriptionKeywords: []string{"revision", "knownGood", "rollback_release_binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "component_name"},
			optionalParams:      []string{"environment", "limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"component_name": testComponentName,
				"environment":    "development",
			},
			expectedMethod: "ListReleaseBindingRevisions",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testComponentName || args[2] != "development" {
					t.Errorf("Expected (%s, %s, development), got (%v, %v, %v)",
						testNamespaceName, testComponentName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_release_binding_revision",
			toolset:             "deployment",
			descriptionKeywords: []string{"revision", "binding spec"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testReleaseBindingName + "-r3",
			},
			expectedMethod: "GetReleaseBindingRevision",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testReleaseBindingName+"-r3" {
					t.Errorf("Expected (%s, %s-r3), got (%v, %v)", testNamespaceName, testReleaseBindingName, args[0], args[1])
				}
			},
		},
		{
			name:    "rollback_release_binding",
			toolset: "deployment",
			// The description must carry the procedure: dry run first, prefer
			// knownGood, report warnings and get confirmation.
			descriptionKeywords: []string{"dry_run=true", "knownGood", "warnings", "confirm", "dry_run=false"},
			descriptionMinLen:   200,
			requiredParams:      []string{"namespace_name", "name", "revision"},
			optionalParams:      []string{"dry_run", "message"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testReleaseBindingName,
				"revision":       3,
			},
			expectedMethod: "RollbackReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testReleaseBindingName || args[2] != int64(3) {
					t.Errorf("Expected (%s, %s, 3), got (%v, %v, %v)",
						testNamespaceName, testReleaseBindingName, args[0], args[1], args[2])
				}
				if args[3] != true {
					t.Errorf("dry_run must default to true when omitted, got %v", args[3])
				}
			},
		},
		{
			name:                "list_resource_release_binding_revisions",
			toolset:             "deployment",
			descriptionKeywords: []string{"revision", "knownGood", "rollback_resource_release_binding"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "resource_name"},
			optionalParams:      []string{"environment", "limit", "cursor"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"resource_name":  "my-db",
			},
			expectedMethod: "ListResourceReleaseBindingRevisions",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != "my-db" || args[2] != "" {
					t.Errorf("Expected (%s, my-db, \"\"), got (%v, %v, %v)", testNamespaceName, args[0], args[1], args[2])
				}
			},
		},
		{
			name:                "get_resource_release_binding_revision",
			toolset:             "deployment",
			descriptionKeywords: []string{"revision", "binding spec"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseBindingName + "-r1",
			},
			expectedMethod: "GetResourceReleaseBindingRevision",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[1] != testResourceReleaseBindingName+"-r1" {
					t.Errorf("Expected %s-r1, got %v", testResourceReleaseBindingName, args[1])
				}
			},
		},
		{
			name:                "rollback_resource_release_binding",
			toolset:             "deployment",
			descriptionKeywords: []string{"dry_run=true", "knownGood", "warnings", "confirm", "dry_run=false"},
			descriptionMinLen:   200,
			requiredParams:      []string{"namespace_name", "name", "revision"},
			optionalParams:      []string{"dry_run", "message"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testResourceReleaseBindingName,
				"revision":       1,
				"dry_run":        false,
			},
			expectedMethod: "RollbackResourceReleaseBinding",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[1] != testResourceReleaseBindingName || args[2] != int64(1) {
					t.Errorf("Expected (%s, 1), got (%v, %v)", testResourceReleaseBindingName, args[1], args[2])
				}
				if args[3] != false {
					t.Errorf("an explicit dry_run=false must reach the handler, got %v", args[3])
				}
			},
		},
	}
}

func TestRollbackArgs_DryRunDefaultsToTrue(t *testing.T) {
	no, yes := false, true
	tests := []struct {
		name string
		args rollbackArgs
		want bool
	}{
		{name: "omitted", args: rollbackArgs{}, want: true},
		{name: "explicit true", args: rollbackArgs{DryRun: &yes}, want: true},
		{name: "explicit false", args: rollbackArgs{DryRun: &no}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.args.dryRun(); got != tt.want {
				t.Errorf("dryRun() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRollbackSchema_AdvertisesSafeDefault(t *testing.T) {
	props := rollbackProperties("binding")
	dryRun, ok := props["dry_run"].(map[string]any)
	if !ok {
		t.Fatal("dry_run property missing")
	}
	if dryRun["default"] != true {
		t.Errorf("dry_run default = %v, want true", dryRun["default"])
	}
	if desc, _ := dryRun["description"].(string); !strings.Contains(desc, "confirmation") {
		t.Errorf("dry_run description must say a real rollback needs the user's confirmation: %q", desc)
	}
}
