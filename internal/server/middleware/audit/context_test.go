// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"testing"
)

func TestSeedHierarchy_NoAuditData_NoOp(t *testing.T) {
	// Must not panic, and there is nothing to assert against — this only
	// guards that a bare context.Background() (no NewAuditContext call, e.g.
	// an unaudited GET) is safely ignored.
	SeedHierarchy(context.Background(), Hierarchy{Project: "p1"})
}

func TestSetHierarchy_NoAuditData_NoOp(t *testing.T) {
	SetHierarchy(context.Background(), Hierarchy{Project: "p1"})
}

// TestSetHierarchy_OverridesSeed guards the common MCP case: a pre-call seed
// fires, then the service-layer Check resolves the real hierarchy off a
// fetched object — the resolved value must win over the claimed one.
func TestSetHierarchy_OverridesSeed(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{})

	SeedHierarchy(ctx, Hierarchy{Project: "claimed-project", Component: "claimed-component"})
	SetHierarchy(ctx, Hierarchy{Project: "resolved-project", Component: "resolved-component"})

	want := Hierarchy{Project: "resolved-project", Component: "resolved-component"}
	if data.Hierarchy != want {
		t.Errorf("Hierarchy = %+v, want %+v (SetHierarchy must override SeedHierarchy)", data.Hierarchy, want)
	}
}

// TestSetHierarchy_LastCheckWins reproduces, at the unit level, the sequence
// observed live against a running cluster for MCP's create_component: a
// pre-call seed (namespace+project only — the seed can't name the
// not-yet-created component), then a precondition Check resolving the
// referenced ComponentType (namespace only), then a precondition Check
// resolving it as a ClusterComponentType instead (empty — cluster-scoped),
// then finally the real create_component Check with the full hierarchy.
//
// An earlier "first Check wins" rule locked in the ComponentType check's
// namespace-only hierarchy and silently dropped project/component from the
// emitted event — a real regression this test would have caught. Last-write
// wins because every *ServiceWithAuthz wrapper Checks before delegating, so
// a precondition lookup's Check always completes before the enclosing
// operation's own — see SetHierarchy's doc comment.
func TestSetHierarchy_LastCheckWins(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{Name: "audit-mcp-comp-3"})

	SeedHierarchy(ctx, Hierarchy{Namespace: "default", Project: "audit-test-proj"})
	SetHierarchy(ctx, Hierarchy{Namespace: "default"})                                                            // componenttype:view precondition
	SetHierarchy(ctx, Hierarchy{})                                                                                // clustercomponenttype:view precondition
	SetHierarchy(ctx, Hierarchy{Namespace: "default", Project: "audit-test-proj", Component: "audit-mcp-comp-3"}) // the real component:create check

	want := Hierarchy{Namespace: "default", Project: "audit-test-proj", Component: "audit-mcp-comp-3"}
	if data.Hierarchy != want {
		t.Errorf("Hierarchy = %+v, want %+v (the last Check call must win)", data.Hierarchy, want)
	}
}

// TestSetHierarchy_SurvivesLaterSetResource guards the entire reason
// Hierarchy is stored as a sibling of Resource rather than merged into it:
// SetResource replaces the whole *Resource, so a later handler call setting
// the real resource ID/name must not wipe out a hierarchy an earlier authz
// check already recorded.
func TestSetHierarchy_SurvivesLaterSetResource(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{Namespace: "ns-1"})

	SetHierarchy(ctx, Hierarchy{Namespace: "ns-1", Project: "p1", Component: "c1"})
	SetResource(ctx, &Resource{Namespace: "ns-1", ID: "uid-123", Name: "c1"})

	want := Hierarchy{Namespace: "ns-1", Project: "p1", Component: "c1"}
	if data.Hierarchy != want {
		t.Errorf("Hierarchy = %+v, want %+v (SetResource must not wipe a previously recorded hierarchy)", data.Hierarchy, want)
	}
	if data.Resource == nil || data.Resource.ID != "uid-123" {
		t.Errorf("Resource = %+v, want SetResource's write to still take effect", data.Resource)
	}
}
