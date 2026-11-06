// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// ExtendWriteDeadline gives requests to one route a longer write deadline than
// the server's WriteTimeout, leaving every other route on the server default.
//
// It must wrap the server's handler directly: http.ResponseController reaches
// the connection through the writer it is given, and a wrapping writer that
// does not unwrap would hide it.
func ExtendWriteDeadline(
	method, path string, timeout time.Duration, logger *slog.Logger,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == method && r.URL.Path == path {
				if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout)); err != nil {
					logger.Warn("Failed to extend the write deadline", "path", path, "error", err)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
