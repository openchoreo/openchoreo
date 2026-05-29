// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemaextract

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProtoExtractor(t *testing.T) {
	greeter := `
syntax = "proto3";
package greeter;
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
  rpc SayGoodbye (HelloRequest) returns (HelloReply);
}
message HelloRequest { string name = 1; }
message HelloReply  { string message = 1; }
`
	multiService := `
syntax = "proto3";
package shop;
service Catalog { rpc List (Empty) returns (Empty); }
service Cart    { rpc Add (Empty) returns (Empty); }
message Empty {}
`
	noPackage := `
syntax = "proto3";
service Bare { rpc Ping (Msg) returns (Msg); }
message Msg { string v = 1; }
`
	wktImport := `
syntax = "proto3";
package wkt;
import "google/protobuf/empty.proto";
service Svc { rpc Do (google.protobuf.Empty) returns (google.protobuf.Empty); }
`
	userImport := `
syntax = "proto3";
package u;
import "common/types.proto";
service Svc { rpc Do (common.T) returns (common.T); }
`

	tests := []struct {
		name    string
		content string
		want    []EndpointResource
		wantErr bool
	}{
		{
			name:    "single service multiple methods",
			content: greeter,
			want: []EndpointResource{
				{Kind: "gRPC", Service: "greeter.Greeter", Method: "SayGoodbye"},
				{Kind: "gRPC", Service: "greeter.Greeter", Method: "SayHello"},
			},
		},
		{
			name:    "multiple services",
			content: multiService,
			want: []EndpointResource{
				{Kind: "gRPC", Service: "shop.Cart", Method: "Add"},
				{Kind: "gRPC", Service: "shop.Catalog", Method: "List"},
			},
		},
		{
			name:    "no package yields bare service name",
			content: noPackage,
			want:    []EndpointResource{{Kind: "gRPC", Service: "Bare", Method: "Ping"}},
		},
		{
			name:    "well-known-type import resolves",
			content: wktImport,
			want:    []EndpointResource{{Kind: "gRPC", Service: "wkt.Svc", Method: "Do"}},
		},
		{
			name:    "user cross-file import fails (documented limitation)",
			content: userImport,
			wantErr: true,
		},
		{
			name:    "broken proto errors",
			content: "this is not proto",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := protoExtractor{}.Extract(tt.content)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			sortResources(got)
			assert.Equal(t, tt.want, got)
		})
	}
}
