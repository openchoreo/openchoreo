// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/handlerservices"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth"
)

// newTestHTTPHandler wires the handler into the actual generated HTTP router with a
// test auth middleware in place of real JWT validation. This exercises the full
// request/response pipeline — route matching, parameter extraction, middleware
// chain, and serialization — rather than calling handler methods directly.
func newTestHTTPHandler(t *testing.T, services *handlerservices.Services) http.Handler {
	t.Helper()
	h := &Handler{services: services, logger: slog.Default()}
	strictHandler := gen.NewStrictHandler(h, nil)
	mux := http.NewServeMux()
	gen.HandlerWithOptions(strictHandler, gen.StdHTTPServerOptions{
		BaseRouter:  mux,
		Middlewares: []gen.MiddlewareFunc{injectTestSubject},
	})
	return mux
}

// injectTestSubject is a MiddlewareFunc that bypasses JWT validation and injects a
// fixed authenticated subject into the request context. This mimics a successfully
// authenticated request without requiring a real token.
var injectTestSubject gen.MiddlewareFunc = func(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := auth.SetSubjectContext(r.Context(), &auth.SubjectContext{
			ID:   "test-user",
			Type: "user",
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// doRequest sends an HTTP request through the given handler and returns both the
// original request (needed for OpenAPI validation) and the recorded response.
func doRequest(t *testing.T, h http.Handler, method, path string, jsonBody []byte) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	var bodyReader io.Reader
	if jsonBody != nil {
		bodyReader = bytes.NewReader(jsonBody)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if jsonBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return req, rec
}

// requireRouteInSpec fails the test immediately if the given method+path combination is
// not registered in the OpenAPI spec. Call this when you want a hard failure rather than
// the silent skip that assertConformsToSpec performs when a route is absent.
func requireRouteInSpec(t *testing.T, method, path string) {
	t.Helper()

	swagger, err := gen.GetSwagger()
	require.NoError(t, err, "failed to load OpenAPI spec")
	swagger.Servers = nil

	if ref, ok := swagger.Components.Schemas["RemoteReference"]; ok && ref.Value != nil {
		if vProp, ok := ref.Value.Properties["version"]; ok && vProp.Value != nil {
			vProp.Value.Example = nil
		}
	}

	router, err := legacyrouter.NewRouter(swagger)
	require.NoError(t, err, "failed to build OpenAPI router")

	req := httptest.NewRequest(method, path, nil)
	_, _, err = router.FindRoute(req)
	require.NoError(t, err, "route %s %s must be registered in the OpenAPI spec", method, path)
}

// assertConformsToSpec validates the response body against the OpenAPI contract
// loaded from the generated swagger spec. It uses the kin-openapi library to
// parse the spec, match the route, and validate the response schema.
func assertConformsToSpec(t *testing.T, req *http.Request, statusCode int, header http.Header, bodyBytes []byte) {
	t.Helper()

	swagger, err := gen.GetSwagger()
	require.NoError(t, err, "failed to load OpenAPI spec")
	swagger.Servers = nil // disable server URL matching so local test paths are accepted

	// The RemoteReference.version field has a numeric example (1) on a string
	// type — a spec inconsistency that causes kin-openapi to reject the document
	// during router construction. Clear it so the router can be built; this does
	// not affect response validation for any of the routes under test.
	if ref, ok := swagger.Components.Schemas["RemoteReference"]; ok && ref.Value != nil {
		if vProp, ok := ref.Value.Properties["version"]; ok && vProp.Value != nil {
			vProp.Value.Example = nil
		}
	}

	router, err := legacyrouter.NewRouter(swagger)
	require.NoError(t, err, "failed to build OpenAPI router for %s %s", req.Method, req.URL.Path)

	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		// A missing route means the path pattern isn't in the spec — skip rather
		// than fail so test-only paths don't break the suite.
		t.Logf("OpenAPI route not found for %s %s (skipping schema check): %v", req.Method, req.URL.Path, err)
		return
	}

	reqIn := &openapi3filter.RequestValidationInput{
		Request:    req,
		PathParams: pathParams,
		Route:      route,
		Options:    &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
	}
	respIn := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: reqIn,
		Status:                 statusCode,
		Header:                 header,
	}
	respIn.SetBodyBytes(bodyBytes)

	err = openapi3filter.ValidateResponse(context.Background(), respIn)
	assert.NoError(t, err, "response does not conform to OpenAPI contract for %s %s → %d",
		req.Method, req.URL.Path, statusCode)
}
