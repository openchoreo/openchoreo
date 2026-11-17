// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

//nolint:staticcheck // these tests exercise the deprecated type and visibility fields on purpose
package workload

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	v1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	protoHTTP = v1.EndpointProtocolHTTP
	protoGRPC = v1.EndpointProtocolGRPC
)

func workloadWith(endpoints map[string]v1.WorkloadEndpoint) *v1.Workload {
	w := &v1.Workload{}
	w.Name = "orders"
	w.Spec.Endpoints = endpoints
	return w
}

func TestSyncedProtocol(t *testing.T) {
	tests := []struct {
		name           string
		protocol, typ  v1.EndpointProtocol
		old            *v1.WorkloadEndpoint
		wantP, wantTyp v1.EndpointProtocol
	}{
		{name: "only type", typ: protoHTTP, wantP: protoHTTP, wantTyp: protoHTTP},
		{name: "only protocol", protocol: protoGRPC, wantP: protoGRPC, wantTyp: protoGRPC},
		{name: "both equal", protocol: protoHTTP, typ: protoHTTP, wantP: protoHTTP, wantTyp: protoHTTP},
		{name: "neither", wantP: "", wantTyp: ""},
		{name: "create with a mismatch keeps both for validation", protocol: protoGRPC, typ: protoHTTP, wantP: protoGRPC, wantTyp: protoHTTP},
		{
			name: "update changing only protocol carries it to type", protocol: protoGRPC, typ: protoHTTP,
			old: &v1.WorkloadEndpoint{Protocol: protoHTTP, Type: protoHTTP}, wantP: protoGRPC, wantTyp: protoGRPC,
		},
		{
			name: "update changing only type carries it to protocol", protocol: protoHTTP, typ: protoGRPC,
			old: &v1.WorkloadEndpoint{Protocol: protoHTTP, Type: protoHTTP}, wantP: protoGRPC, wantTyp: protoGRPC,
		},
		{
			name: "update changing both differently keeps both for validation", protocol: protoGRPC, typ: v1.EndpointProtocolTCP,
			old: &v1.WorkloadEndpoint{Protocol: protoHTTP, Type: protoHTTP}, wantP: protoGRPC, wantTyp: v1.EndpointProtocolTCP,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, typ := syncedProtocol(tt.protocol, tt.typ, tt.old)
			if p != tt.wantP || typ != tt.wantTyp {
				t.Fatalf("got (%q, %q), want (%q, %q)", p, typ, tt.wantP, tt.wantTyp)
			}
		})
	}
}

func TestSyncEndpointProtocolsWarnings(t *testing.T) {
	w := workloadWith(map[string]v1.WorkloadEndpoint{
		"new":    {Protocol: protoHTTP, Port: 8080},
		"legacy": {Type: protoGRPC, Port: 9090, Visibility: []v1.EndpointVisibility{v1.EndpointVisibilityExternal}},
	})

	warnings := syncEndpointProtocols(w, nil)

	want := []string{
		"spec.endpoints[legacy].type is deprecated; use spec.endpoints[legacy].protocol",
		"spec.endpoints[legacy].visibility is deprecated; expose the endpoint through an Endpoint, whose spec.visibility carries it",
	}
	if len(warnings) != len(want) {
		t.Fatalf("got warnings %q, want %q", warnings, want)
	}
	for i := range want {
		if warnings[i] != want[i] {
			t.Errorf("warning %d: got %q, want %q", i, warnings[i], want[i])
		}
	}
	if got := w.Spec.Endpoints["legacy"]; got.Protocol != protoGRPC || got.Type != protoGRPC {
		t.Errorf("legacy endpoint not synced: %+v", got)
	}
	if got := w.Spec.Endpoints["new"]; got.Protocol != protoHTTP || got.Type != protoHTTP {
		t.Errorf("new endpoint not synced: %+v", got)
	}
}

func TestDefaulterHandle(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	d := &Defaulter{decoder: admission.NewDecoder(scheme)}

	raw, err := json.Marshal(workloadWith(map[string]v1.WorkloadEndpoint{"api": {Type: protoHTTP, Port: 8080}}))
	if err != nil {
		t.Fatal(err)
	}
	resp := d.Handle(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation: admissionv1.Create,
		Object:    runtime.RawExtension{Raw: raw},
	}})

	if !resp.Allowed {
		t.Fatalf("request denied: %v", resp.Result)
	}
	found := false
	for _, op := range resp.Patches {
		if op.Operation == "add" && op.Path == "/spec/endpoints/api/protocol" && op.Value == "HTTP" {
			found = true
		}
	}
	if !found {
		t.Errorf("patches %v do not add the protocol", resp.Patches)
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("got warnings %q, want one deprecation warning", resp.Warnings)
	}
}

func TestValidateEndpointProtocols(t *testing.T) {
	tests := []struct {
		name      string
		endpoints map[string]v1.WorkloadEndpoint
		errs      int
	}{
		{name: "protocol only", endpoints: map[string]v1.WorkloadEndpoint{"a": {Protocol: protoHTTP}}},
		{name: "type only", endpoints: map[string]v1.WorkloadEndpoint{"a": {Type: protoHTTP}}},
		{name: "both equal", endpoints: map[string]v1.WorkloadEndpoint{"a": {Protocol: protoHTTP, Type: protoHTTP}}},
		{name: "neither", endpoints: map[string]v1.WorkloadEndpoint{"a": {Port: 80}}, errs: 1},
		{name: "mismatch", endpoints: map[string]v1.WorkloadEndpoint{"a": {Protocol: protoGRPC, Type: protoHTTP}}, errs: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateEndpointProtocols(tt.endpoints, field.NewPath("spec", "endpoints"))
			if len(errs) != tt.errs {
				t.Fatalf("got %d errors, want %d: %v", len(errs), tt.errs, errs)
			}
		})
	}
}
