// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package tools

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// endpointRolloutSpecs returns test specs for the rollout tools on an Endpoint.
func endpointRolloutSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_endpoint_rollout",
			toolset:             "endpoint",
			descriptionKeywords: []string{"rollout"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name"},
			testArgs:            map[string]any{"namespace_name": testNamespaceName, "name": testEndpointName},
			expectedMethod:      "GetEndpointRollout",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[0] != testNamespaceName || args[1] != testEndpointName {
					t.Errorf("Expected (%s, %s), got (%v, %v)", testNamespaceName, testEndpointName, args[0], args[1])
				}
			},
		},
		{
			name:                "start_endpoint_rollout",
			toolset:             "endpoint",
			descriptionKeywords: []string{"rollout", "candidate"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "strategy", "stable", "candidate"},
			optionalParams:      []string{"steps", "dry_run"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
				"strategy":       "canary",
				"stable":         "stable",
				"candidate": map[string]any{
					"name": "canary", "component": "greeter-v2", "endpoint": "http",
					"match": map[string]any{"headers": []any{map[string]any{"name": "x-canary", "value": "true"}}},
				},
				"steps": []any{10, 100},
			},
			expectedMethod: "StartEndpointRollout",
			validateCall: func(t *testing.T, args []interface{}) {
				body, ok := args[2].(*gen.EndpointRolloutStartRequest)
				if !ok || body.Backends == nil || len(*body.Backends) != 1 {
					t.Fatalf("Expected one candidate backend, got %#v", args[2])
				}
				ref := (*body.Backends)[0]
				if ref.Weight == nil || *ref.Weight != 0 || ref.Match == nil || ref.Component.Name != "greeter-v2" {
					t.Errorf("Expected the candidate at weight 0 with its match, got %#v", ref)
				}
				if body.Rollout.Canary == nil || len(body.Rollout.Canary.Steps) != 2 ||
					body.Rollout.Canary.Steps[1].Backends[0].Percent != 100 {
					t.Errorf("Expected steps 10 and 100, got %#v", body.Rollout.Canary)
				}
			},
		},
		{
			name:                "step_endpoint_rollout",
			toolset:             "endpoint",
			descriptionKeywords: []string{"step", "environment"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "environment"},
			optionalParams:      []string{"to", "expected_step", "dry_run"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
				"environment":    "development",
				"expected_step":  1,
			},
			expectedMethod: "StepEndpointRollout",
			validateCall: func(t *testing.T, args []interface{}) {
				body, ok := args[2].(*gen.EndpointRolloutStepRequest)
				if !ok || body.Environment != "development" || body.ExpectedStep == nil || *body.ExpectedStep != 1 {
					t.Errorf("Expected development at step 1, got %#v", args[2])
				}
			},
		},
		{
			name:                "abort_endpoint_rollout",
			toolset:             "endpoint",
			descriptionKeywords: []string{"stable"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "environment"},
			optionalParams:      []string{"dry_run"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
				"environment":    "development",
			},
			expectedMethod: "AbortEndpointRollout",
			validateCall: func(t *testing.T, args []interface{}) {
				body, ok := args[2].(*gen.EndpointRolloutAbortRequest)
				if !ok || body.Environment != "development" {
					t.Errorf("Expected development, got %#v", args[2])
				}
			},
		},
		{
			name:                "finish_endpoint_rollout",
			toolset:             "endpoint",
			descriptionKeywords: []string{"rollout", "keep"},
			descriptionMinLen:   10,
			requiredParams:      []string{"namespace_name", "name", "keep"},
			optionalParams:      []string{"dry_run"},
			testArgs: map[string]any{
				"namespace_name": testNamespaceName,
				"name":           testEndpointName,
				"keep":           "canary",
			},
			expectedMethod: "FinishEndpointRollout",
			validateCall: func(t *testing.T, args []interface{}) {
				body, ok := args[2].(*gen.EndpointRolloutFinishRequest)
				if !ok || body.Keep != "canary" {
					t.Errorf("Expected keep canary, got %#v", args[2])
				}
			},
		},
	}
}

func TestRolloutStartRequest(t *testing.T) {
	t.Run("a mirror candidate gets a mirror and no match", func(t *testing.T) {
		body, err := rolloutStartRequest("mirror", "stable", endpointBackendArg{
			Name: "shadow", Component: "greeter-v2", Endpoint: "http",
			Match: &gen.EndpointMatch{Headers: &[]gen.EndpointHeaderMatch{{Name: "x-canary", Value: "true"}}},
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		ref := (*body.Backends)[0]
		if ref.Mirror == nil || ref.Match != nil {
			t.Errorf("Expected a mirror and no match, got %#v", ref)
		}
		if len(body.Rollout.Mirror.Steps) != len(defaultRolloutSteps) {
			t.Errorf("Expected the default steps, got %#v", body.Rollout.Mirror)
		}
	})

	t.Run("a candidate the endpoint has keeps no component", func(t *testing.T) {
		body, err := rolloutStartRequest("blueGreen", "stable", endpointBackendArg{Name: "green"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if ref := (*body.Backends)[0]; ref.Component != nil || body.Rollout.Canary != nil {
			t.Errorf("Expected no component and no steps, got %#v %#v", ref, body.Rollout)
		}
	})

	t.Run("refuses", func(t *testing.T) {
		for name, args := range map[string]struct {
			strategy  string
			candidate endpointBackendArg
		}{
			"an unknown strategy":          {"rolling", endpointBackendArg{Name: "canary"}},
			"an abTest without a cookie":   {"abTest", endpointBackendArg{Name: "b"}},
			"a component with no endpoint": {"canary", endpointBackendArg{Name: "canary", Component: "greeter-v2"}},
			"a candidate with no name":     {"canary", endpointBackendArg{}},
		} {
			if _, err := rolloutStartRequest(args.strategy, "stable", args.candidate, nil); err == nil {
				t.Errorf("%s: expected an error", name)
			}
		}
	})
}
