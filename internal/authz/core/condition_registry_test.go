// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"maps"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/stretchr/testify/require"
)

// SetConditionRegistryForTest replaces conditionRegistry for the duration of a test
// and restores the original via t.Cleanup. Use this instead of assigning to
// conditionRegistry directly so the production hot path stays unsynchronized.
func SetConditionRegistryForTest(tb testing.TB, reg map[string][]AttributeSpec) {
	tb.Helper()
	orig := maps.Clone(conditionRegistry)
	conditionRegistry = reg
	tb.Cleanup(func() { conditionRegistry = orig })
}

func TestLookupConditions(t *testing.T) {
	t.Run("known action returns expected specs", func(t *testing.T) {
		specs := LookupConditions(ActionCreateReleaseBinding)
		require.NotNil(t, specs)
		require.Len(t, specs, 1)
		require.Equal(t, AttrResourceEnvironment.Key, specs[0].Key)
	})

	t.Run("unknown action returns nil", func(t *testing.T) {
		specs := LookupConditions("component:view")
		require.Nil(t, specs)
	})

	t.Run("empty action returns nil", func(t *testing.T) {
		specs := LookupConditions("")
		require.Nil(t, specs)
	})
}

func TestIntersectConditionsForActions(t *testing.T) {
	// resource.tier and resource.region are test-only attrs not wired to production code.
	attrCT := AttributeSpec{Key: "resource.componentType", CELType: cel.StringType}
	attrLabel := AttributeSpec{Key: "resource.label", CELType: cel.StringType}

	SetConditionRegistryForTest(t, map[string][]AttributeSpec{
		ActionCreateReleaseBinding: {AttrResourceEnvironment, attrCT, attrLabel},
		ActionViewReleaseBinding:   {AttrResourceEnvironment, attrCT, attrLabel},
		ActionUpdateReleaseBinding: {AttrResourceEnvironment, attrCT, attrLabel},
		ActionDeleteReleaseBinding: {AttrResourceEnvironment, attrCT, attrLabel},
		ActionViewLogs:             {AttrResourceEnvironment, attrCT},
		ActionViewMetrics:          {AttrResourceEnvironment},
		ActionViewTraces:           {attrCT, attrLabel},
	})

	tests := []struct {
		name     string
		actions  []string
		wantKeys []string // nil → expect nil result; empty slice → expect non-nil empty
	}{
		{
			name:     "full overlap returns all attrs",
			actions:  []string{ActionCreateReleaseBinding, ActionViewReleaseBinding, ActionUpdateReleaseBinding, ActionDeleteReleaseBinding},
			wantKeys: []string{AttrResourceEnvironment.Key, attrCT.Key, attrLabel.Key},
		},
		{
			name:     "partial overlap drops missing attr",
			actions:  []string{ActionCreateReleaseBinding, ActionViewLogs},
			wantKeys: []string{AttrResourceEnvironment.Key, attrCT.Key},
		},
		{
			name:     "overlap narrows to single attr",
			actions:  []string{ActionCreateReleaseBinding, ActionViewMetrics},
			wantKeys: []string{AttrResourceEnvironment.Key},
		},
		{
			name:     "overlap on non-env attrs only",
			actions:  []string{ActionCreateReleaseBinding, ActionViewTraces},
			wantKeys: []string{attrCT.Key, attrLabel.Key},
		},
		{
			name:     "completely disjoint actions yield empty",
			actions:  []string{ActionViewMetrics, ActionViewTraces},
			wantKeys: []string{},
		},
		{
			name:     "unknown action yields empty",
			actions:  []string{ActionCreateReleaseBinding, "component:view"},
			wantKeys: []string{},
		},
		{
			name:     "resource wildcard expands and returns common attrs",
			actions:  []string{"releasebinding:*"},
			wantKeys: []string{AttrResourceEnvironment.Key, attrCT.Key, attrLabel.Key},
		},
		{
			name:     "resource wildcard intersected with concrete action",
			actions:  []string{"releasebinding:*", ActionViewMetrics},
			wantKeys: []string{AttrResourceEnvironment.Key},
		},
		{
			name:     "resource wildcard for resource with no registered attrs yields empty",
			actions:  []string{"project:*"},
			wantKeys: []string{},
		},
		{
			name:     "global wildcard yields empty (no attr common to every action)",
			actions:  []string{"*"},
			wantKeys: []string{},
		},
		{
			name:     "unknown resource wildcard yields empty",
			actions:  []string{"bogus:*"},
			wantKeys: []string{},
		},
		{
			name:     "nil input returns nil",
			actions:  nil,
			wantKeys: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IntersectConditionsForActions(tt.actions)
			if tt.wantKeys == nil {
				require.Nil(t, got)
				return
			}
			gotKeys := make([]string, len(got))
			for i, s := range got {
				gotKeys[i] = s.Key
			}
			require.ElementsMatch(t, tt.wantKeys, gotKeys)
		})
	}
}

func TestConditionRegistry(t *testing.T) {
	t.Run("keys reference valid action names", func(t *testing.T) {
		valid := make(map[string]bool, len(systemActions))
		for _, a := range systemActions {
			valid[a.Name] = true
		}
		for name := range conditionRegistry {
			if !valid[name] {
				t.Errorf("conditionRegistry has entry for unknown action %q", name)
			}
		}
	})

	t.Run("attribute keys have root.leaf format", func(t *testing.T) {
		for action, specs := range conditionRegistry {
			for _, spec := range specs {
				parts := strings.SplitN(spec.Key, ".", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					t.Errorf("action %q condition key %q is not in root.leaf format", action, spec.Key)
				}
			}
		}
	})
}

func TestAttributeSpec_RootLeaf(t *testing.T) {
	tests := []struct {
		name     string
		spec     AttributeSpec
		wantRoot string
		wantLeaf string
	}{
		{
			name:     "resource.environment splits correctly",
			spec:     AttrResourceEnvironment,
			wantRoot: "resource",
			wantLeaf: "environment",
		},
		{
			name:     "custom dotted path",
			spec:     AttributeSpec{Key: "principal.groups"},
			wantRoot: "principal",
			wantLeaf: "groups",
		},
		{
			name:     "no dot returns full key as root, empty leaf",
			spec:     AttributeSpec{Key: "nodot"},
			wantRoot: "nodot",
			wantLeaf: "",
		},
		{
			name:     "dotted path with more than two parts",
			spec:     AttributeSpec{Key: "resource.something.extra"},
			wantRoot: "resource",
			wantLeaf: "something.extra",
		},
		{
			name:     "empty key returns empty root and leaf",
			spec:     AttributeSpec{Key: ""},
			wantRoot: "",
			wantLeaf: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantRoot, tt.spec.Root())
			require.Equal(t, tt.wantLeaf, tt.spec.Leaf())
		})
	}
}
