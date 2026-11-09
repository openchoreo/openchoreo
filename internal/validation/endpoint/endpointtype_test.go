// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"os"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestValidateEndpointTypeSpec_ShippedSampleIsValid(t *testing.T) {
	raw, err := os.ReadFile("../../../config/samples/openchoreo_v1alpha1_clusterendpointtype.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cet v1alpha1.ClusterEndpointType
	if err := yaml.Unmarshal(raw, &cet); err != nil {
		t.Fatal(err)
	}
	if errs := ValidateClusterEndpointTypeSpec(&cet.Spec, field.NewPath("spec")); len(errs) > 0 {
		t.Fatalf("shipped sample must pass admission, got: %v", errs.ToAggregate())
	}
}

func TestValidateEndpointTypeSpec(t *testing.T) {
	const paramsSchema = `{"type":"object","properties":{"pathPrefix":{"type":"string","default":"/"}}}`
	tmpl := func(id, body string) v1alpha1.ResourceTemplate {
		return v1alpha1.ResourceTemplate{ID: id, Template: &runtime.RawExtension{Raw: []byte(body)}}
	}
	route := func(name string) string {
		return `{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"` + name + `"}}`
	}
	withParams := func(spec v1alpha1.EndpointTypeSpec) v1alpha1.EndpointTypeSpec {
		spec.Parameters = &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(paramsSchema)}}
		return spec
	}

	tests := []struct {
		name    string
		spec    v1alpha1.EndpointTypeSpec
		wantErr string // empty means valid; otherwise a substring of the error
	}{
		{
			name: "every context variable resolves",
			spec: withParams(v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{tmpl("route",
				`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"${metadata.name}","namespace":"${metadata.namespace}"},`+
					`"data":{"a":"${parameters.pathPrefix}","b":"${string(size(visibility))}","c":"${backends[0].serviceName}",`+
					`"d":"${metadata.endpointName + metadata.environmentName}","e":"${string(backends[0].port)}"}}`)}}),
		},
		{
			name: "unknown backend field",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{
				tmpl("route", route("${backends[0].hostname}"))}},
			wantErr: "spec.resources[0].template",
		},
		{
			name: "undeclared parameter",
			spec: withParams(v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{
				tmpl("route", route("${parameters.host}"))}}),
			wantErr: "host",
		},
		{
			name: "component-only helper is not available",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{
				tmpl("route", route("${workload.name}"))}},
			wantErr: "workload",
		},
		{
			name: "includeWhen over visibility",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{{
				ID: "route", IncludeWhen: `${"external" in visibility}`,
				Template: &runtime.RawExtension{Raw: []byte(route("${metadata.name}"))},
			}}},
		},
		{
			name: "includeWhen must be boolean",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{{
				ID: "route", IncludeWhen: `${metadata.name}`,
				Template: &runtime.RawExtension{Raw: []byte(route("x"))},
			}}},
			wantErr: "includeWhen must return boolean",
		},
		{
			name: "forEach over backends scopes the loop variable",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{{
				ID: "svc", ForEach: "${backends}", Var: "backend",
				Template: &runtime.RawExtension{Raw: []byte(route("${metadata.name + '-' + backend.name}"))},
			}}},
		},
		{
			name: "forEach loop variable is typed",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{{
				ID: "svc", ForEach: "${backends}", Var: "backend",
				Template: &runtime.RawExtension{Raw: []byte(route("${backend.nope}"))},
			}}},
			wantErr: "nope",
		},
		{
			name: "validation rule must be boolean",
			spec: v1alpha1.EndpointTypeSpec{
				Validations: []v1alpha1.ValidationRule{{Rule: "${size(backends)}", Message: "m"}},
				Resources:   []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
			},
			wantErr: "spec.validations[0].rule",
		},
		{
			name: "validation rule over backends",
			spec: v1alpha1.EndpointTypeSpec{
				Validations: []v1alpha1.ValidationRule{{Rule: `${backends.all(b, b.protocol == "HTTP")}`, Message: "m"}},
				Resources:   []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
			},
		},
		{
			name: "non-dataplane target plane",
			spec: v1alpha1.EndpointTypeSpec{Resources: []v1alpha1.ResourceTemplate{{
				ID: "route", TargetPlane: "observabilityplane",
				Template: &runtime.RawExtension{Raw: []byte(route("x"))},
			}}},
			wantErr: "spec.resources[0].targetPlane",
		},
		{
			name: "malformed parameters schema",
			spec: v1alpha1.EndpointTypeSpec{
				Parameters: &v1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type":`)}},
				Resources:  []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
			},
			wantErr: "spec.parameters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpointTypeSpec(&tt.spec, field.NewPath("spec")).ToAggregate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("expected an error containing %q, got none", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Fatalf("expected an error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}
