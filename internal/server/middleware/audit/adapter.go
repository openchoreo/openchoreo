// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/jwt"
)

// This file holds the helpers every surface adapter (REST's Middleware,
// MCP's audit middleware) calls to turn its own request shape into an
// Envelope. Nothing here is HTTP-handler- or MCP-SDK-shaped — http.Header is
// just a neutral header-value type — so an MCP-SDK-coupled package can depend
// on this file without pulling in anything REST-specific.

// DefaultActorIDClaim is the token claim recorded as actor.id by default.
const DefaultActorIDClaim = "sub"

// ExtractActor derives the audit Actor from the authenticated subject stored
// in ctx by the auth middleware. Shared by every surface adapter so
// actor-identity logic exists in exactly one place.
func ExtractActor(ctx context.Context, idClaim string) Actor {
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

	// "unknown" is reached only when even sub is absent. The "<nil>" check is
	// defense-in-depth: a fabricated actor identity in an audit trail is
	// undetectable downstream, so this guards against a future SubjectContext
	// constructor formatting a missing claim with fmt.Sprintf.
	actorID := "unknown"
	if id := actorIDFromContext(ctx, subjectCtx, idClaim); id != "" && id != "<nil>" {
		actorID = id
	}

	// SessionID is empty whenever the IdP issues no sid claim, which OIDC
	// leaves optional.
	actor := Actor{
		Type:      actorType,
		ID:        actorID,
		Issuer:    subjectCtx.Issuer,
		SessionID: subjectCtx.SessionID,
	}
	// Omit the entry entirely when there's no entitlement claim, rather than
	// recording an empty-keyed one that would log a spurious "entitlements":{"":null}.
	if subjectCtx.EntitlementClaim != "" {
		actor.Entitlements = map[string][]string{subjectCtx.EntitlementClaim: subjectCtx.EntitlementValues}
	}
	return actor
}

// The per-mechanism claim comes first because one global claim cannot identify
// every actor type: a claim naming a person is absent from the machine tokens a
// service account presents.
//
// The cost of falling through is that actor.id can be drawn from different
// claims on different records, with nothing in the envelope recording which, so
// policies[].match.actors selectors and --actor filters are reliable within an
// actor type but not necessarily across types.
//
// The JWT middleware is the only production writer of SubjectContext and
// stores the validated claims on the same ctx first, so wherever a subject is
// present its claims are too.
func actorIDFromContext(ctx context.Context, subjectCtx *auth.SubjectContext, idClaim string) string {
	if subjectCtx.ReadableID != "" {
		return subjectCtx.ReadableID
	}
	if idClaim != "" && idClaim != DefaultActorIDClaim {
		if claims, ok := jwt.GetClaimsFromContext(ctx); ok {
			if id, _ := claims[idClaim].(string); id != "" {
				return id
			}
		}
	}
	return subjectCtx.ID
}

// newUUID returns a UUID v7, falling back to v4 if v7 generation fails.
func newUUID() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.New().String()
}

// NewRequestInfo captures the facts an audit event needs from a request as it
// arrives. Pass nil for httpInfo on a surface with no request line.
func NewRequestInfo(httpInfo *HTTPInfo) RequestInfo {
	return RequestInfo{
		EventTime: time.Now(),
		HTTP:      httpInfo,
	}
}

// HTTPInfoFromRequest records r's request line: the decoded path, query string
// excluded.
//
// Decoded (URL.Path, not EscapedPath) so it agrees with r.PathValue, which the
// resource group is seeded from. Not length-capped: bounding client input
// belongs in validation or MaxHeaderBytes, not here.
func HTTPInfoFromRequest(r *http.Request) *HTTPInfo {
	return &HTTPInfo{
		Method: r.Method,
		Path:   r.URL.Path,
	}
}

// requestIDRejections counts inbound X-Request-ID headers rejected for not
// parsing as a UUID. An absent header isn't a rejection, so it doesn't
// increment this.
var requestIDRejections atomic.Int64

// RequestIDRejections returns the number of inbound X-Request-ID headers
// rejected so far because they didn't parse as a UUID.
func RequestIDRejections() int64 {
	return requestIDRejections.Load()
}

// RequestIDFromHeader returns the X-Request-ID header value if it parses as a
// UUID, generating a fresh UUID v7 otherwise (absent, malformed, or an
// arbitrary client-chosen string). Shared by every surface adapter.
//
// A client-chosen value reaches Event.RequestID verbatim otherwise, so
// without validation a client could inflate every audit record for its
// request with an oversized or arbitrary string. Requiring a valid UUID
// bounds it to a fixed shape.
//
// On REST, MCP, exec and wirelogs, logger.Middleware already runs this same
// validation against the inbound header and normalizes it before this ever
// executes, so here it's a no-op in the common case — this stays so the audit
// envelope is still well-formed if that ever changes.
func RequestIDFromHeader(h http.Header) string {
	requestID := h.Get("X-Request-ID")
	if requestID != "" {
		if _, err := uuid.Parse(requestID); err == nil {
			return requestID
		}
		requestIDRejections.Add(1)
	}
	return newUUID()
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
	ctx context.Context, emitter *Emitter, actorIDClaim string, op *Operation, surface Surface, result Result,
	auditData *AuditData, header http.Header, sourceIPFallback string,
) {
	sourceIP := SourceIPFromHeader(header)
	if sourceIP == "" {
		sourceIP = sourceIPFallback
	}
	env := Envelope{
		Surface:   surface,
		Actor:     ExtractActor(ctx, actorIDClaim),
		Result:    result,
		Resource:  auditData.Resource,
		Hierarchy: auditData.Hierarchy,
		Request:   auditData.Request,
		RequestID: RequestIDFromHeader(header),
		SourceIP:  sourceIP,
		UserAgent: header.Get("User-Agent"),
		Metadata:  auditData.Metadata,
	}
	emitter.Emit(ctx, op, env)
}
