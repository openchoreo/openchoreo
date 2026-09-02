// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
)

// Middleware handles audit logging for HTTP requests. It is service-agnostic:
// patternMap is built from the caller's own Operations and its own OpenAPI
// spec (see BuildPatternMap), so any REST service can construct one of these
// from its own data. openchoreo-api is the only one that does today —
// observer has its own unrelated NewHTTPServer and no audit middleware.
type Middleware struct {
	logger     *slog.Logger // pre-flight "should never happen" logging only, see Handler
	patternMap map[string]*Operation
	emitter    *Emitter
	enabled    bool
}

// NewMiddleware builds the pattern map from ops and the caller's OpenAPI spec
// (getSwagger — pass gen.GetSwagger directly, the signature matches), then
// constructs the Middleware. One call so each REST service doesn't repeat
// the fetch-spec/cross-reference/wire sequence itself.
//
// enabled controls whether the middleware emits events; it stays in the
// request chain unconditionally so no configuration path can remove audit
// coverage without a code change.
func NewMiddleware(
	logger *slog.Logger, ops []Operation, getSwagger func() (*openapi3.T, error), emitter *Emitter, enabled bool,
) (*Middleware, error) {
	swagger, err := getSwagger()
	if err != nil {
		return nil, fmt.Errorf("audit: failed to load OpenAPI spec: %w", err)
	}
	patternMap, err := BuildPatternMap(ops, swagger)
	if err != nil {
		return nil, err
	}
	return newMiddleware(logger, patternMap, emitter, enabled), nil
}

// newMiddleware constructs a Middleware from an already-built pattern map.
// Exported callers go through NewMiddleware; this stays unexported so tests
// in this package can construct one directly from a hand-built patternMap
// without needing a real OpenAPI spec.
func newMiddleware(logger *slog.Logger, patternMap map[string]*Operation, emitter *Emitter, enabled bool) *Middleware {
	return &Middleware{
		logger:     logger,
		patternMap: patternMap,
		emitter:    emitter,
		enabled:    enabled,
	}
}

// NewMiddlewareForRoutes builds a Middleware from a hand-declared route ->
// Operation map, bypassing BuildPatternMap's OpenAPI-spec cross-referencing
// entirely. For routes registered outside the OpenAPI-generated mux, which
// have no operationId to resolve against a spec but whose Go 1.22+ ServeMux
// registration pattern is known and fixed at wiring time.
//
// Each key must be the exact string net/http.Request.Pattern reports once
// routed for that route (e.g. "GET /api/v1/namespaces/{namespace}/environments/{environment}/wirelogs",
// or a bare subtree prefix like "/exec/" for a pattern registered without a
// method).
func NewMiddlewareForRoutes(logger *slog.Logger, patternMap map[string]*Operation, emitter *Emitter, enabled bool) *Middleware {
	return newMiddleware(logger, patternMap, emitter, enabled)
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter to http.ResponseController, so
// a handler behind this middleware can still reach optional interfaces
// (Flusher, Hijacker, the deadline setters) implemented by the real
// ResponseWriter but not by this wrapper.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// Handler returns the HTTP middleware handler
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}

		if r.Pattern == "" {
			// Should be unreachable when BaseRouter is a Go 1.22+ ServeMux —
			// routing always runs before this middleware. But BaseRouter is
			// declared as an interface (gen.StdHTTPServerOptions.BaseRouter),
			// so a caller could wire in a router that never sets r.Pattern; this
			// fires for every one of the 102+ unaudited routes too, not just
			// audited ones. Panicking here would turn a dropped-connection-style
			// router mismatch into a 500 on the entire API. Log loudly and pass
			// through instead — construction time (BuildPatternMap) is where an
			// audit-config problem should fail loudly; this is a request-time
			// signal, not a place to sever the response.
			m.logger.Error("audit: request has no matched route pattern (r.Pattern is empty); "+
				"skipping audit for this request", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		op, ok := m.patternMap[r.Pattern]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		// Seed a placeholder resource (path-derived namespace and name, where
		// the operation has a path parameter) before calling next: a PDP
		// denial raised inside the handler never reaches the SetResource call
		// that would otherwise set it, so this is the only source of
		// resource.namespace/name on a denied or failed request. Every
		// OpenAPI-spec-derived route is nested under
		// /namespaces/{namespaceName}/..., so the path parameter name is
		// fixed across all of them — but a route registered outside the spec
		// (NotInOpenAPISpec, e.g. wirelogs' {namespace}) can use a different
		// parameter name, in which case this seed's namespace comes back
		// empty and the handler must call SetResource itself with the real
		// value before its own first exit (see wirelogs.go). resource.type
		// needs no seed here — it is stamped from op.ResourceType at emit
		// time regardless of whether a resource was ever set (see
		// buildEvent). Mirrors the MCP adapter's pre-call seed (see
		// mcpaudit.newAuditMiddleware).
		ctx, auditData := NewAuditContext(r.Context(), &Resource{
			Namespace: r.PathValue("namespaceName"),
			Name:      r.PathValue(op.RESTResourceParam),
		})

		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			written:        false,
		}

		// A panic in next must still produce an audit record — recover just
		// long enough to emit as a failure, then re-panic so the response
		// behavior above is unchanged.
		defer func() {
			if p := recover(); p != nil {
				markEmitted(ctx)
				EmitFromContext(ctx, m.emitter, op, OriginAPI, ResultFailure, auditData, r.Header, r.RemoteAddr)
				panic(p)
			}
			// A hijacking handler (e.g. exec's WebSocket upgrade) can call
			// SetResult once the response status code stops reflecting the
			// real outcome — see AuditData.Result's doc comment. Anything
			// that didn't hijack leaves this nil and falls through to the
			// ordinary status-code classification.
			result := determineResult(rw.statusCode)
			if auditData.Result != nil {
				result = *auditData.Result
			}
			markEmitted(ctx)
			EmitFromContext(ctx, m.emitter, op, OriginAPI, result, auditData, r.Header, r.RemoteAddr)
		}()

		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// determineResult maps HTTP status code to audit result
func determineResult(statusCode int) Result {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return ResultSuccess
	case statusCode == 401:
		return ResultUnauthenticated
	case statusCode == 403:
		return ResultDenied
	default:
		return ResultFailure
	}
}
