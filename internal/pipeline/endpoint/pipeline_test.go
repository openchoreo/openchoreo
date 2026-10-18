// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointpipeline

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
)

// rawExt marshals v and wraps it in a RawExtension.
func rawExt(t *testing.T, v any) *runtime.RawExtension {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return &runtime.RawExtension{Raw: b}
}

func fixtureMetadata() MetadataContext {
	return MetadataContext{
		Name:              "e-greeter-api-dev-1a2b3c4d",
		Namespace:         "dp-org-shop-dev-a1b2c3d",
		EndpointNamespace: "org",
		EndpointName:      "greeter-api",
		EndpointUID:       "00000000-0000-0000-0000-000000000004",
		ProjectName:       "shop",
		ProjectUID:        "00000000-0000-0000-0000-000000000001",
		EnvironmentName:   "dev",
		EnvironmentUID:    "00000000-0000-0000-0000-000000000002",
		DataPlaneName:     "primary",
		DataPlaneUID:      "00000000-0000-0000-0000-000000000003",
		Labels:            map[string]string{"openchoreo.dev/endpoint": "greeter-api"},
	}
}

func fixtureBackends() []Backend {
	return []Backend{
		{
			Name: "stable", Kind: "Component", ComponentName: "greeter", Endpoint: "http", Protocol: "HTTP",
			ServiceName: "greeter-dev-1a2b", Namespace: "dp-org-shop-dev-a1b2c3d", Port: 8080, Weight: 90,
			ServiceURL: "http://greeter-dev-1a2b.dp-org-shop-dev-a1b2c3d.svc.cluster.local:8080", BasePath: "/",
		},
		{
			Name: "canary", Kind: "Component", ComponentName: "greeter-v2", Endpoint: "http", Protocol: "HTTP",
			ServiceName: "greeter-v2-dev-3c4d", Namespace: "dp-org-shop-dev-a1b2c3d", Port: 8080, Weight: 10,
			ServiceURL: "http://greeter-v2-dev-3c4d.dp-org-shop-dev-a1b2c3d.svc.cluster.local:8080", BasePath: "/",
		},
	}
}

func fixtureEnvironment() EnvironmentContext {
	return EnvironmentContext{Gateway: &GatewayData{Ingress: &projectpipeline.GatewayNetworkData{
		External: &projectpipeline.GatewayEndpointData{
			Name:      "gateway-default",
			Namespace: "openchoreo-data-plane",
			HTTP:      &projectpipeline.GatewayListenerData{ListenerName: "http", Port: 80, Host: "openchoreoapis.localhost"},
		},
	}}}
}

// sampleTypeSpec loads the shipped ClusterEndpointType sample, so the sample
// and the CEL surface it relies on cannot drift apart unnoticed.
func sampleTypeSpec(t *testing.T) *v1alpha1.EndpointTypeSpec {
	t.Helper()
	raw, err := os.ReadFile("../../../config/samples/openchoreo_v1alpha1_clusterendpointtype.yaml")
	require.NoError(t, err)
	var cet v1alpha1.ClusterEndpointType
	require.NoError(t, yaml.Unmarshal(raw, &cet))
	spec := v1alpha1.EndpointTypeSpec(cet.Spec)
	return &spec
}

func sampleInput(t *testing.T) *RenderInput {
	return &RenderInput{
		EndpointTypeSpec: sampleTypeSpec(t),
		Parameters:       rawExt(t, map[string]any{"pathPrefix": "/greeter"}),
		Visibility:       []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityExternal},
		Backends:         fixtureBackends(),
		Metadata:         fixtureMetadata(),
		Environment:      fixtureEnvironment(),
	}
}

func TestRender_SampleRendersWeightedHTTPRoute(t *testing.T) {
	out, err := NewPipeline().Render(context.Background(), sampleInput(t))
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)

	route := out.Entries[0]
	assert.Equal(t, "httproute-external", route.ID)
	assert.Equal(t, "HTTPRoute", route.Object["kind"])

	md := route.Object["metadata"].(map[string]any)
	assert.Equal(t, "dp-org-shop-dev-a1b2c3d", md["namespace"])
	assert.Equal(t, map[string]any{"openchoreo.dev/endpoint": "greeter-api"}, md["labels"])

	spec := route.Object["spec"].(map[string]any)
	assert.Equal(t, []any{map[string]any{"name": "gateway-default", "namespace": "openchoreo-data-plane"}},
		spec["parentRefs"])
	assert.Equal(t, []any{"dev-greeter-api.openchoreoapis.localhost"}, spec["hostnames"])

	rules := spec["rules"].([]any)
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]any)
	assert.Equal(t, "/greeter", rule["matches"].([]any)[0].(map[string]any)["path"].(map[string]any)["value"])
	assert.Equal(t, []any{
		map[string]any{"name": "greeter-dev-1a2b", "port": float64(8080), "weight": float64(90)},
		map[string]any{"name": "greeter-v2-dev-3c4d", "port": float64(8080), "weight": float64(10)},
	}, rule["backendRefs"])
}

func TestRender_VisibilityGatesRoutes(t *testing.T) {
	input := sampleInput(t)
	input.Visibility = []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityInternal}

	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	assert.Empty(t, out.Entries, "the sample only renders an external route")
}

func TestRender_AppliesParameterDefaults(t *testing.T) {
	input := sampleInput(t)
	input.Parameters = nil

	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	rule := out.Entries[0].Object["spec"].(map[string]any)["rules"].([]any)[0].(map[string]any)
	assert.Equal(t, "/", rule["matches"].([]any)[0].(map[string]any)["path"].(map[string]any)["value"])
}

func TestRender_ExposesBackendFields(t *testing.T) {
	input := sampleInput(t)
	input.EndpointTypeSpec = &v1alpha1.EndpointTypeSpec{
		Resources: []v1alpha1.ResourceTemplate{{
			ID: "probe",
			Template: rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "${metadata.name}"},
				"data":       map[string]any{"backend": "${backends[0]}"},
			}),
		}},
	}

	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 1)
	assert.Equal(t, map[string]any{
		"name": "stable", "kind": "Component", "componentName": "greeter", "endpoint": "http",
		"protocol": "HTTP", "serviceName": "greeter-dev-1a2b", "namespace": "dp-org-shop-dev-a1b2c3d",
		"port": float64(8080), "weight": float64(90),
		"serviceURL": "http://greeter-dev-1a2b.dp-org-shop-dev-a1b2c3d.svc.cluster.local:8080",
		"basePath":   "/",
	}, out.Entries[0].Object["data"].(map[string]any)["backend"])
}

func TestRender_ForEachOverBackends(t *testing.T) {
	input := sampleInput(t)
	input.EndpointTypeSpec = &v1alpha1.EndpointTypeSpec{
		Resources: []v1alpha1.ResourceTemplate{{
			ID:      "per-backend",
			ForEach: "${backends}",
			Var:     "backend",
			Template: rawExt(t, map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata":   map[string]any{"name": "${metadata.name + '-' + backend.name}"},
			}),
		}},
	}

	out, err := NewPipeline().Render(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, out.Entries, 2)
	assert.Equal(t, "per-backend-0", out.Entries[0].ID)
	assert.Equal(t, "e-greeter-api-dev-1a2b3c4d-stable", out.Entries[0].Object["metadata"].(map[string]any)["name"])
	assert.Equal(t, "per-backend-1", out.Entries[1].ID)
	assert.Equal(t, "e-greeter-api-dev-1a2b3c4d-canary", out.Entries[1].Object["metadata"].(map[string]any)["name"])
}

func TestRender_ValidationRuleOverBackendsAborts(t *testing.T) {
	input := sampleInput(t)
	input.EndpointTypeSpec.Validations = []v1alpha1.ValidationRule{{
		Rule:    `${backends.all(b, b.protocol == "gRPC")}`,
		Message: "this type only routes gRPC backends",
	}}

	_, err := NewPipeline().Render(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "this type only routes gRPC backends")
}

func TestRender_RejectsNonDataPlaneTarget(t *testing.T) {
	input := sampleInput(t)
	input.EndpointTypeSpec.Resources[0].TargetPlane = "observabilityplane"

	_, err := NewPipeline().Render(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `targetPlane "observabilityplane" is not supported`)
}

func TestRender_InputValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*RenderInput)
		wantErr string
	}{
		{name: "nil spec", mutate: func(in *RenderInput) { in.EndpointTypeSpec = nil }, wantErr: "EndpointTypeSpec is nil"},
		{name: "no namespace", mutate: func(in *RenderInput) { in.Metadata.Namespace = "" }, wantErr: "Namespace is empty"},
		{name: "no backends", mutate: func(in *RenderInput) { in.Backends = nil }, wantErr: "Backends is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := sampleInput(t)
			tt.mutate(input)
			_, err := NewPipeline().Render(context.Background(), input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
