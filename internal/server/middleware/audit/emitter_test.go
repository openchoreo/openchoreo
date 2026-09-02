// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// testCreateProjectAction avoids a goconst violation from the repeated
// "create_project" literal across this file's fixtures and assertions.
const testCreateProjectAction = "create_project"

// recordingSink is a test double that records every Event it receives.
type recordingSink struct {
	events []*Event
}

func (s *recordingSink) LogEvent(event *Event) {
	s.events = append(s.events, event)
}

// TestNewEmitter_ErrorsOnNilPolicySet guards against the failure moving from
// construction time (a clear error here) to inside a deferred EmitFromContext
// call on the first request — after the response is already written, and
// outside that defer's own handler-panic recover (where it could replace the
// real handler panic being re-raised instead of letting it through).
func TestNewEmitter_ErrorsOnNilPolicySet(t *testing.T) {
	emitter, err := NewEmitter("test-service", nil, &recordingSink{})
	if err == nil {
		t.Fatal("expected NewEmitter to error on a nil *PolicySet, got none")
	}
	if emitter != nil {
		t.Errorf("expected a nil Emitter alongside the error, got %+v", emitter)
	}
}

func TestEmitter_FansOutToEverySink(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sinkA := &recordingSink{}
	sinkB := &recordingSink{}
	emitter, err := NewEmitter("test-service", policies, sinkA, sinkB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "project", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultSuccess})

	if len(sinkA.events) != 1 || len(sinkB.events) != 1 {
		t.Fatalf("expected exactly one event per sink, got sinkA=%d sinkB=%d", len(sinkA.events), len(sinkB.events))
	}
	if sinkA.events[0].Action != testCreateProjectAction || sinkB.events[0].Action != testCreateProjectAction {
		t.Errorf("expected both sinks to receive the same event, got sinkA=%+v sinkB=%+v", sinkA.events[0], sinkB.events[0])
	}
	if sinkA.events[0] != sinkB.events[0] {
		t.Error("expected both sinks to receive the identical *Event (built once, shared across sinks), got two different pointers")
	}
}

// TestEmitter_StampsIdentityForEverySink guards against a sink mutating the
// shared *Event to fill in its own identity — buildEvent stamps
// EventID/Timestamp/Service once before any sink sees the event, so all
// sinks must agree.
func TestEmitter_StampsIdentityForEverySink(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sinkA := &recordingSink{}
	sinkB := &recordingSink{}
	emitter, err := NewEmitter("test-service", policies, sinkA, sinkB)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "project", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultSuccess})

	for name, sink := range map[string]*recordingSink{"sinkA": sinkA, "sinkB": sinkB} {
		if len(sink.events) != 1 {
			t.Fatalf("%s: expected exactly one event, got %d", name, len(sink.events))
		}
		event := sink.events[0]
		if event.EventID == "" {
			t.Errorf("%s: EventID is empty, want a stamped UUID", name)
		}
		if event.Timestamp.IsZero() {
			t.Errorf("%s: Timestamp is zero, want stamped", name)
		}
		if event.Service != "test-service" {
			t.Errorf("%s: Service = %q, want test-service", name, event.Service)
		}
	}
}

// TestEmitter_StampsResourceTypeFromOperation guards the single-source-of-
// truth invariant for resource.type: it must come from Operation.ResourceType
// even when the Envelope carries no Resource at all (e.g. a denial before any
// handler ran) — never from a handler-supplied value, since Resource has no
// Type field to supply one.
func TestEmitter_StampsResourceTypeFromOperation(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sink := &recordingSink{}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "project", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultDenied})

	if len(sink.events) != 1 {
		t.Fatalf("expected exactly one event, got %d", len(sink.events))
	}
	if got := sink.events[0].ResourceType; got != "project" {
		t.Errorf("ResourceType = %q, want %q", got, "project")
	}
	if sink.events[0].Resource != nil {
		t.Errorf("Resource = %+v, want nil (Envelope carried none)", sink.events[0].Resource)
	}
}

// TestBuildEvent_FillsEmptyNamespaceFromHierarchy guards the fallback that
// lets a hierarchy captured in AuthzChecker.Check (e.g. CreateNamespace,
// whose REST path has no {namespaceName} to seed Resource.Namespace from)
// still produce a populated resource.namespace.
func TestBuildEvent_FillsEmptyNamespaceFromHierarchy(t *testing.T) {
	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "namespace", Category: CategoryManagement}
	resource := &Resource{Name: "ns-1"}
	env := Envelope{
		Origin: OriginAPI, Result: ResultSuccess,
		Resource: resource, Hierarchy: Hierarchy{Namespace: "ns-1"},
	}

	event := buildEvent(op, env, "test-service")

	if event.Resource == nil || event.Resource.Namespace != "ns-1" {
		t.Fatalf("Resource = %+v, want Namespace filled from Hierarchy", event.Resource)
	}
	if event.Resource == resource {
		t.Error("buildEvent must not mutate the Envelope's *Resource in place; it must return a copy")
	}
	if resource.Namespace != "" {
		t.Errorf("original Resource mutated: Namespace = %q, want empty", resource.Namespace)
	}
}

// TestBuildEvent_DoesNotOverrideExistingNamespace guards that the hierarchy
// fallback only fills a gap — a handler-supplied namespace (the existing,
// authoritative source per E.1) is never replaced.
func TestBuildEvent_DoesNotOverrideExistingNamespace(t *testing.T) {
	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "component", Category: CategoryManagement}
	env := Envelope{
		Origin: OriginAPI, Result: ResultSuccess,
		Resource: &Resource{Namespace: "handler-namespace"}, Hierarchy: Hierarchy{Namespace: "hierarchy-namespace"},
	}

	event := buildEvent(op, env, "test-service")

	if event.Resource.Namespace != "handler-namespace" {
		t.Errorf("Resource.Namespace = %q, want the handler-supplied value preserved", event.Resource.Namespace)
	}
}

// TestBuildEvent_CarriesHierarchyEvenWithNilResource guards a denial before
// any handler ran (Envelope.Resource nil), where the hierarchy is the only
// tenancy information available.
func TestBuildEvent_CarriesHierarchyEvenWithNilResource(t *testing.T) {
	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "component", Category: CategoryManagement}
	env := Envelope{
		Origin: OriginAPI, Result: ResultDenied,
		Hierarchy: Hierarchy{Namespace: "ns-1", Project: "p1", Component: "c1"},
	}

	event := buildEvent(op, env, "test-service")

	if event.Hierarchy != env.Hierarchy {
		t.Errorf("Hierarchy = %+v, want %+v", event.Hierarchy, env.Hierarchy)
	}
	if event.Resource == nil || event.Resource.Namespace != "ns-1" {
		t.Errorf("Resource = %+v, want a synthesized Resource carrying the hierarchy's namespace", event.Resource)
	}
}

func TestEmitter_SkipsAllSinksWhenPolicyDenies(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: false}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sink := &recordingSink{}
	emitter, err := NewEmitter("test-service", policies, sink)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	op := &Operation{ID: testProjectOpID, Action: testCreateProjectAction, ResourceType: "project", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultSuccess})

	if len(sink.events) != 0 {
		t.Errorf("expected no events when policy denies publish, got %d", len(sink.events))
	}
}
