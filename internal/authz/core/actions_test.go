// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
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
		expectedCount := 53 // Update this when intentionally adding/removing actions
		if len(actions) != expectedCount {
			t.Errorf("Expected %d actions, got %d. Update expected count if intentional.", expectedCount, len(actions))
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
}
