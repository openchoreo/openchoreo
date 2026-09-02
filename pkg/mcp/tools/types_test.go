// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import "testing"

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		name string
		perm ToolPermission
		want bool
	}{
		{"plain view action", ToolPermission{Action: "namespace:view"}, true},
		{"plain mutating action", ToolPermission{Action: "component:create"}, false},
		{"no actions at all", ToolPermission{}, false},
		{
			"scoped, all view",
			ToolPermission{ScopedActions: map[string]string{
				ScopeNamespace: "component:view", ScopeCluster: "clustercomponent:view",
			}},
			true,
		},
		{
			"scoped, mixed view and mutating",
			ToolPermission{ScopedActions: map[string]string{
				ScopeNamespace: "component:view", ScopeCluster: "clustercomponent:create",
			}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.perm.IsReadOnly(); got != tt.want {
				t.Errorf("IsReadOnly() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestAllToolsetTypes pins the enumeration against Toolsets' own fields, so
// a new toolset field added without a matching entry here — the drift that
// let tools/auditcoverage silently fall out of sync with production before
// this function existed — fails this test instead of shipping unnoticed.
func TestAllToolsetTypes(t *testing.T) {
	want := []ToolsetType{
		ToolsetNamespace, ToolsetProject, ToolsetComponent, ToolsetDeployment,
		ToolsetBuild, ToolsetPE, ToolsetResource,
	}
	got := AllToolsetTypes()

	if len(got) != len(want) {
		t.Fatalf("AllToolsetTypes() has %d entries, want %d: %v", len(got), len(want), got)
	}
	for _, ts := range want {
		if !got[ts] {
			t.Errorf("AllToolsetTypes() missing %q", ts)
		}
	}
}

func TestNewToolsets(t *testing.T) {
	handler := NewMockCoreToolsetHandler()

	t.Run("subset leaves the rest nil", func(t *testing.T) {
		ts := NewToolsets(handler, map[ToolsetType]bool{ToolsetProject: true, ToolsetBuild: true})
		if ts.ProjectToolset == nil || ts.BuildToolset == nil {
			t.Fatal("NewToolsets() left an enabled toolset nil")
		}
		if ts.NamespaceToolset != nil || ts.ComponentToolset != nil || ts.DeploymentToolset != nil ||
			ts.PEToolset != nil || ts.ResourceToolset != nil {
			t.Error("NewToolsets() backed a toolset that wasn't in the enabled map")
		}
	})

	t.Run("AllToolsetTypes backs every field", func(t *testing.T) {
		ts := NewToolsets(handler, AllToolsetTypes())
		if ts.NamespaceToolset == nil || ts.ProjectToolset == nil || ts.ComponentToolset == nil ||
			ts.DeploymentToolset == nil || ts.BuildToolset == nil || ts.PEToolset == nil || ts.ResourceToolset == nil {
			t.Errorf("NewToolsets(AllToolsetTypes()) left a field nil: %+v", ts)
		}
	})

	t.Run("empty map backs nothing", func(t *testing.T) {
		ts := NewToolsets(handler, nil)
		if *ts != (Toolsets{}) {
			t.Errorf("NewToolsets(nil) = %+v, want a zero-value Toolsets", ts)
		}
	})
}
