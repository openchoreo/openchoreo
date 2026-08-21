// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

func TestNewPolicySet_RejectsInadmissibleSelectors(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
	}{
		{
			name: "actors selector used for suppression rejected",
			policy: Policy{
				Match: Selector{Actors: []string{"user-123"}},
				Set:   PartialSettings{Publish: new(false)},
			},
		},
		{
			name: "entitlements selector used for suppression rejected",
			policy: Policy{
				Match: Selector{Entitlements: []string{"admin"}},
				Set:   PartialSettings{Publish: new(false)},
			},
		},
		{
			name: "policy that sets nothing rejected",
			policy: Policy{
				Match: Selector{Categories: []Category{CategoryManagement}},
				Set:   PartialSettings{},
			},
		},
		{
			name: "empty match with publish:false rejected",
			policy: Policy{
				Match: Selector{},
				Set:   PartialSettings{Publish: new(false)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, []Policy{tt.policy})
			if len(errs) == 0 {
				t.Fatalf("expected validation errors, got none")
			}
		})
	}
}

func TestNewPolicySet_AllowsAdmissibleSelectors(t *testing.T) {
	tests := []struct {
		name   string
		policy Policy
	}{
		{
			name: "actors selector for escalation (publish:true) is fine",
			policy: Policy{
				Match: Selector{Actors: []string{"user-123"}},
				Set:   PartialSettings{Publish: new(true)},
			},
		},
		{
			name: "entitlements selector for escalation (publish:true) is fine",
			policy: Policy{
				Match: Selector{Entitlements: []string{"admin"}},
				Set:   PartialSettings{Publish: new(true)},
			},
		},
		{
			name: "results selector on a publish-only rule is fine",
			policy: Policy{
				Match: Selector{Results: []Result{ResultFailure}},
				Set:   PartialSettings{Publish: new(false)},
			},
		},
		{
			name: "actor_types selector for suppression is fine",
			policy: Policy{
				Match: Selector{ActorTypes: []string{"service_account"}},
				Set:   PartialSettings{Publish: new(false)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, []Policy{tt.policy})
			if len(errs) != 0 {
				t.Fatalf("expected no validation errors, got: %v", errs)
			}
		})
	}
}

func TestPolicySet_Resolve_FirstMatchWins(t *testing.T) {
	ps, errs := NewPolicySet(coreconfig.NewPath("audit"),
		Settings{Publish: true},
		[]Policy{
			{
				Match: Selector{Categories: []Category{CategoryAuthorization}},
				Set:   PartialSettings{Publish: new(false)},
			},
			{
				Match: Selector{Actions: []string{"create_workflow_run"}, ActorTypes: []string{"service_account"}},
				Set:   PartialSettings{Publish: new(false)},
			},
		})
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	t.Run("matches first rule by category", func(t *testing.T) {
		settings := ps.Resolve(ResolveContext{
			Operation: &Operation{ID: "createAuthzRole", Category: CategoryAuthorization},
		})
		if settings.Publish {
			t.Errorf("settings.Publish = true, want false (suppressed by category rule)")
		}
	})

	t.Run("matches second rule by action+actor_type", func(t *testing.T) {
		settings := ps.Resolve(ResolveContext{
			Operation: &Operation{ID: "createWorkflowRun", Action: "create_workflow_run", Category: CategoryManagement},
			Actor:     Actor{Type: "service_account"},
		})
		if settings.Publish {
			t.Errorf("settings.Publish = true, want false (suppressed)")
		}
	})

	t.Run("falls through to defaults when nothing matches", func(t *testing.T) {
		settings := ps.Resolve(ResolveContext{
			Operation: &Operation{ID: "createProject", Action: "create_project", Category: CategoryManagement},
		})
		if !settings.Publish {
			t.Errorf("settings = %+v, want the bare defaults (Publish=true)", settings)
		}
	})

	t.Run("nil operation falls through operation-shaped selectors safely", func(t *testing.T) {
		settings := ps.Resolve(ResolveContext{Operation: nil})
		if !settings.Publish {
			t.Errorf("settings.Publish = false, want true (defaults, no panic on nil Operation)")
		}
	})
}

// TestEntitlementsMatch guards Actor.Entitlements' type: reads values
// directly as []string, no type assertion to silently fail closed on.
func TestEntitlementsMatch(t *testing.T) {
	tests := []struct {
		name         string
		entitlements map[string][]string
		want         []string
		wantMatch    bool
	}{
		{name: "nil entitlements never match", entitlements: nil, want: []string{"admin"}, wantMatch: false},
		{
			name: "matching value in claim", entitlements: map[string][]string{"groups": {"admin", "dev"}},
			want: []string{"admin"}, wantMatch: true,
		},
		{
			name: "no overlap", entitlements: map[string][]string{"groups": {"dev"}},
			want: []string{"admin"}, wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entitlementsMatch(tt.entitlements, tt.want); got != tt.wantMatch {
				t.Errorf("entitlementsMatch(%v, %v) = %v, want %v", tt.entitlements, tt.want, got, tt.wantMatch)
			}
		})
	}
}

func TestPolicySet_ImmutableAfterConstruction(t *testing.T) {
	suppress := false
	match := Selector{Actions: []string{"create_project"}}
	policies := []Policy{{Match: match, Set: PartialSettings{Publish: &suppress}}}

	ps, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, policies)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	// Mutate the caller's copies after construction.
	suppress = true
	match.Actions[0] = "delete_project"

	settings := ps.Resolve(ResolveContext{
		Operation: &Operation{ID: "createProject", Action: "create_project", Category: CategoryManagement},
	})
	if settings.Publish {
		t.Errorf("settings.Publish = true, want false — PolicySet must have cloned Set.Publish and Match.Actions at construction")
	}
}
