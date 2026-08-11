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
// from its own data — openchoreo-api and observer both use it.
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

// Handler returns the HTTP middleware handler
func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.enabled {
			next.ServeHTTP(w, r)
			return
		}

		if r.Pattern == "" {
			// Should be unreachable — routing always runs before this
			// middleware. If it does fire, panic loudly rather than silently
			// skip auditing; there's no recover() in this chain, so net/http
			// aborts just this one response, not the process.
			m.logger.Error("audit: request has no matched route pattern (r.Pattern is empty)",
				"method", r.Method, "path", r.URL.Path)
			panic("audit: request has no matched route pattern (r.Pattern is empty)")
		}

		op, ok := m.patternMap[r.Pattern]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}

		ctx, auditData := NewAuditContext(r.Context(), nil)

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
				EmitFromContext(ctx, m.emitter, op, OriginAPI, ResultFailure, auditData, r.Header, r.RemoteAddr)
				panic(p)
			}
			result := determineResult(rw.statusCode)
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
	case statusCode == 401 || statusCode == 403:
		return ResultDenied
	default:
		return ResultFailure
	}
}
