// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package schemaextract parses an endpoint's API schema (OpenAPI for HTTP,
// protobuf for gRPC) into a flat list of renderable routes. The result is
// injected into the template-rendering CEL context as
// workload.endpoints[name].resources so ComponentType templates can emit
// exact path/method route matches instead of catch-all routing.
//
// Extraction is best-effort: a missing, empty, or unparseable schema yields an
// empty (non-nil) result so templates can fall back to catch-all routing. The
// caller is expected to log returned errors as warnings, never to fail the render.
package schemaextract

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// EndpointResource is one renderable route extracted from an endpoint's API
// schema. JSON tags are lowercase so CEL templates see r.kind / r.service /
// r.method / r.path after the context is marshaled to a generic map.
type EndpointResource struct {
	// Kind is the protocol of the resource: "HTTP" or "gRPC".
	Kind string `json:"kind"`
	// Service is the fully-qualified gRPC service name (e.g. "greeter.Greeter").
	// Empty for HTTP resources.
	Service string `json:"service,omitempty"`
	// Method is the HTTP verb (GET/POST/...) or the gRPC method name (e.g. "SayHello").
	Method string `json:"method,omitempty"`
	// Path is the HTTP path template (e.g. "/v1/pets/{id}"). Empty for gRPC resources.
	Path string `json:"path,omitempty"`
}

// Extractor parses one schema content blob into resources. Implementations must
// be pure and side-effect free.
type Extractor interface {
	Extract(content string) ([]EndpointResource, error)
}

// SchemaType is a canonical schema-format identifier used to select an extractor.
type SchemaType string

const (
	SchemaTypeOpenAPI  SchemaType = "openapi"
	SchemaTypeProto    SchemaType = "proto"
	SchemaTypeGraphQL  SchemaType = "graphql"  // reserved; no extractor registered yet
	SchemaTypeAsyncAPI SchemaType = "asyncapi" // reserved; no extractor registered yet
)

// inferenceByEndpointType maps an endpoint protocol to its default schema format.
var inferenceByEndpointType = map[v1alpha1.EndpointType]SchemaType{
	v1alpha1.EndpointTypeHTTP:    SchemaTypeOpenAPI,
	v1alpha1.EndpointTypeGRPC:    SchemaTypeProto,
	v1alpha1.EndpointTypeGraphQL: SchemaTypeGraphQL,
}

// registry maps a canonical schema type to its extractor.
var registry = map[SchemaType]Extractor{
	SchemaTypeOpenAPI: openAPIExtractor{},
	SchemaTypeProto:   protoExtractor{},
}

// Extract returns the routes extracted from an endpoint's schema. It never fails
// fatally: a nil/blank schema, an unresolved type, an unregistered extractor, or
// a parse error all return a non-nil empty slice. A non-nil error is returned for
// the caller to log as a warning only (rendering must continue regardless).
//
// The schema is parsed on each call. The pipeline only invokes this once per
// render (and only when a template opts into the workload.toEndpointResources()
// macro), so no result memoization is kept here.
func Extract(epType v1alpha1.EndpointType, schema *v1alpha1.Schema) ([]EndpointResource, error) {
	if schema == nil || strings.TrimSpace(schema.Content) == "" {
		return empty(), nil
	}

	// Explicit override wins; otherwise infer from the endpoint protocol.
	st, ok := normalizeSchemaType(schema.Type)
	if !ok {
		st, ok = inferenceByEndpointType[epType]
		if !ok {
			// TCP/UDP/Websocket etc.: nothing to extract, not an error.
			return empty(), nil
		}
	}

	ex, ok := registry[st]
	if !ok {
		// e.g. an explicitly declared graphql/asyncapi schema (deferred): surface a
		// warning but degrade to catch-all.
		return empty(), fmt.Errorf("no extractor registered for schema type %q", st)
	}

	res, err := ex.Extract(schema.Content)
	if err != nil {
		return empty(), fmt.Errorf("extract %s schema: %w", st, err)
	}
	if res == nil {
		res = empty()
	}
	sortResources(res)
	return res, nil
}

// empty returns a non-nil empty slice so CEL always sees a list (never null),
// keeping endpoint.value.resources.map(...) / size(...) valid.
func empty() []EndpointResource { return []EndpointResource{} }

// normalizeSchemaType maps a free-form Schema.Type value to a canonical type.
// The second return is false when the value is empty (meaning "infer instead").
func normalizeSchemaType(raw string) (SchemaType, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return "", false
	case "openapi", "openapiv3", "oas", "oas3", "swagger":
		return SchemaTypeOpenAPI, true
	case "proto", "protobuf", "grpc":
		return SchemaTypeProto, true
	case "graphql", "gql":
		return SchemaTypeGraphQL, true
	case "asyncapi":
		return SchemaTypeAsyncAPI, true
	default:
		// Unknown explicit type: keep it as-is so it fails extractor lookup with a
		// clear warning rather than silently inferring from the endpoint type.
		return SchemaType(strings.ToLower(strings.TrimSpace(raw))), true
	}
}

// sortResources orders resources deterministically so rendered output is stable
// (OpenAPI map iteration and protoreflect traversal are otherwise unordered).
func sortResources(rs []EndpointResource) {
	sort.Slice(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Method < b.Method
	})
}
