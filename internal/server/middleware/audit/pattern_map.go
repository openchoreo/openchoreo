// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// BuildPatternMap cross-references each Operation's ID (the OpenAPI
// operationId) against a parsed OpenAPI spec to find its registered
// method+path, formatted exactly as net/http.Request.Pattern reports it once
// routed: "METHOD "+path. swagger is the caller's own generated spec (e.g.
// gen.GetSwagger()), so any REST service using oapi-codegen can reuse this
// with its own Operations and spec.
//
// Assumes the caller's generated handler is never built with a non-empty
// BaseURL — true everywhere today. If that changes, pattern strings won't
// match r.Pattern and audit silently stops for that service (not a wrong
// record) until a baseURL parameter is added back here.
//
// An operation with NotInOpenAPISpec set is skipped entirely — its route
// isn't in the spec to look up, by design, and its caller owns its own
// pattern-map entry. Every other operation is cross-referenced: returns an
// error if its operationId has no match in the spec, if two operations
// collide on the same pattern, or if its RESTResourceParam names a path
// parameter the resolved pattern doesn't have.
func BuildPatternMap(ops []Operation, swagger *openapi3.T) (map[string]*Operation, error) {
	byOperationID := make(map[string]string, len(swagger.Paths.InMatchingOrder())*4)
	for _, path := range swagger.Paths.InMatchingOrder() {
		item := swagger.Paths.Find(path)
		for method, op := range item.Operations() {
			if op.OperationID == "" {
				continue
			}
			byOperationID[op.OperationID] = method + " " + path
		}
	}

	result := make(map[string]*Operation, len(ops))
	for i := range ops {
		op := &ops[i]
		if op.NotInOpenAPISpec {
			continue
		}
		pattern, ok := byOperationID[op.ID]
		if !ok {
			return nil, fmt.Errorf("audit: operationId %q has no matching route in the OpenAPI spec", op.ID)
		}
		if existing, collides := result[pattern]; collides {
			return nil, fmt.Errorf("audit: operations %q and %q both resolve to pattern %q",
				existing.ID, op.ID, pattern)
		}
		// RESTResourceParam must name an actual {wildcard} in the resolved
		// pattern — r.PathValue(op.RESTResourceParam) silently returns "" for
		// a name the route doesn't have, so a typo here would surface as a
		// silently empty resource.name on exactly the denied/failed events
		// the middleware seeds it for, not a construction-time error. Empty
		// RESTResourceParam (a Create route with no path parameter) is valid
		// and skips this check.
		if op.RESTResourceParam != "" && !strings.Contains(pattern, "{"+op.RESTResourceParam+"}") {
			return nil, fmt.Errorf("audit: operation %q declares RESTResourceParam %q, "+
				"but pattern %q has no such path parameter", op.ID, op.RESTResourceParam, pattern)
		}
		result[pattern] = op
	}
	return result, nil
}
