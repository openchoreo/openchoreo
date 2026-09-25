// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func strPtr(s string) *string { return &s }

// Every HookParameter source must survive a JSON round trip unchanged, because the
// webhook and the gate decide precedence (value → from → binding → default) purely
// from which fields are set; a lost or defaulted field silently changes precedence.
func TestHookParameterJSONRoundTrip(t *testing.T) {
	cases := map[string]HookParameter{
		"fixed value":        {Name: "severity", Value: strPtr("CRITICAL")},
		"from context":       {Name: "image", From: "${deployment.workload.containers.main.image}"},
		"from overridable":   {Name: "threshold", From: "${deployment.environment.name}", Overridable: true},
		"default":            {Name: "timeout", Default: strPtr("10m")},
		"required":           {Name: "ticket", Required: true},
		"empty string value": {Name: "empty", Value: strPtr("")},
		"with schema": {Name: "mode", Default: strPtr("fast"),
			Schema: &runtime.RawExtension{Raw: []byte(`{"type":"string","enum":["fast","slow"]}`)}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			b, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var out HookParameter
			if err := json.Unmarshal(b, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(in, out) {
				t.Fatalf("round trip changed the parameter:\n in: %+v\nout: %+v\njson: %s", in, out, b)
			}
		})
	}
}

// An environment manifest with hooks must round-trip unchanged, and an
// environment without hooks must not gain a hooks key (existing manifests stay
// byte-stable).
func TestEnvironmentHooksJSONRoundTrip(t *testing.T) {
	in := EnvironmentSpec{
		IsProduction: true,
		Hooks: &HookSet{
			PreDeploy: []HookBinding{{
				Name:       "image-scan",
				HookRef:    HookRef{Kind: HookRefKindClusterHook, Name: "trivy-image-scan"},
				Mode:       HookModeSync,
				Parameters: &runtime.RawExtension{Raw: []byte(`{"severity":"HIGH"}`)},
				AppliesTo:  []HookSubjectSelector{{Kind: HookSubjectSelectorKindClusterComponentType, Name: "service"}},
				OnFailure:  HookFailurePolicyBlock,
				Timeout:    "20m",
				Retries:    1,
			}},
			PostDeploy: []HookBinding{{
				Name:      "smoke",
				HookRef:   HookRef{Name: "smoke-test"},
				Mode:      HookModeAsync,
				OnFailure: HookFailurePolicyAlert,
			}},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out EnvironmentSpec
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed the spec:\n in: %+v\nout: %+v", in, out)
	}

	plain, err := json.Marshal(EnvironmentSpec{IsProduction: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(plain), "hooks") {
		t.Fatalf("environment without hooks gained a hooks key: %s", plain)
	}
}
