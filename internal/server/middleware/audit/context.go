// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
)

// NewAuditContext returns a copy of ctx carrying a fresh audit data
// container pre-populated with resource, plus the container itself so the
// caller can read back whatever SetResource later wrote into it.
func NewAuditContext(ctx context.Context, resource *Resource) (context.Context, *AuditData) {
	data := &AuditData{Resource: resource}
	return context.WithValue(ctx, auditDataKey, data), data
}

// getAuditData retrieves or creates the audit data container from context
func getAuditData(ctx context.Context) *AuditData {
	if data, ok := ctx.Value(auditDataKey).(*AuditData); ok {
		return data
	}
	return nil
}

// SetResource stores resource information for audit logging. Handlers should
// call this once they know the resource's real identity (typically from the
// object a create/update returned).
//
// This replaces the whole *Resource; it does not merge fields into whatever
// was seeded before (e.g. NewAuditContext's placeholder, or an earlier
// SetResource call), so a call with a partially-populated Resource loses any
// field only a previous call had set. No current call site depends on
// merging, but a future one that means to add a single field to what's
// already there must read the existing Resource out of AuditData first,
// rather than constructing a new one.
func SetResource(ctx context.Context, resource *Resource) {
	if data := getAuditData(ctx); data != nil {
		data.Resource = resource
	}
}

// SetResult overrides the Result the REST middleware would otherwise derive
// from the response status code. For a handler that hijacks the connection
// (a WebSocket upgrade, an SSE stream taken over with http.Hijacker): once
// hijacked, nothing written to the raw connection touches the wrapped
// ResponseWriter's status code, so a failure after that point would
// otherwise audit identically to success. Call this immediately before
// returning on such a failure. No-op with no audit data in context.
func SetResult(ctx context.Context, result Result) {
	if data := getAuditData(ctx); data != nil {
		data.Result = &result
	}
}

// SeedHierarchy records a pre-call, claimed resource hierarchy — e.g. an MCP
// tool's raw call arguments, read before any authz check runs. It exists so
// MCP's filter-layer denials (which never reach AuthzChecker.Check) still
// carry a hierarchy in the audit record.
//
// Last write wins between this and SetHierarchy — see SetHierarchy's doc
// comment for why that resolves correctly in practice rather than needing an
// explicit precedence rule. No-op with no audit data in context.
func SeedHierarchy(ctx context.Context, h Hierarchy) {
	if data := getAuditData(ctx); data != nil {
		data.Hierarchy = h
	}
}

// SetHierarchy records the resource hierarchy an authorization decision was
// made on — called by AuthzChecker.Check, before the PDP evaluates, so it is
// recorded even on a denial or a PDP error.
//
// Last write wins, deliberately, with no guard against an earlier call. A
// single request can trigger more than one Check — e.g. a handler resolving
// a referenced object through another authz-wrapped service (itself doing
// its own Check) before running its own — and an earlier "first write wins"
// rule here permanently locked in whichever precondition check happened to
// run first, silently losing the actual operation's hierarchy.
//
// Last-wins is safe because of a structural fact, not an assumption: every
// *ServiceWithAuthz wrapper calls its own Check before delegating to the
// unwrapped implementation, so any precondition lookup's Check necessarily
// completes before the outer method reaches its own. That makes the last
// Check in a request reliably the one for the operation actually being
// audited (confirmed by tracing both REST and MCP requests).
//
// This depends on every wrapped service calling the unwrapped Service
// directly rather than another wrapper's method after its own Check — the
// convention component/service_authz.go documents. If that's ever violated,
// the later call's hierarchy would win instead, the same failure mode
// inverted, and worth revisiting if observed.
//
// No-op with no audit data in context — which is what keeps unaudited GETs
// and FilteredList's per-item checks (routed through BatchCheck, deliberately
// not hooked here) out of the audit record.
func SetHierarchy(ctx context.Context, h Hierarchy) {
	if data := getAuditData(ctx); data != nil {
		data.Hierarchy = h
	}
}

// emittedMarkerKey is the context key for emittedMarker.
type emittedMarkerKey struct{}

// emittedMarker is a mutable flag shared, via context, between an outer HTTP
// middleware (NewUnauthenticatedMiddleware) and an inner one (Middleware)
// composed beneath it, so the outer can tell whether the inner emitted an
// event.
//
// A plain re-read of r.Context() after next.ServeHTTP returns wouldn't work:
// every http.Request.WithContext call downstream (auth setting the subject,
// this package's own NewAuditContext) returns a new *http.Request that never
// propagates back to a variable the outer's closure already captured.
// Sharing a pointer sidesteps that — the outer seeds it before calling next,
// the inner flips it when it emits, and the outer reads that same pointer
// afterward.
type emittedMarker struct {
	emitted bool
}

// withEmittedMarker returns a copy of ctx carrying a fresh, unset
// emittedMarker, plus the marker itself.
func withEmittedMarker(ctx context.Context) (context.Context, *emittedMarker) {
	m := &emittedMarker{}
	return context.WithValue(ctx, emittedMarkerKey{}, m), m
}

// markEmitted flags ctx's emittedMarker, if one is present. A no-op when
// none was seeded — e.g. a Middleware used without an enclosing
// NewUnauthenticatedMiddleware instance, as exec/wirelogs are today.
func markEmitted(ctx context.Context) {
	if m, ok := ctx.Value(emittedMarkerKey{}).(*emittedMarker); ok {
		m.emitted = true
	}
}
