// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// recordingSink is a test double that records every Event it receives.
type recordingSink struct {
	events []*Event
}

func (s *recordingSink) LogEvent(event *Event) {
	s.events = append(s.events, event)
}

func TestEmitter_FansOutToEverySink(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sinkA := &recordingSink{}
	sinkB := &recordingSink{}
	emitter := NewEmitter("test-service", policies, sinkA, sinkB)

	op := &Operation{ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultSuccess})

	if len(sinkA.events) != 1 || len(sinkB.events) != 1 {
		t.Fatalf("expected exactly one event per sink, got sinkA=%d sinkB=%d", len(sinkA.events), len(sinkB.events))
	}
	if sinkA.events[0].Action != "create_project" || sinkB.events[0].Action != "create_project" {
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
	emitter := NewEmitter("test-service", policies, sinkA, sinkB)

	op := &Operation{ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: CategoryManagement}
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

func TestEmitter_SkipsAllSinksWhenPolicyDenies(t *testing.T) {
	policies, errs := NewPolicySet(coreconfig.NewPath("audit"), Settings{Publish: false}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	sink := &recordingSink{}
	emitter := NewEmitter("test-service", policies, sink)

	op := &Operation{ID: "CreateProject", Action: "create_project", ResourceType: "projects", Category: CategoryManagement}
	emitter.Emit(context.Background(), op, Envelope{Origin: OriginAPI, Result: ResultSuccess})

	if len(sink.events) != 0 {
		t.Errorf("expected no events when policy denies publish, got %d", len(sink.events))
	}
}
