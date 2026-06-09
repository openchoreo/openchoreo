// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemaextract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

const sampleProto = `
syntax = "proto3";
package greeter;
service Greeter { rpc SayHello (Req) returns (Req); }
message Req { string name = 1; }
`

const sampleOpenAPI = `
openapi: 3.0.0
info: {title: t, version: "1.0"}
paths:
  /ping: {get: {}}
`

func TestExtract(t *testing.T) {
	tests := []struct {
		name    string
		epType  v1alpha1.EndpointType
		schema  *v1alpha1.Schema
		want    []EndpointResource
		wantErr bool
	}{
		{
			name:   "nil schema returns empty non-nil",
			epType: v1alpha1.EndpointTypeGRPC,
			schema: nil,
			want:   []EndpointResource{},
		},
		{
			name:   "blank content returns empty",
			epType: v1alpha1.EndpointTypeGRPC,
			schema: &v1alpha1.Schema{Type: "proto", Content: "   "},
			want:   []EndpointResource{},
		},
		{
			name:   "gRPC infers proto parser",
			epType: v1alpha1.EndpointTypeGRPC,
			schema: &v1alpha1.Schema{Content: sampleProto},
			want:   []EndpointResource{{Kind: "gRPC", Service: "greeter.Greeter", Method: "SayHello"}},
		},
		{
			name:   "HTTP infers openapi parser",
			epType: v1alpha1.EndpointTypeHTTP,
			schema: &v1alpha1.Schema{Content: sampleOpenAPI},
			want:   []EndpointResource{{Kind: "HTTP", Method: "GET", Path: "/ping"}},
		},
		{
			name:   "explicit Schema.Type override (protobuf) wins",
			epType: v1alpha1.EndpointTypeHTTP, // would infer openapi, but override forces proto
			schema: &v1alpha1.Schema{Type: "protobuf", Content: sampleProto},
			want:   []EndpointResource{{Kind: "gRPC", Service: "greeter.Greeter", Method: "SayHello"}},
		},
		{
			name:   "swagger normalizes to openapi",
			epType: v1alpha1.EndpointTypeHTTP,
			schema: &v1alpha1.Schema{Type: "swagger", Content: sampleOpenAPI},
			want:   []EndpointResource{{Kind: "HTTP", Method: "GET", Path: "/ping"}},
		},
		{
			name:   "TCP endpoint without override returns empty (no inference)",
			epType: v1alpha1.EndpointTypeTCP,
			schema: &v1alpha1.Schema{Content: sampleProto},
			want:   []EndpointResource{},
		},
		{
			name:    "deferred graphql type returns empty with warning",
			epType:  v1alpha1.EndpointTypeGraphQL,
			schema:  &v1alpha1.Schema{Content: "type Query { hello: String }"},
			want:    []EndpointResource{},
			wantErr: true,
		},
		{
			name:    "malformed proto returns empty with warning",
			epType:  v1alpha1.EndpointTypeGRPC,
			schema:  &v1alpha1.Schema{Content: "not a proto"},
			want:    []EndpointResource{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Extract(tt.epType, tt.schema)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			// Result is always non-nil so templates can map/size over it.
			assert.NotNil(t, got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeSchemaType(t *testing.T) {
	cases := map[string]struct {
		want SchemaType
		ok   bool
	}{
		"":         {"", false},
		"  ":       {"", false},
		"openapi":  {SchemaTypeOpenAPI, true},
		"OAS3":     {SchemaTypeOpenAPI, true},
		"Swagger":  {SchemaTypeOpenAPI, true},
		"proto":    {SchemaTypeProto, true},
		"protobuf": {SchemaTypeProto, true},
		"gRPC":     {SchemaTypeProto, true},
		"graphql":  {SchemaTypeGraphQL, true},
		"asyncapi": {SchemaTypeAsyncAPI, true},
		"openrpc":  {SchemaType("openrpc"), true}, // unknown kept as-is (fails lookup later)
	}
	for raw, exp := range cases {
		got, ok := normalizeSchemaType(raw)
		assert.Equal(t, exp.ok, ok, "raw=%q ok", raw)
		assert.Equal(t, exp.want, got, "raw=%q value", raw)
	}
}
