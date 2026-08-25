// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"net/http"
)

// NewUnauthenticatedMiddleware returns the outer HTTP audit adapter. It must
// wrap the auth middleware, not sit inside it: auth short-circuits a
// rejected request with its own response and never calls next, so the inner
// pattern-map-driven Middleware never runs on an auth failure. This is the
// only place that gap can be closed.
//
// It emits exactly one event, with a nil Operation and Result:
// ResultUnauthenticated, when the response status is 401 and the inner
// Middleware did not already emit for this request. Every other outcome
// emits nothing: a 403 PDP denial, or a 401 the inner Middleware itself
// recorded against a resolved operation, are both events the inner instance
// already owns.
//
// Mutual exclusivity is tracked via emittedMarker rather than by checking
// auth.GetSubjectContextFromContext(r.Context()) after next.ServeHTTP
// returns: that read would always see no subject regardless of whether auth
// succeeded, since auth (and the inner Middleware) attach the subject to a
// *derived* request via r.WithContext, and that derived request never
// propagates back to the r this closure captured. See emittedMarker's doc
// comment.
//
// A panic is audited the same way a status code is classified — as a
// failure, but only when the inner Middleware hasn't already recorded one;
// double-emitting the same panic on both would be worse than the asymmetry
// it would otherwise leave (a panic in auth itself, or between the two
// middlewares, going unaudited).
func NewUnauthenticatedMiddleware(emitter *Emitter, enabled bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !enabled {
				next.ServeHTTP(w, r)
				return
			}

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			ctx, marker := withEmittedMarker(r.Context())
			r = r.WithContext(ctx)

			defer func() {
				p := recover()
				if marker.emitted {
					if p != nil {
						panic(p)
					}
					return
				}

				result := ResultUnauthenticated
				if p != nil {
					result = ResultFailure
				} else if rw.statusCode != http.StatusUnauthorized {
					return
				}

				_, auditData := NewAuditContext(r.Context(), nil)
				EmitFromContext(r.Context(), emitter, nil, OriginAPI, result, auditData, r.Header, r.RemoteAddr)

				if p != nil {
					panic(p)
				}
			}()

			next.ServeHTTP(rw, r)
		})
	}
}
