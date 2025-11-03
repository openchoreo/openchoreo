// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import "net/http"

// Middleware is a function that wraps an http.Handler
type Middleware func(http.Handler) http.Handler

// Chain applies multiple middleware functions in order.
// The first middleware in the slice is the outermost (executed first).
func Chain(middlewares ...Middleware) Middleware {
	return func(handler http.Handler) http.Handler {
		// Apply middlewares in reverse order so the first middleware
		// in the slice is the outermost
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
		return handler
	}
}

// RouteBuilder provides a fluent API for building routes with middleware chains
type RouteBuilder struct {
	mux         *http.ServeMux
	middlewares []Middleware
}

// NewRouteBuilder creates a new RouteBuilder with the given ServeMux
func NewRouteBuilder(mux *http.ServeMux) *RouteBuilder {
	return &RouteBuilder{
		mux:         mux,
		middlewares: make([]Middleware, 0),
	}
}

// With adds middlewares to the chain
func (rb *RouteBuilder) With(middlewares ...Middleware) *RouteBuilder {
	return &RouteBuilder{
		mux:         rb.mux,
		middlewares: append(rb.middlewares, middlewares...),
	}
}

// Handle registers a handler with the middleware chain
func (rb *RouteBuilder) Handle(pattern string, handler http.Handler) {
	if len(rb.middlewares) > 0 {
		handler = Chain(rb.middlewares...)(handler)
	}
	rb.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function with the middleware chain
func (rb *RouteBuilder) HandleFunc(pattern string, handlerFunc http.HandlerFunc) {
	rb.Handle(pattern, handlerFunc)
}

// Group creates a new RouteBuilder with additional middlewares
// This is useful for grouping routes with common middleware
func (rb *RouteBuilder) Group(middlewares ...Middleware) *RouteBuilder {
	return &RouteBuilder{
		mux:         rb.mux,
		middlewares: append(append([]Middleware{}, rb.middlewares...), middlewares...),
	}
}
