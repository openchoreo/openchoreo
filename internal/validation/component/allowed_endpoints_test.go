// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateAllowedEndpoints(t *testing.T) {
	http := v1alpha1.EndpointProtocolHTTP
	grpc := v1alpha1.EndpointProtocolGRPC
	graphql := v1alpha1.EndpointProtocolGraphQL

	entry := func(name string, protocols []v1alpha1.EndpointProtocol, def bool) v1alpha1.AllowedEndpointType {
		return v1alpha1.AllowedEndpointType{
			Name:      name,
			Protocols: protocols,
			Ref:       v1alpha1.EndpointTypeRef{Kind: v1alpha1.EndpointTypeRefKindClusterEndpointType, Name: "http-route"},
			Default:   def,
		}
	}

	tests := []struct {
		name    string
		entries []v1alpha1.AllowedEndpointType
		errs    int
	}{
		{name: "empty", entries: nil},
		{
			name: "one default per protocol",
			entries: []v1alpha1.AllowedEndpointType{
				entry("", []v1alpha1.EndpointProtocol{http, graphql}, true),
				entry("", []v1alpha1.EndpointProtocol{grpc}, true),
			},
		},
		{
			name: "non-default entries may overlap a default",
			entries: []v1alpha1.AllowedEndpointType{
				entry("web", []v1alpha1.EndpointProtocol{http}, true),
				entry("managed-api", []v1alpha1.EndpointProtocol{http}, false),
			},
		},
		{
			name: "two defaults for the same protocol",
			entries: []v1alpha1.AllowedEndpointType{
				entry("", []v1alpha1.EndpointProtocol{http, grpc}, true),
				entry("", []v1alpha1.EndpointProtocol{http}, true),
			},
			errs: 1,
		},
		{
			name: "duplicate alias",
			entries: []v1alpha1.AllowedEndpointType{
				entry("web", []v1alpha1.EndpointProtocol{http}, false),
				entry("web", []v1alpha1.EndpointProtocol{grpc}, false),
			},
			errs: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateAllowedEndpoints(tt.entries, field.NewPath("spec", "allowedEndpoints"))
			if len(errs) != tt.errs {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.errs, errs)
			}
		})
	}
}
