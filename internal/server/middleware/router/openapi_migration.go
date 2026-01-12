// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package router provides HTTP routing middleware for directing requests
// to different handlers based on request attributes.
package router

import (
	"net/http"
)

const (
	// HeaderUseOpenAPI is the header name used to opt-in to the OpenAPI handler.
	// When set to "true", requests are routed to the OpenAPI-generated handler.
	// When absent or set to any other value, requests are routed to the legacy handler.
	HeaderUseOpenAPI = "X-Use-OpenAPI"
)

// OpenAPIMigrationRouter returns an http.Handler that routes requests based on
// the X-Use-OpenAPI header during the migration from legacy handlers to
// OpenAPI-generated handlers.
//
// This middleware enables gradual migration by allowing clients to opt-in to the
// new API via a header. This approach:
//   - Allows new features to use spec-first development immediately
//   - Enables incremental migration of existing endpoints
//   - Requires no URL changes - same paths work with both handlers
//   - Supports generated clients that automatically include the header
//
// Routing logic:
//   - X-Use-OpenAPI: true → openapiHandler
//   - Header absent or other value → legacyHandler
//
// After migration is complete:
//  1. Remove this router and use openapiHandler directly
//  2. Clients remove the X-Use-OpenAPI header
//  3. Delete legacy handlers
//
// Example usage:
//
//	legacyHandler := legacyhandlers.New(services, cfg, logger).Routes()
//	openapiHandler := gen.HandlerWithOptions(strictHandler, options)
//
//	handler := router.OpenAPIMigrationRouter(openapiHandler, legacyHandler)
//	server := &http.Server{Handler: handler}
func OpenAPIMigrationRouter(openapiHandler, legacyHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderUseOpenAPI) == "true" {
			openapiHandler.ServeHTTP(w, r)
			return
		}
		legacyHandler.ServeHTTP(w, r)
	})
}
