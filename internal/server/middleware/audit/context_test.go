// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
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
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})

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
	ctx, data := NewAuditContext(context.Background(), &Resource{Name: "audit-mcp-comp-3"}, RequestInfo{})

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
	ctx, data := NewAuditContext(context.Background(), &Resource{Namespace: "ns-1"}, RequestInfo{})

	SetHierarchy(ctx, Hierarchy{Namespace: "ns-1", Project: "p1", Component: "c1"})
	SetResource(ctx, &Resource{Namespace: "ns-1", UID: "uid-123", Name: "c1"})

	want := Hierarchy{Namespace: "ns-1", Project: "p1", Component: "c1"}
	if data.Hierarchy != want {
		t.Errorf("Hierarchy = %+v, want %+v (SetResource must not wipe a previously recorded hierarchy)", data.Hierarchy, want)
	}
	if data.Resource == nil || data.Resource.UID != "uid-123" {
		t.Errorf("Resource = %+v, want SetResource's write to still take effect", data.Resource)
	}
}

// TestSetters_ConcurrentCallsAreSafe mirrors query_platform_logs, which runs
// several authz-wrapped reads in parallel on one context. Meaningful under
// -race.
func TestSetters_ConcurrentCallsAreSafe(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			SetHierarchy(ctx, Hierarchy{Namespace: "ns"})
			SetRequest(ctx, map[string]any{"podName": []string{"p"}})
			SetReturned(ctx, Returned{Count: 1})
			SetResult(ctx, ResultDenied)
			SetResource(ctx, &Resource{Name: "r"})
		})
	}
	wg.Wait()

	if data.Hierarchy.Namespace != "ns" || data.Returned == nil || data.Parameters == nil {
		t.Errorf("AuditData = %+v, want every setter's write", data)
	}
}

func TestSetRequestAndSetReturned_NoAuditData_NoOp(t *testing.T) {
	SetRequest(context.Background(), map[string]any{"searchPhrase": "x"})
	SetReturned(context.Background(), Returned{Count: 1})
}

func TestSetRequest_CapsStringsAndCopies(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})

	long := strings.Repeat("é", MaxParameterLength+10)
	params := map[string]any{
		"searchPhrase": long,
		"logLevels":    []string{long, "ERROR"},
		"limit":        100,
		"actor":        map[string]any{"id": []string{long}},
	}
	SetRequest(ctx, params)
	params["searchPhrase"] = "changed"

	got := data.Parameters
	phrase, _ := got["searchPhrase"].(string)
	if n := utf8.RuneCountInString(phrase); n != MaxParameterLength {
		t.Errorf("searchPhrase has %d runes, want it cut to %d", n, MaxParameterLength)
	}
	levels, _ := got["logLevels"].([]string)
	if len(levels) != 2 || utf8.RuneCountInString(levels[0]) != MaxParameterLength || levels[1] != "ERROR" {
		t.Errorf("logLevels = %v, want each string cut to %d runes", levels, MaxParameterLength)
	}
	if got["limit"] != 100 {
		t.Errorf("limit = %v, want non-string values kept as they are", got["limit"])
	}
	actor, _ := got["actor"].(map[string]any)
	ids, _ := actor["id"].([]string)
	if len(ids) != 1 || utf8.RuneCountInString(ids[0]) != MaxParameterLength {
		t.Errorf("actor.id = %v, want nested strings cut to %d runes", actor["id"], MaxParameterLength)
	}
}

func TestSetRequest_EmptyParamsRecordNone(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})
	SetRequest(ctx, map[string]any{})
	if data.Parameters != nil {
		t.Errorf("Parameters = %+v, want none recorded", data.Parameters)
	}
}

func TestSetReturned_CopiesItems(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})
	items := []ReturnedItem{{Type: "secret", Name: "db-creds"}}
	SetReturned(ctx, Returned{Count: 1, Items: items})
	items[0].Name = "changed"

	if data.Returned == nil || data.Returned.Items[0].Name != "db-creds" {
		t.Errorf("Returned = %+v, want the items as they were at the call", data.Returned)
	}
}

func TestSetReturned_CapsItemsAndKeepsCount(t *testing.T) {
	ctx, data := NewAuditContext(context.Background(), &Resource{}, RequestInfo{})
	items := make([]ReturnedItem, MaxReturnedItems+5)
	SetReturned(ctx, Returned{Count: len(items), Items: items})

	got := data.Returned
	if len(got.Items) != MaxReturnedItems || got.Count != MaxReturnedItems+5 {
		t.Errorf("got %d items and count %d, want %d items and count %d",
			len(got.Items), got.Count, MaxReturnedItems, MaxReturnedItems+5)
	}
}
