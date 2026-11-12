// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateEndpointTypeResourcesWithSchema(t *testing.T) {
	route := func(name string) *runtime.RawExtension {
		return &runtime.RawExtension{Raw: []byte(
			`{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"` + name + `"}}`)}
	}

	tests := []struct {
		name        string
		resources   []v1alpha1.ResourceTemplate
		validations []v1alpha1.ValidationRule
		wantErr     string // empty means valid; otherwise a substring of the aggregated error
	}{
		{
			name: "endpoint context variables type-check",
			resources: []v1alpha1.ResourceTemplate{{
				ID:       "route",
				Template: route(`${metadata.name + '-' + metadata.endpointName + '-' + metadata.environmentName}`),
			}},
		},
		{
			name: "backends and visibility are available",
			resources: []v1alpha1.ResourceTemplate{{
				ID:          "route",
				IncludeWhen: `${"external" in visibility && size(backends) > 0}`,
				Template:    route(`${backends[0].serviceName}`),
			}},
		},
		{
			name: "the network policy ID prefix is reserved",
			resources: []v1alpha1.ResourceTemplate{{
				ID: "openchoreo-networkpolicy-stable", Template: route("x"),
			}},
			wantErr: `spec.resources[0].id: Invalid value: "openchoreo-networkpolicy-stable"`,
		},
		{
			name: "component-only helpers are rejected",
			resources: []v1alpha1.ResourceTemplate{{
				ID: "route", Template: route(`${workload.name}`),
			}},
			wantErr: "workload",
		},
		{
			name: "forEach loop variable is scoped to its template",
			resources: []v1alpha1.ResourceTemplate{{
				ID: "svc", ForEach: "${backends}", Var: "backend",
				Template: route(`${metadata.name + '-' + backend.name}`),
			}},
		},
		{
			name: "forEach loop variable is typed",
			resources: []v1alpha1.ResourceTemplate{{
				ID: "svc", ForEach: "${backends}", Var: "backend",
				Template: route(`${backend.nope}`),
			}},
			wantErr: "nope",
		},
		{
			name: "errors carry the resource path",
			resources: []v1alpha1.ResourceTemplate{
				{ID: "ok", Template: route(`${metadata.name}`)},
				{ID: "bad", Template: route(`${metadata.nope}`)},
			},
			wantErr: "spec.resources[1]",
		},
		{
			name:        "validation rules type-check against the endpoint context",
			resources:   []v1alpha1.ResourceTemplate{{ID: "route", Template: route("x")}},
			validations: []v1alpha1.ValidationRule{{Rule: `${size(backends) > 0}`, Message: "needs a backend"}},
		},
		{
			name:        "validation rules must be boolean",
			resources:   []v1alpha1.ResourceTemplate{{ID: "route", Template: route("x")}},
			validations: []v1alpha1.ValidationRule{{Rule: `${size(backends)}`, Message: "m"}},
			wantErr:     "spec.validations[0]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateEndpointTypeResourcesWithSchema(tt.resources, tt.validations, nil, nil, nil, field.NewPath("spec"))
			if tt.wantErr == "" {
				if len(errs) > 0 {
					t.Fatalf("expected no errors, got: %v", errs.ToAggregate())
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("expected an error containing %q, got none", tt.wantErr)
			}
			if got := errs.ToAggregate().Error(); !strings.Contains(got, tt.wantErr) {
				t.Fatalf("expected an error containing %q, got: %s", tt.wantErr, got)
			}
		})
	}
}
