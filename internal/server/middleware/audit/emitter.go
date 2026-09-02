// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"errors"
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
//
// policies must not be nil: Emit calls policies.Resolve on every event, so a
// nil PolicySet would return an error.
func NewEmitter(serviceName string, policies *PolicySet, sinks ...Sink) (*Emitter, error) {
	if policies == nil {
		return nil, errors.New("audit: NewEmitter requires a non-nil *PolicySet")
	}
	return &Emitter{serviceName: serviceName, sinks: append([]Sink(nil), sinks...), policies: policies}, nil
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
		Resource:  withHierarchyNamespaceFallback(env.Resource, env.Hierarchy),
		Hierarchy: env.Hierarchy,
		RequestID: env.RequestID,
		SourceIP:  env.SourceIP,
		Metadata:  env.Metadata,
	}
	if op != nil {
		event.Action = op.Action
		event.Category = op.Category
		event.OperationID = op.ID
		event.ResourceType = op.ResourceType
	}
	return event
}

// withHierarchyNamespaceFallback fills resource.Namespace from h.Namespace
// when resource carries none itself — e.g. CreateNamespace, whose REST path
// has no {namespaceName} to seed from, but whose authz check already knows
// ns.Name. Namespace's existing source (a handler's SetResource call, or the
// surface adapter's pre-call seed) stays authoritative: this only fills a gap,
// never overrides.
//
// Returns a shallow copy rather than mutating resource in place: resource is
// the same *Resource pointer AuditData holds for the lifetime of the request,
// so mutating it here would leak this fallback into whatever the caller
// (e.g. a later NewAuditContext caller reading it back) still holds.
func withHierarchyNamespaceFallback(resource *Resource, h Hierarchy) *Resource {
	if h.Namespace == "" {
		return resource
	}
	if resource == nil {
		return &Resource{Namespace: h.Namespace}
	}
	if resource.Namespace != "" {
		return resource
	}
	cp := *resource
	cp.Namespace = h.Namespace
	return &cp
}
