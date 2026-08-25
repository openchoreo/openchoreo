// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// This file holds the helpers every surface adapter (REST's Middleware,
// MCP's audit middleware) calls to turn its own request shape into an
// Envelope. Nothing here is HTTP-handler- or MCP-SDK-shaped — http.Header is
// just a neutral header-value type — so an MCP-SDK-coupled package can depend
// on this file without pulling in anything REST-specific.

// ExtractActor derives the audit Actor from the authenticated subject stored
// in ctx by the auth middleware. Shared by every surface adapter so
// actor-identity logic exists in exactly one place.
func ExtractActor(ctx context.Context) Actor {
	subjectCtx, ok := auth.GetSubjectContextFromContext(ctx)
	if !ok || subjectCtx == nil {
		return Actor{
			Type: "anonymous",
			ID:   "anonymous",
		}
	}

	actorType := subjectCtx.Type
	if actorType == "" {
		actorType = "user"
	}

	// Identity is the token's validated sub claim. An absent sub falls back
	// to "unknown" rather than being recorded as a real identity. The "<nil>"
	// check is defense-in-depth: jwt/resolver.go (the only production
	// constructor of SubjectContext today) already reads sub explicitly
	// rather than through fmt.Sprintf, so it never produces the literal
	// string "<nil>" — but a fabricated actor identity in an audit trail is
	// undetectable downstream, so this stays belt-and-braces against some
	// other future constructor reintroducing that failure mode.
	actorID := "unknown"
	if subjectCtx.ID != "" && subjectCtx.ID != "<nil>" {
		actorID = subjectCtx.ID
	}

	actor := Actor{Type: actorType, ID: actorID}
	// Omit the entry entirely when there's no entitlement claim, rather than
	// recording an empty-keyed one that would log a spurious "entitlements":{"":null}.
	if subjectCtx.EntitlementClaim != "" {
		actor.Entitlements = map[string][]string{subjectCtx.EntitlementClaim: subjectCtx.EntitlementValues}
	}
	return actor
}

// maxRequestIDLen bounds the client-supplied X-Request-ID recorded in audit
// events. Without a cap, a client can send an oversized header value (up to
// the server's MaxHeaderBytes limit) and inflate every audit record — and
// every sink — for that request. 128 comfortably fits a UUID (36 chars) or a
// typical trace ID with room for prefixes.
const maxRequestIDLen = 128

// RequestIDFromHeader returns the X-Request-ID header value, generating a
// fresh UUID v7 if absent or too long. Shared by every surface adapter.
func RequestIDFromHeader(h http.Header) string {
	requestID := h.Get("X-Request-ID")
	if requestID == "" || len(requestID) > maxRequestIDLen {
		if id, err := uuid.NewV7(); err == nil {
			requestID = id.String()
		} else {
			requestID = uuid.New().String() // fallback if v7 generation fails
		}
	}
	return requestID
}

// SourceIPFromHeader extracts the client IP from proxy headers
// (X-Forwarded-For, X-Real-IP). Returns "" if neither is present — a caller
// with a more specific fallback (e.g. REST's r.RemoteAddr) should apply it.
//
// source_ip is a hint, not forensic evidence. X-Forwarded-For is read
// leftmost-first, which is the client-supplied end of the chain, so a client
// can choose its own recorded source_ip. A proxy that appends — the common
// case, including the gateway shipped with OpenChoreo — does not prevent
// that; only one that strips or overwrites both headers does.
//
// Left this way deliberately. Reading the chain right-to-left would instead
// yield the address the edge proxy actually observed, but only given a
// trusted-hop count, which is a property of each deployment rather than of
// this code, and a count set one too high restores the spoof. Actor identity
// comes from a validated JWT sub, so nothing forensically load-bearing rests
// on source_ip today. Revisit if something downstream starts needing it to
// be trustworthy; the fix then spans this function and the gateway's
// xffNumTrustedHops together, not this function alone.
func SourceIPFromHeader(h http.Header) string {
	if xff := h.Get("X-Forwarded-For"); xff != "" {
		if first, _, found := strings.Cut(xff, ","); found {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(xff)
	}

	if xri := h.Get("X-Real-IP"); xri != "" {
		return xri
	}

	return ""
}

// EmitFromContext assembles the Envelope both surface adapters emit from and
// calls emitter.Emit. Resource/Metadata come from auditData, the container
// NewAuditContext created and a handler may have mutated via SetResource —
// reading both off one struct here keeps REST and MCP from building the
// Envelope differently. sourceIPFallback applies only when the header carries
// no IP hint — REST passes r.RemoteAddr, MCP passes "".
func EmitFromContext(
	ctx context.Context, emitter *Emitter, op *Operation, origin Origin, result Result,
	auditData *AuditData, header http.Header, sourceIPFallback string,
) {
	sourceIP := SourceIPFromHeader(header)
	if sourceIP == "" {
		sourceIP = sourceIPFallback
	}
	env := Envelope{
		Origin:    origin,
		Actor:     ExtractActor(ctx),
		Result:    result,
		Resource:  auditData.Resource,
		RequestID: RequestIDFromHeader(header),
		SourceIP:  sourceIP,
		Metadata:  auditData.Metadata,
	}
	emitter.Emit(ctx, op, env)
}
