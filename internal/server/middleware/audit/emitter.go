// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Sink is an audit event destination. *Logger implements Sink today; more can
// be added later (webhook, queue, ...) without changing Emitter's API. A Sink
// must not mutate the *Event it's given — the same pointer is handed to every
// configured sink, so a mutation would leak into the others' view of it.
type Sink interface {
	LogEvent(event *Event)
}

// Emitter applies policy and emits. It knows nothing about either surface: an
// adapter resolves an Operation and builds an Envelope, and Emitter does the
// rest. Immutable after construction — see PolicySet's doc comment for why
// that matters under concurrent MCP tool calls.
type Emitter struct {
	serviceName string
	sinks       []Sink
	policies    *PolicySet
}

// NewEmitter creates an Emitter from an immutable policy set and one or more
// sinks. serviceName identifies the emitting process (e.g. "openchoreo-api")
// and is stamped onto every event by buildEvent, once, rather than by each
// sink — so multiple sinks can't disagree about one event's identity.
func NewEmitter(serviceName string, policies *PolicySet, sinks ...Sink) *Emitter {
	return &Emitter{serviceName: serviceName, sinks: append([]Sink(nil), sinks...), policies: policies}
}

// Emit builds the audit Event from op and env, resolves the publish setting
// (op may be nil — see ResolveContext), and skips emission if policy says not
// to publish. ctx is unused today; kept for signature symmetry with a future
// delivery/cancellation-aware sink.
func (e *Emitter) Emit(_ context.Context, op *Operation, env Envelope) {
	settings := e.policies.Resolve(ResolveContext{
		Operation: op,
		Actor:     env.Actor,
		Origin:    env.Origin,
		Result:    env.Result,
	})
	if !settings.Publish {
		return
	}

	event := buildEvent(op, env, e.serviceName)
	for _, sink := range e.sinks {
		sink.LogEvent(event)
	}
}

// buildEvent maps an Operation and Envelope into the Event every surface
// emits, stamping EventID, Timestamp, and Service exactly once here. Both
// REST and MCP build their own Envelope and hand it to Emit, which calls this
// same function, so the two surfaces can't drift into differently-shaped
// events.
func buildEvent(op *Operation, env Envelope, serviceName string) *Event {
	eventID, err := uuid.NewV7()
	eventIDStr := eventID.String()
	if err != nil {
		// Fallback to v4 if v7 generation fails.
		eventIDStr = uuid.New().String()
	}

	event := &Event{
		EventID:   eventIDStr,
		Timestamp: time.Now(),
		Service:   serviceName,
		Actor:     env.Actor,
		Result:    env.Result,
		Origin:    env.Origin,
		Resource:  env.Resource,
		RequestID: env.RequestID,
		SourceIP:  env.SourceIP,
		Metadata:  env.Metadata,
	}
	if op != nil {
		event.Action = op.Action
		event.Category = op.Category
		event.OperationID = op.ID
	}
	return event
}
