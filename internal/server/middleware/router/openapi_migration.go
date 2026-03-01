// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package router provides HTTP routing middleware for directing requests
// to different handlers based on request attributes.
package router

import (
	"net/http"
)

const (
	// HeaderUseLegacyRoutes is the header name used to opt-in to the legacy handler.
	// When set to "true", requests are routed to the legacy handler.
	// When absent or set to any other value, requests are routed to the OpenAPI handler.
	HeaderUseLegacyRoutes = "X-Use-Legacy-Routes"
)

// OpenAPIMigrationRouter returns an http.Handler that routes requests based on
// the X-Use-Legacy-Routes header during the migration from legacy handlers to
// OpenAPI-generated handlers.
//
// This middleware enables gradual migration by allowing clients to opt-in to the
// legacy API via a header. This approach:
//   - Routes all traffic to OpenAPI handlers by default
//   - Allows legacy clients to opt-in to old handlers during migration
//   - Requires no URL changes - same paths work with both handlers
//
// Routing logic:
//   - X-Use-Legacy-Routes: true → legacyHandler
//   - Header absent or other value → openapiHandler
//
// After migration is complete:
//  1. Remove this router and use openapiHandler directly
//  2. Clients remove the X-Use-Legacy-Routes header
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
		if r.Header.Get(HeaderUseLegacyRoutes) == "true" {
			legacyHandler.ServeHTTP(w, r)
			return
		}
		openapiHandler.ServeHTTP(w, r)
	})
}
