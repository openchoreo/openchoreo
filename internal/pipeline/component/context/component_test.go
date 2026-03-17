// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// validMetadata returns a MetadataContext that satisfies all "required" validation tags.
func validMetadata() MetadataContext {
	return MetadataContext{
		Name:               "comp-dev-12345678",
		Namespace:          "dp-ns",
		ComponentName:      "comp",
		ComponentUID:       "uid-comp",
		ComponentNamespace: "cp-ns",
		ProjectName:        "proj",
		ProjectUID:         "uid-proj",
		DataPlaneName:      "dp",
		DataPlaneUID:       "uid-dp",
		EnvironmentName:    "dev",
		EnvironmentUID:     "uid-env",
		Labels:             map[string]string{},
		Annotations:        map[string]string{},
		PodSelectors:       map[string]string{"app": "comp"},
	}
}

// minimalDataPlane returns a DataPlane with no optional fields.
func minimalDataPlane() *v1alpha1.DataPlane {
	return &v1alpha1.DataPlane{
		Spec: v1alpha1.DataPlaneSpec{},
	}
}

// minimalEnvironment returns an Environment with no gateway config.
func minimalEnvironment() *v1alpha1.Environment {
	return &v1alpha1.Environment{
		Spec: v1alpha1.EnvironmentSpec{},
	}
}

// openAPIV3Schema builds a SchemaSection from a proper OpenAPI v3 schema map.
// Example: openAPIV3Schema(map[string]any{"type": "object", "properties": map[string]any{...}})
func openAPIV3Schema(schema map[string]any) *v1alpha1.SchemaSection {
	raw, _ := json.Marshal(schema)
	return &v1alpha1.SchemaSection{
		OpenAPIV3Schema: &runtime.RawExtension{Raw: raw},
	}
}

// integerPropSchema returns an OpenAPI v3 property schema for an integer field.
func integerPropSchema(opts ...any) map[string]any {
	prop := map[string]any{"type": "integer"}
	if len(opts) > 0 {
		prop["default"] = opts[0]
	}
	return prop
}

// stringPropSchema returns an OpenAPI v3 property schema for a string field.
func stringPropSchema(opts ...any) map[string]any {
	prop := map[string]any{"type": "string"}
	if len(opts) > 0 {
		prop["default"] = opts[0]
	}
	return prop
}

// objectSchema wraps properties into a full OpenAPI v3 object schema.
func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

// rawParams builds a *runtime.RawExtension from a map.
func rawParams(m map[string]any) *runtime.RawExtension {
	raw, _ := json.Marshal(m)
	return &runtime.RawExtension{Raw: raw}
}

// --- BuildComponentContext validation tests ---

func TestBuildComponentContext_ValidationErrors(t *testing.T) {
	tests := []struct {
		name  string
		input *ComponentContextInput
	}{
		{
			name: "nil Component",
			input: &ComponentContextInput{
				Component:     nil,
				ComponentType: &v1alpha1.ComponentType{},
				DataPlane:     minimalDataPlane(),
				Environment:   minimalEnvironment(),
				Metadata:      validMetadata(),
			},
		},
		{
			name: "nil ComponentType",
			input: &ComponentContextInput{
				Component:     &v1alpha1.Component{},
				ComponentType: nil,
				DataPlane:     minimalDataPlane(),
				Environment:   minimalEnvironment(),
				Metadata:      validMetadata(),
			},
		},
		{
			name: "nil DataPlane",
			input: &ComponentContextInput{
				Component:     &v1alpha1.Component{},
				ComponentType: &v1alpha1.ComponentType{},
				DataPlane:     nil,
				Environment:   minimalEnvironment(),
				Metadata:      validMetadata(),
			},
		},
		{
			name: "nil Environment",
			input: &ComponentContextInput{
				Component:     &v1alpha1.Component{},
				ComponentType: &v1alpha1.ComponentType{},
				DataPlane:     minimalDataPlane(),
				Environment:   nil,
				Metadata:      validMetadata(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := BuildComponentContext(tt.input)
			require.Error(t, err)
			assert.Nil(t, ctx)
			assert.Contains(t, err.Error(), "validation failed")
		})
	}
}

func TestBuildComponentContext_NilMetadataMaps(t *testing.T) {
	input := &ComponentContextInput{
		Component:     &v1alpha1.Component{},
		ComponentType: &v1alpha1.ComponentType{},
		DataPlane:     minimalDataPlane(),
		Environment:   minimalEnvironment(),
		Metadata: MetadataContext{
			Name:               "comp-dev-12345678",
			Namespace:          "dp-ns",
			ComponentName:      "comp",
			ComponentUID:       "uid-comp",
			ComponentNamespace: "cp-ns",
			ProjectName:        "proj",
			ProjectUID:         "uid-proj",
			DataPlaneName:      "dp",
			DataPlaneUID:       "uid-dp",
			EnvironmentName:    "dev",
			EnvironmentUID:     "uid-env",
			Labels:             nil,
			Annotations:        nil,
			PodSelectors:       map[string]string{"app": "comp"},
		},
	}

	// The validator will reject nil Labels/Annotations because they have `validate:"required"`.
	// We need to check that the validation accepts them or that after passing they get initialized.
	// Since PodSelectors has `validate:"required,min=1"` and Labels/Annotations have `validate:"required"`,
	// nil maps should fail validation.
	_, err := BuildComponentContext(input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")

	// Now test with empty maps (which pass validation) — verify they remain initialized
	input.Metadata.Labels = map[string]string{}
	input.Metadata.Annotations = map[string]string{}
	ctx, err := BuildComponentContext(input)
	require.NoError(t, err)
	assert.NotNil(t, ctx.Metadata.Labels)
	assert.NotNil(t, ctx.Metadata.Annotations)
	assert.NotNil(t, ctx.Metadata.PodSelectors)
}

// --- processComponentParameters tests ---

func TestProcessComponentParameters_InvalidSchema(t *testing.T) {
	input := &ComponentContextInput{
		Component: &v1alpha1.Component{},
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Parameters: &v1alpha1.SchemaSection{
					OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type": "object", "properties": "invalid"}`)},
				},
			},
		},
		DataPlane:   minimalDataPlane(),
		Environment: minimalEnvironment(),
		Metadata:    validMetadata(),
	}

	ctx, err := BuildComponentContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "schema")
}

func TestProcessComponentParameters_InvalidJSON(t *testing.T) {
	input := &ComponentContextInput{
		Component: &v1alpha1.Component{
			Spec: v1alpha1.ComponentSpec{
				Parameters: &runtime.RawExtension{Raw: []byte(`{invalid json}`)},
			},
		},
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Parameters: openAPIV3Schema(objectSchema(map[string]any{
					"replicas": integerPropSchema(),
				})),
			},
		},
		DataPlane:   minimalDataPlane(),
		Environment: minimalEnvironment(),
		Metadata:    validMetadata(),
	}

	ctx, err := BuildComponentContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "parameters")
}

func TestProcessComponentParameters_ValidationFailure(t *testing.T) {
	// Schema expects integer, but we provide a string value
	input := &ComponentContextInput{
		Component: &v1alpha1.Component{
			Spec: v1alpha1.ComponentSpec{
				Parameters: rawParams(map[string]any{"replicas": "not-an-integer"}),
			},
		},
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Parameters: openAPIV3Schema(objectSchema(map[string]any{
					"replicas": integerPropSchema(),
				})),
			},
		},
		DataPlane:   minimalDataPlane(),
		Environment: minimalEnvironment(),
		Metadata:    validMetadata(),
	}

	ctx, err := BuildComponentContext(input)
	require.Error(t, err)
	assert.Nil(t, ctx)
	assert.Contains(t, err.Error(), "validation failed")
}

func TestProcessComponentParameters_NoSchema(t *testing.T) {
	input := &ComponentContextInput{
		Component:     &v1alpha1.Component{},
		ComponentType: &v1alpha1.ComponentType{},
		DataPlane:     minimalDataPlane(),
		Environment:   minimalEnvironment(),
		Metadata:      validMetadata(),
	}

	ctx, err := BuildComponentContext(input)
	require.NoError(t, err)
	assert.Empty(t, ctx.Parameters)
	assert.Empty(t, ctx.EnvironmentConfigs)
}

func TestProcessComponentParameters_PruneAndDefault(t *testing.T) {
	// Schema defines "replicas" (integer, default=1), "image" (string), and "protocol" (string, default="TCP").
	// Component provides "image" and "extra" but omits "replicas" and "protocol" — those should get defaults.
	input := &ComponentContextInput{
		Component: &v1alpha1.Component{
			Spec: v1alpha1.ComponentSpec{
				Parameters: rawParams(map[string]any{
					"image": "myapp:v1",
					"extra": "should-be-pruned",
				}),
			},
		},
		ComponentType: &v1alpha1.ComponentType{
			Spec: v1alpha1.ComponentTypeSpec{
				Parameters: openAPIV3Schema(objectSchema(map[string]any{
					"replicas": integerPropSchema(1),
					"image":    stringPropSchema(),
					"protocol": stringPropSchema("TCP"),
				})),
			},
		},
		DataPlane:   minimalDataPlane(),
		Environment: minimalEnvironment(),
		Metadata:    validMetadata(),
	}

	ctx, err := BuildComponentContext(input)
	require.NoError(t, err)
	// extra key should be pruned
	_, hasExtra := ctx.Parameters["extra"]
	assert.False(t, hasExtra, "extra key should be pruned")
	// provided values should remain
	assert.Equal(t, "myapp:v1", ctx.Parameters["image"])
	// omitted fields should get schema defaults
	assert.Equal(t, int64(1), ctx.Parameters["replicas"])
	assert.Equal(t, "TCP", ctx.Parameters["protocol"])
}

// --- extractDataPlaneData tests ---

func TestExtractDataPlaneData_Full(t *testing.T) {
	dp := &v1alpha1.DataPlane{
		Spec: v1alpha1.DataPlaneSpec{
			SecretStoreRef: &v1alpha1.SecretStoreRef{
				Name: "my-secret-store",
			},
			ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
				Kind: v1alpha1.ObservabilityPlaneRefKindClusterObservabilityPlane,
				Name: "cluster-obs",
			},
			Gateway: v1alpha1.GatewaySpec{
				Ingress: &v1alpha1.GatewayNetworkSpec{
					External: &v1alpha1.GatewayEndpointSpec{
						Name:      "ext-gw",
						Namespace: "gw-ns",
						HTTPS: &v1alpha1.GatewayListenerSpec{
							ListenerName: "https",
							Port:         443,
							Host:         "api.example.com",
						},
					},
				},
			},
		},
	}

	data := extractDataPlaneData(dp)
	assert.Equal(t, "my-secret-store", data.SecretStore)
	require.NotNil(t, data.ObservabilityPlaneRef)
	assert.Equal(t, "ClusterObservabilityPlane", data.ObservabilityPlaneRef.Kind)
	assert.Equal(t, "cluster-obs", data.ObservabilityPlaneRef.Name)
	require.NotNil(t, data.Gateway)
	require.NotNil(t, data.Gateway.Ingress)
	require.NotNil(t, data.Gateway.Ingress.External)
	assert.Equal(t, "ext-gw", data.Gateway.Ingress.External.Name)
	require.NotNil(t, data.Gateway.Ingress.External.HTTPS)
	assert.Equal(t, "api.example.com", data.Gateway.Ingress.External.HTTPS.Host)
	assert.Equal(t, int32(443), data.Gateway.Ingress.External.HTTPS.Port)
}

func TestExtractDataPlaneData_Minimal(t *testing.T) {
	dp := minimalDataPlane()
	data := extractDataPlaneData(dp)
	assert.Empty(t, data.SecretStore)
	assert.Nil(t, data.ObservabilityPlaneRef)
	assert.Nil(t, data.Gateway)
}

// --- toGatewayData tests ---

func TestToGatewayData_Full(t *testing.T) {
	gw := &v1alpha1.GatewaySpec{
		Ingress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "ingress-gw",
				Namespace: "gw-ns",
			},
		},
		Egress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      "egress-gw",
				Namespace: "gw-ns",
			},
		},
	}

	data := toGatewayData(gw)
	require.NotNil(t, data)
	require.NotNil(t, data.Ingress)
	require.NotNil(t, data.Ingress.External)
	assert.Equal(t, "ingress-gw", data.Ingress.External.Name)
	require.NotNil(t, data.Egress)
	require.NotNil(t, data.Egress.External)
	assert.Equal(t, "egress-gw", data.Egress.External.Name)
}

func TestToGatewayData_NilInput(t *testing.T) {
	data := toGatewayData(nil)
	assert.Nil(t, data)
}

func TestToGatewayData_EmptySpec(t *testing.T) {
	// GatewaySpec with neither Ingress nor Egress should return nil
	gw := &v1alpha1.GatewaySpec{}
	data := toGatewayData(gw)
	assert.Nil(t, data)
}

// --- toGatewayNetworkData tests ---

func TestToGatewayNetworkData_Full(t *testing.T) {
	network := &v1alpha1.GatewayNetworkSpec{
		External: &v1alpha1.GatewayEndpointSpec{
			Name:      "ext-gw",
			Namespace: "gw-ns",
		},
		Internal: &v1alpha1.GatewayEndpointSpec{
			Name:      "int-gw",
			Namespace: "gw-ns",
		},
	}

	data := toGatewayNetworkData(network)
	require.NotNil(t, data)
	require.NotNil(t, data.External)
	assert.Equal(t, "ext-gw", data.External.Name)
	require.NotNil(t, data.Internal)
	assert.Equal(t, "int-gw", data.Internal.Name)
}

func TestToGatewayNetworkData_Nil(t *testing.T) {
	data := toGatewayNetworkData(nil)
	assert.Nil(t, data)
}

// --- toGatewayEndpointData tests ---

func TestToGatewayEndpointData_Full(t *testing.T) {
	ep := &v1alpha1.GatewayEndpointSpec{
		Name:      "gw",
		Namespace: "gw-ns",
		HTTP: &v1alpha1.GatewayListenerSpec{
			ListenerName: "http-listener",
			Port:         80,
			Host:         "http.example.com",
		},
		HTTPS: &v1alpha1.GatewayListenerSpec{
			ListenerName: "https-listener",
			Port:         443,
			Host:         "https.example.com",
		},
		TLS: &v1alpha1.GatewayListenerSpec{
			ListenerName: "tls-listener",
			Port:         8443,
			Host:         "tls.example.com",
		},
	}

	data := toGatewayEndpointData(ep)
	require.NotNil(t, data)
	assert.Equal(t, "gw", data.Name)
	assert.Equal(t, "gw-ns", data.Namespace)

	require.NotNil(t, data.HTTP)
	assert.Equal(t, "http-listener", data.HTTP.ListenerName)
	assert.Equal(t, int32(80), data.HTTP.Port)
	assert.Equal(t, "http.example.com", data.HTTP.Host)

	require.NotNil(t, data.HTTPS)
	assert.Equal(t, "https-listener", data.HTTPS.ListenerName)
	assert.Equal(t, int32(443), data.HTTPS.Port)
	assert.Equal(t, "https.example.com", data.HTTPS.Host)

	require.NotNil(t, data.TLS)
	assert.Equal(t, "tls-listener", data.TLS.ListenerName)
	assert.Equal(t, int32(8443), data.TLS.Port)
	assert.Equal(t, "tls.example.com", data.TLS.Host)
}

func TestToGatewayEndpointData_Nil(t *testing.T) {
	data := toGatewayEndpointData(nil)
	assert.Nil(t, data)
}

func TestToGatewayEndpointData_PartialListeners(t *testing.T) {
	// Only HTTP listener set, HTTPS and TLS are nil
	ep := &v1alpha1.GatewayEndpointSpec{
		Name:      "gw",
		Namespace: "gw-ns",
		HTTP: &v1alpha1.GatewayListenerSpec{
			ListenerName: "http",
			Port:         80,
			Host:         "api.example.com",
		},
	}

	data := toGatewayEndpointData(ep)
	require.NotNil(t, data)
	require.NotNil(t, data.HTTP)
	assert.Equal(t, int32(80), data.HTTP.Port)
	assert.Nil(t, data.HTTPS)
	assert.Nil(t, data.TLS)
}

// --- extractEnvironmentData / mergeGatewayData tests ---

func TestExtractEnvironmentData_GatewayMerge(t *testing.T) {
	// Environment overrides dp ingress external; dp provides ingress internal as fallback
	dp := &v1alpha1.DataPlane{
		Spec: v1alpha1.DataPlaneSpec{
			Gateway: v1alpha1.GatewaySpec{
				Ingress: &v1alpha1.GatewayNetworkSpec{
					External: &v1alpha1.GatewayEndpointSpec{
						Name:      "dp-ext-gw",
						Namespace: "dp-ns",
						HTTPS: &v1alpha1.GatewayListenerSpec{
							ListenerName: "dp-https",
							Port:         443,
							Host:         "dp.example.com",
						},
					},
					Internal: &v1alpha1.GatewayEndpointSpec{
						Name:      "dp-int-gw",
						Namespace: "dp-ns",
						HTTP: &v1alpha1.GatewayListenerSpec{
							ListenerName: "dp-http",
							Port:         80,
							Host:         "internal.dp.example.com",
						},
					},
				},
			},
		},
	}

	env := &v1alpha1.Environment{
		Spec: v1alpha1.EnvironmentSpec{
			Gateway: v1alpha1.GatewaySpec{
				Ingress: &v1alpha1.GatewayNetworkSpec{
					External: &v1alpha1.GatewayEndpointSpec{
						Name:      "env-ext-gw",
						Namespace: "env-ns",
						HTTPS: &v1alpha1.GatewayListenerSpec{
							ListenerName: "env-https",
							Port:         443,
							Host:         "env.example.com",
						},
					},
					// Internal not set in env -> should fall back to dp
				},
			},
		},
	}

	data := extractEnvironmentData(env, dp, "slack-channel")
	require.NotNil(t, data.Gateway)
	require.NotNil(t, data.Gateway.Ingress)

	// External should come from env (env wins)
	require.NotNil(t, data.Gateway.Ingress.External)
	assert.Equal(t, "env-ext-gw", data.Gateway.Ingress.External.Name)
	assert.Equal(t, "env.example.com", data.Gateway.Ingress.External.HTTPS.Host)

	// Internal should fall back to dp
	require.NotNil(t, data.Gateway.Ingress.Internal)
	assert.Equal(t, "dp-int-gw", data.Gateway.Ingress.Internal.Name)

	assert.Equal(t, "slack-channel", data.DefaultNotificationChannel)
}

func TestMergeGatewayData_BothNil(t *testing.T) {
	data := mergeGatewayData(nil, nil)
	assert.Nil(t, data)
}

// --- mergeGatewayNetworkData tests ---

func TestMergeGatewayNetworkData_Fallback(t *testing.T) {
	envNetwork := &v1alpha1.GatewayNetworkSpec{
		External: &v1alpha1.GatewayEndpointSpec{
			Name:      "env-ext",
			Namespace: "env-ns",
		},
	}
	dpNetwork := &v1alpha1.GatewayNetworkSpec{
		Internal: &v1alpha1.GatewayEndpointSpec{
			Name:      "dp-int",
			Namespace: "dp-ns",
		},
	}

	data := mergeGatewayNetworkData(envNetwork, dpNetwork)
	require.NotNil(t, data)
	require.NotNil(t, data.External)
	assert.Equal(t, "env-ext", data.External.Name)
	require.NotNil(t, data.Internal)
	assert.Equal(t, "dp-int", data.Internal.Name)
}

func TestMergeGatewayNetworkData_BothNil(t *testing.T) {
	data := mergeGatewayNetworkData(nil, nil)
	assert.Nil(t, data)
}

func TestMergeGatewayNetworkData_EnvWins(t *testing.T) {
	envNetwork := &v1alpha1.GatewayNetworkSpec{
		External: &v1alpha1.GatewayEndpointSpec{
			Name:      "env-ext",
			Namespace: "env-ns",
		},
	}
	dpNetwork := &v1alpha1.GatewayNetworkSpec{
		External: &v1alpha1.GatewayEndpointSpec{
			Name:      "dp-ext",
			Namespace: "dp-ns",
		},
	}

	data := mergeGatewayNetworkData(envNetwork, dpNetwork)
	require.NotNil(t, data)
	require.NotNil(t, data.External)
	// env external should take precedence over dp external
	assert.Equal(t, "env-ext", data.External.Name)
	assert.Equal(t, "env-ns", data.External.Namespace)
}

// --- ExtractWorkloadData tests ---

func TestExtractWorkloadData_WithEndpoints(t *testing.T) {
	workload := &v1alpha1.Workload{
		Spec: v1alpha1.WorkloadSpec{
			WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
				Container: v1alpha1.Container{
					Image:   "myapp:v1",
					Command: []string{"/bin/app"},
					Args:    []string{"--port=8080"},
				},
				Endpoints: map[string]v1alpha1.WorkloadEndpoint{
					"http-ep": {
						DisplayName: "HTTP Endpoint",
						Type:        v1alpha1.EndpointTypeHTTP,
						Port:        8080,
						TargetPort:  9090,
						BasePath:    "/api",
						Visibility:  []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityExternal, v1alpha1.EndpointVisibilityProject},
						Schema: &v1alpha1.Schema{
							Type:    "openapi",
							Content: "spec-content",
						},
					},
					"grpc-ep": {
						Type:       v1alpha1.EndpointTypeGRPC,
						Port:       9090,
						Visibility: []v1alpha1.EndpointVisibility{v1alpha1.EndpointVisibilityNamespace},
					},
				},
			},
		},
	}

	data := ExtractWorkloadData(workload)

	// Container
	assert.Equal(t, "myapp:v1", data.Container.Image)
	assert.Equal(t, []string{"/bin/app"}, data.Container.Command)
	assert.Equal(t, []string{"--port=8080"}, data.Container.Args)

	// HTTP endpoint
	httpEp, ok := data.Endpoints["http-ep"]
	require.True(t, ok)
	assert.Equal(t, "HTTP Endpoint", httpEp.DisplayName)
	assert.Equal(t, int32(8080), httpEp.Port)
	assert.Equal(t, int32(9090), httpEp.TargetPort)
	assert.Equal(t, "/api", httpEp.BasePath)
	assert.Equal(t, "HTTP", httpEp.Type)

	// Visibility dedup: "project" always included first, then "external" added (duplicate "project" removed)
	assert.Equal(t, "project", httpEp.Visibility[0])
	assert.Contains(t, httpEp.Visibility, "external")
	// Should not have duplicate "project"
	projectCount := 0
	for _, v := range httpEp.Visibility {
		if v == "project" {
			projectCount++
		}
	}
	assert.Equal(t, 1, projectCount, "project should appear exactly once")

	// gRPC endpoint: targetPort should fall back to port
	grpcEp, ok := data.Endpoints["grpc-ep"]
	require.True(t, ok)
	assert.Equal(t, int32(9090), grpcEp.Port)
	assert.Equal(t, int32(9090), grpcEp.TargetPort, "targetPort should fall back to port when 0")
	assert.Equal(t, "gRPC", grpcEp.Type)

	// gRPC visibility: project always first, then namespace
	assert.Equal(t, "project", grpcEp.Visibility[0])
	assert.Contains(t, grpcEp.Visibility, "namespace")
}

func TestExtractWorkloadData_NilWorkload(t *testing.T) {
	data := ExtractWorkloadData(nil)
	assert.NotNil(t, data.Endpoints, "Endpoints map should be initialized, not nil")
	assert.Empty(t, data.Endpoints)
	assert.Empty(t, data.Container.Image)
}

// --- extractParameters tests ---

func TestExtractParameters_NilRaw(t *testing.T) {
	result, err := extractParameters(nil)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestExtractParameters_EmptyRaw(t *testing.T) {
	// RawExtension with nil Raw bytes
	raw := &runtime.RawExtension{Raw: nil}
	result, err := extractParameters(raw)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestExtractParameters_InvalidJSON(t *testing.T) {
	raw := &runtime.RawExtension{Raw: []byte(`{bad json`)}
	result, err := extractParameters(raw)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestExtractParameters_ValidJSON(t *testing.T) {
	raw := &runtime.RawExtension{Raw: []byte(`{"replicas":3,"name":"test"}`)}
	result, err := extractParameters(raw)
	require.NoError(t, err)
	assert.Equal(t, float64(3), result["replicas"])
	assert.Equal(t, "test", result["name"])
}

// --- BuildStructuralSchemas tests ---

func TestBuildStructuralSchemas_OneSchemaValid(t *testing.T) {
	input := &SchemaInput{
		ParametersSchema: openAPIV3Schema(objectSchema(map[string]any{
			"replicas": integerPropSchema(1),
		})),
		EnvironmentConfigsSchema: nil,
	}

	paramBundle, envBundle, err := BuildStructuralSchemas(input)
	require.NoError(t, err)
	require.NotNil(t, paramBundle)
	assert.NotNil(t, paramBundle.Structural)
	assert.NotNil(t, paramBundle.JSONSchema)
	assert.Nil(t, envBundle)
}

func TestBuildStructuralSchemas_BothNil(t *testing.T) {
	input := &SchemaInput{
		ParametersSchema:         nil,
		EnvironmentConfigsSchema: nil,
	}

	paramBundle, envBundle, err := BuildStructuralSchemas(input)
	require.NoError(t, err)
	assert.Nil(t, paramBundle)
	assert.Nil(t, envBundle)
}

func TestBuildStructuralSchemas_InvalidSchema(t *testing.T) {
	input := &SchemaInput{
		ParametersSchema: &v1alpha1.SchemaSection{
			OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(`{"type": "object", "properties": "invalid"}`)},
		},
	}

	paramBundle, envBundle, err := BuildStructuralSchemas(input)
	require.Error(t, err)
	assert.Nil(t, paramBundle)
	assert.Nil(t, envBundle)
	assert.Contains(t, err.Error(), "parameters schema")
}
