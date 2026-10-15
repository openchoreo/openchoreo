// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"unicode/utf8"
)

// NewAuditContext returns a copy of ctx carrying a fresh audit data
// container pre-populated with resource and req, plus the container itself so
// the caller can read back whatever SetResource later wrote into it.
//
// Call it at the adapter's entry, not at emit time — see RequestInfo.
func NewAuditContext(ctx context.Context, resource *Resource, req RequestInfo) (context.Context, *AuditData) {
	data := &AuditData{Resource: resource, Request: req}
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
		data.mu.Lock()
		defer data.mu.Unlock()
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
		data.mu.Lock()
		defer data.mu.Unlock()
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
		data.mu.Lock()
		defer data.mu.Unlock()
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
// No-op with no audit data in context, which keeps unaudited GETs out of the
// audit record. An audited list is the exception to last-wins: FilteredList
// checks each item, so its wrapper records the list's scope afterwards.
func SetHierarchy(ctx context.Context, h Hierarchy) {
	if data := getAuditData(ctx); data != nil {
		data.mu.Lock()
		defer data.mu.Unlock()
		data.Hierarchy = h
	}
}

// SetRequest records the operation's request parameters. Call it from the
// operation's authz wrapper before the authz check, so a denial still records
// what was asked for.
//
// params is a reviewed allowlist under the API's own parameter names, never
// a dump of the query string or body. Leave out path parameters that
// http.path and resource already carry. Every string, at any depth, is cut to
// MaxParameterLength runes.
//
// Last write wins, so this relies on the same convention as SetHierarchy: a
// wrapped service never calls another wrapper, or that wrapper's capture
// would be published on this operation's record. No-op with no audit data in
// context.
func SetRequest(ctx context.Context, params map[string]any) {
	data := getAuditData(ctx)
	if data == nil {
		return
	}
	captured := capParameters(params)
	data.mu.Lock()
	defer data.mu.Unlock()
	data.Parameters = captured
}

// SetReturned records what the operation handed back. Call it after the
// inner call succeeds; buildEvent still drops it on any other result. Items
// beyond MaxReturnedItems are dropped and Count is left as given. Same
// last-write rule as SetRequest.
func SetReturned(ctx context.Context, returned Returned) {
	data := getAuditData(ctx)
	if data == nil {
		return
	}
	returned.Items = append([]ReturnedItem(nil), returned.Items[:min(len(returned.Items), MaxReturnedItems)]...)
	data.mu.Lock()
	defer data.mu.Unlock()
	data.Returned = &returned
}

// capParameters copies params, so the caller's map can't change the record
// after the call.
func capParameters(params map[string]any) map[string]any {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = capValue(v)
	}
	return out
}

func capValue(v any) any {
	switch val := v.(type) {
	case string:
		return capString(val)
	case []string:
		capped := make([]string, len(val))
		for i, s := range val {
			capped[i] = capString(s)
		}
		return capped
	case []any:
		capped := make([]any, len(val))
		for i, e := range val {
			capped[i] = capValue(e)
		}
		return capped
	case map[string]any:
		return capParameters(val)
	default:
		return v
	}
}

func capString(s string) string {
	if utf8.RuneCountInString(s) <= MaxParameterLength {
		return s
	}
	return string([]rune(s)[:MaxParameterLength])
}
