// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
	t.Run("single known action returns its specs", func(t *testing.T) {
		specs := IntersectConditionsForActions([]string{ActionCreateReleaseBinding})
		require.Len(t, specs, 1)
		require.Equal(t, AttrResourceEnvironment.Key, specs[0].Key)
	})

	t.Run("multiple known actions sharing an attr returns that attr", func(t *testing.T) {
		// All releasebinding actions share AttrResourceEnvironment.
		specs := IntersectConditionsForActions([]string{
			ActionCreateReleaseBinding,
			ActionUpdateReleaseBinding,
			ActionDeleteReleaseBinding,
		})
		require.Len(t, specs, 1)
		require.Equal(t, AttrResourceEnvironment.Key, specs[0].Key)
	})

	t.Run("disjoint actions (one unknown) returns empty", func(t *testing.T) {
		specs := IntersectConditionsForActions([]string{
			ActionCreateReleaseBinding,
			"component:view",
		})
		require.Empty(t, specs)
	})

	t.Run("empty input returns nil", func(t *testing.T) {
		specs := IntersectConditionsForActions(nil)
		require.Nil(t, specs)
	})

	t.Run("single unknown action returns empty", func(t *testing.T) {
		specs := IntersectConditionsForActions([]string{"unknown:action"})
		require.Empty(t, specs)
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
