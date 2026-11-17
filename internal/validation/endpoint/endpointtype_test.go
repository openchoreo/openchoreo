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
		{
			name: "urls template every field from the render context",
			spec: withParams(v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					IncludeWhen: "${gateway.?ingress.?external.?http.hasValue()}",
					Scheme:      `${"external" in visibility ? "https" : "http"}`,
					Host:        "${metadata.endpointName}.${gateway.ingress.external.http.host}",
					Port:        "${gateway.ingress.external.http.port}",
					Path:        "${parameters.pathPrefix}/${backends[0].endpoint}",
					Security: &v1alpha1.EndpointSecurityTemplate{
						Scheme: "apiKey",
						APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "header", Name: "x-api-key"},
					},
				}},
			}),
		},
		{
			name: "every visibility may be declared",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{
					Project:   &v1alpha1.EndpointURLTemplate{Scheme: "http", Host: "p", Port: "8080"},
					Namespace: &v1alpha1.EndpointURLTemplate{Scheme: "grpc", Host: "n"},
					Internal:  &v1alpha1.EndpointURLTemplate{Scheme: "https", Host: "i"},
					External:  &v1alpha1.EndpointURLTemplate{Scheme: "wss", Host: "e"},
				},
			},
		},
		{
			name: "unknown literal scheme",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs:      &v1alpha1.EndpointTypeURLs{Internal: &v1alpha1.EndpointURLTemplate{Scheme: "ftp", Host: "h"}},
			},
			wantErr: "spec.urls.internal.scheme",
		},
		{
			name: "literal port out of range",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Port: "70000"}},
			},
			wantErr: "spec.urls.external.port",
		},
		{
			name: "standalone host expression must return a string",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "${size(backends)}"}},
			},
			wantErr: "spec.urls.external.host",
		},
		{
			name: "standalone port expression must return a number or string",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Port: "${visibility}"}},
			},
			wantErr: "spec.urls.external.port",
		},
		{
			name: "url field with undeclared parameter",
			spec: withParams(v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Path: "${parameters.basePath}"}},
			}),
			wantErr: "spec.urls.external.path",
		},
		{
			name: "url field cannot use component-only helpers",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "${workload.name}"}},
			},
			wantErr: "workload",
		},
		{
			name: "url includeWhen must be boolean",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					IncludeWhen: "${metadata.name}", Scheme: "https", Host: "h"}},
			},
			wantErr: "spec.urls.external.includeWhen",
		},
		{
			name: "unknown literal security scheme",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Security: &v1alpha1.EndpointSecurityTemplate{Scheme: "jwt"}}},
			},
			wantErr: "spec.urls.external.security.scheme",
		},
		{
			name: "apiKey scheme requires apiKey",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Security: &v1alpha1.EndpointSecurityTemplate{Scheme: "apiKey"}}},
			},
			wantErr: "spec.urls.external.security.apiKey",
		},
		{
			name: "apiKey only with apiKey scheme",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Security: &v1alpha1.EndpointSecurityTemplate{
						Scheme: "none", APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "header", Name: "k"}}}},
			},
			wantErr: "spec.urls.external.security.apiKey",
		},
		{
			name: "unknown literal apiKey location",
			spec: v1alpha1.EndpointTypeSpec{
				Resources: []v1alpha1.ResourceTemplate{tmpl("route", route("x"))},
				URLs: &v1alpha1.EndpointTypeURLs{External: &v1alpha1.EndpointURLTemplate{
					Scheme: "https", Host: "h", Security: &v1alpha1.EndpointSecurityTemplate{
						Scheme: "apiKey", APIKey: &v1alpha1.EndpointAPIKeyTemplate{In: "cookie", Name: "k"}}}},
			},
			wantErr: "spec.urls.external.security.apiKey.in",
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
