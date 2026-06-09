// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schemaextract

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
)

// virtualProtoName is the in-memory file name used when compiling the inline
// proto source supplied in an endpoint schema.
const virtualProtoName = "endpoint.proto"

// protoExtractor extracts gRPC routes from protobuf source. It compiles the
// inline .proto content and emits one EndpointResource per (service, method).
//
// Only the single inline file plus protobuf well-known types are resolvable;
// user cross-file imports (e.g. import "common/types.proto") cannot be resolved
// and result in a compile error, which the caller degrades to catch-all routing.
type protoExtractor struct{}

func (protoExtractor) Extract(content string) ([]EndpointResource, error) {
	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(
			&protocompile.SourceResolver{
				Accessor: protocompile.SourceAccessorFromMap(map[string]string{
					virtualProtoName: content,
				}),
			},
		),
		// We only walk services/methods; no source location info is needed.
		SourceInfoMode: protocompile.SourceInfoNone,
	}

	files, err := compiler.Compile(context.Background(), virtualProtoName)
	if err != nil {
		return nil, fmt.Errorf("compile proto source: %w", err)
	}
	if len(files) == 0 {
		return empty(), nil
	}

	fd := files[0] // linker.File implements protoreflect.FileDescriptor
	var out []EndpointResource
	services := fd.Services()
	for i := 0; i < services.Len(); i++ {
		svc := services.Get(i)
		// FullName() is package-qualified (e.g. "greeter.Greeter"), matching what
		// the Gateway API GRPCRoute method.service field expects. A proto without a
		// package declaration yields a bare service name.
		serviceName := string(svc.FullName())
		methods := svc.Methods()
		for j := 0; j < methods.Len(); j++ {
			out = append(out, EndpointResource{
				Kind:    "gRPC",
				Service: serviceName,
				Method:  string(methods.Get(j).Name()),
			})
		}
	}
	return out, nil
}
