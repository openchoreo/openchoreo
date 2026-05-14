// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAllActions tests loading all system-defined actions
func TestAllActions(t *testing.T) {
	actions := AllActions()

	t.Run("returns non-empty list", func(t *testing.T) {
		if len(actions) == 0 {
			t.Error("AllActions() returned empty actions list")
		}
	})

	t.Run("returns expected action count", func(t *testing.T) {
		if len(actions) != len(systemActions) {
			t.Errorf("AllActions() returned %d actions, expected %d (len of systemActions)", len(actions), len(systemActions))
		}
	})

	t.Run("no duplicate action names", func(t *testing.T) {
		seen := make(map[string]bool)
		for _, action := range actions {
			if seen[action.Name] {
				t.Errorf("Duplicate action name found: %s", action.Name)
			}
			seen[action.Name] = true
		}
	})

	t.Run("action names follow resource:operation format", func(t *testing.T) {
		for _, action := range actions {
			parts := strings.Split(action.Name, ":")
			if len(parts) != 2 {
				t.Errorf("Action name %q does not follow 'resource:operation' format", action.Name)
				continue
			}
			if parts[0] == "" {
				t.Errorf("Action name %q has empty resource part", action.Name)
			}
			if parts[1] == "" {
				t.Errorf("Action name %q has empty operation part", action.Name)
			}
		}
	})

	t.Run("action names are lowercase", func(t *testing.T) {
		for _, action := range actions {
			if !strings.Contains(action.Name, "*") && action.Name != strings.ToLower(action.Name) {
				t.Errorf("Action name %q should be lowercase", action.Name)
			}
		}
	})

	t.Run("all actions have a valid LowestScope", func(t *testing.T) {
		valid := map[ActionScope]bool{
			ScopeCluster: true, ScopeNamespace: true,
			ScopeProject: true, ScopeComponent: true,
		}
		for _, action := range actions {
			if !valid[action.LowestScope] {
				t.Errorf("Action %q has invalid LowestScope %q", action.Name, action.LowestScope)
			}
		}
	})
}

// TestPublicActions tests listing public actions
func TestPublicActions(t *testing.T) {
	actions := PublicActions()

	t.Run("returns non-empty list", func(t *testing.T) {
		if len(actions) == 0 {
			t.Error("PublicActions() returned empty actions list")
		}
	})

	t.Run("excludes internal actions", func(t *testing.T) {
		for _, action := range actions {
			if action.IsInternal {
				t.Errorf("PublicActions() returned internal action: %s", action.Name)
			}
		}
	})

	t.Run("actions with conditions in registry have conditions populated", func(t *testing.T) {
		for _, a := range actions {
			if _, inRegistry := conditionRegistry[a.Name]; inRegistry {
				if len(a.Conditions) == 0 {
					t.Errorf("action %q expected conditions but got none", a.Name)
				}
			}
		}
	})

	t.Run("actions not in registry have no conditions", func(t *testing.T) {
		for _, a := range actions {
			if _, inRegistry := conditionRegistry[a.Name]; !inRegistry {
				if len(a.Conditions) != 0 {
					t.Errorf("action %q not in registry but got %d conditions", a.Name, len(a.Conditions))
				}
			}
		}
	})
}

// TestExpandActionPattern tests pattern expansion for concrete actions and wildcards.
func TestExpandActionPattern(t *testing.T) {
	t.Run("empty pattern returns nil", func(t *testing.T) {
		require.Nil(t, ExpandActionPattern(""))
	})

	t.Run("concrete known action returns itself", func(t *testing.T) {
		got := ExpandActionPattern(ActionCreateReleaseBinding)
		require.Equal(t, []string{ActionCreateReleaseBinding}, got)
	})

	t.Run("unknown concrete action returns nil", func(t *testing.T) {
		require.Nil(t, ExpandActionPattern("bogus:action"))
	})

	t.Run("resource wildcard expands to all that resource's actions", func(t *testing.T) {
		got := ExpandActionPattern("releasebinding:*")
		require.ElementsMatch(t, []string{
			ActionViewReleaseBinding,
			ActionCreateReleaseBinding,
			ActionUpdateReleaseBinding,
			ActionDeleteReleaseBinding,
		}, got)
	})

	t.Run("resource wildcard for unknown resource returns empty", func(t *testing.T) {
		got := ExpandActionPattern("bogus:*")
		require.Empty(t, got)
	})

	t.Run("global wildcard returns all concrete public actions", func(t *testing.T) {
		got := ExpandActionPattern("*")
		require.Equal(t, len(ConcretePublicActions()), len(got))
	})

	t.Run("resource wildcard prefix matches on full resource token", func(t *testing.T) {
		// "cluster:*" must not match "clustertrait:view", "clusterdataplane:view", etc.
		// Prefix match is on the full "cluster:" token, not the substring "cluster".
		got := ExpandActionPattern("cluster:*")
		require.Empty(t, got)
	})
}

// TestConcretePublicActions tests listing concrete public actions
func TestConcretePublicActions(t *testing.T) {
	actions := ConcretePublicActions()

	t.Run("returns non-empty list", func(t *testing.T) {
		if len(actions) == 0 {
			t.Error("ConcretePublicActions() returned empty actions list")
		}
	})

	t.Run("excludes wildcarded actions", func(t *testing.T) {
		for _, action := range actions {
			if strings.Contains(action.Name, "*") {
				t.Errorf("ConcretePublicActions() returned wildcarded action: %s", action.Name)
			}
		}
	})

	t.Run("excludes internal actions", func(t *testing.T) {
		for _, action := range actions {
			if action.IsInternal {
				t.Errorf("ConcretePublicActions() returned internal action: %s", action.Name)
			}
		}
	})

	t.Run("actions with conditions in registry have conditions populated", func(t *testing.T) {
		for _, a := range actions {
			if _, inRegistry := conditionRegistry[a.Name]; inRegistry {
				if len(a.Conditions) == 0 {
					t.Errorf("action %q expected conditions but got none", a.Name)
				}
			}
		}
	})

	t.Run("actions not in registry have no conditions", func(t *testing.T) {
		for _, a := range actions {
			if _, inRegistry := conditionRegistry[a.Name]; !inRegistry {
				if len(a.Conditions) != 0 {
					t.Errorf("action %q not in registry but got %d conditions", a.Name, len(a.Conditions))
				}
			}
		}
	})
}
