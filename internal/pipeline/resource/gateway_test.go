// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// gatewayWithHost is a small fixture builder: builds a v1alpha1.GatewaySpec
// with a single ingress / external / HTTPS listener carrying host. Used by
// the merge tests below.
func gatewayWithHost(name, host string) v1alpha1.GatewaySpec {
	return v1alpha1.GatewaySpec{
		Ingress: &v1alpha1.GatewayNetworkSpec{
			External: &v1alpha1.GatewayEndpointSpec{
				Name:      name,
				Namespace: "openchoreo-system",
				HTTPS: &v1alpha1.GatewayListenerSpec{
					ListenerName: "https",
					Port:         443,
					Host:         host,
				},
			},
		},
	}
}

func TestBuildDataPlaneContext(t *testing.T) {
	t.Run("returns_empty_when_dataplane_is_nil", func(t *testing.T) {
		got := BuildDataPlaneContext(nil)
		require.Empty(t, got.SecretStore)
		require.Nil(t, got.Gateway)
		require.Nil(t, got.ObservabilityPlaneRef)
	})

	t.Run("populates_secret_store_and_observability_plane_ref", func(t *testing.T) {
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				SecretStoreRef: &v1alpha1.SecretStoreRef{Name: "kind-cluster-store"},
				ObservabilityPlaneRef: &v1alpha1.ObservabilityPlaneRef{
					Kind: "ObservabilityPlane",
					Name: "primary-obs",
				},
			},
		}

		got := BuildDataPlaneContext(dp)
		require.Equal(t, "kind-cluster-store", got.SecretStore)
		require.NotNil(t, got.ObservabilityPlaneRef)
		require.Equal(t, "ObservabilityPlane", got.ObservabilityPlaneRef.Kind)
		require.Equal(t, "primary-obs", got.ObservabilityPlaneRef.Name)
	})

	t.Run("populates_gateway_from_dataplane_spec", func(t *testing.T) {
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				Gateway: gatewayWithHost("dp-gateway", "*.dp.example.com"),
			},
		}

		got := BuildDataPlaneContext(dp)
		require.NotNil(t, got.Gateway)
		require.NotNil(t, got.Gateway.Ingress)
		require.NotNil(t, got.Gateway.Ingress.External)
		require.NotNil(t, got.Gateway.Ingress.External.HTTPS)
		require.Equal(t, "*.dp.example.com", got.Gateway.Ingress.External.HTTPS.Host)
		require.EqualValues(t, 443, got.Gateway.Ingress.External.HTTPS.Port)
		require.Equal(t, "dp-gateway", got.Gateway.Ingress.External.Name)
	})

	t.Run("gateway_nil_when_dataplane_has_no_gateway", func(t *testing.T) {
		// A DataPlane with no ingress and no egress yields a nil Gateway so
		// templates can guard with has(dataplane.gateway).
		dp := &v1alpha1.DataPlane{Spec: v1alpha1.DataPlaneSpec{}}
		got := BuildDataPlaneContext(dp)
		require.Nil(t, got.Gateway)
	})
}

func TestBuildEnvironmentContext(t *testing.T) {
	t.Run("returns_empty_when_both_inputs_nil", func(t *testing.T) {
		got := BuildEnvironmentContext(nil, nil)
		require.Nil(t, got.Gateway)
	})

	t.Run("uses_dataplane_gateway_when_env_unset", func(t *testing.T) {
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				Gateway: gatewayWithHost("dp-gateway", "*.dp.example.com"),
			},
		}
		env := &v1alpha1.Environment{}

		got := BuildEnvironmentContext(env, dp)
		require.NotNil(t, got.Gateway)
		require.Equal(t, "*.dp.example.com", got.Gateway.Ingress.External.HTTPS.Host)
	})

	t.Run("env_gateway_overrides_dataplane_at_external_dimension", func(t *testing.T) {
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				Gateway: gatewayWithHost("dp-gateway", "*.dp.example.com"),
			},
		}
		env := &v1alpha1.Environment{
			Spec: v1alpha1.EnvironmentSpec{
				Gateway: gatewayWithHost("env-gateway", "*.dev.example.com"),
			},
		}

		got := BuildEnvironmentContext(env, dp)
		require.NotNil(t, got.Gateway)
		require.Equal(t, "*.dev.example.com", got.Gateway.Ingress.External.HTTPS.Host)
		require.Equal(t, "env-gateway", got.Gateway.Ingress.External.Name)
	})

	t.Run("env_internal_falls_back_to_dataplane_when_env_only_sets_external", func(t *testing.T) {
		// Locks the merge granularity: when env sets only external, dp's
		// internal must come through. Mirrors mergeGatewayNetworkData's
		// per-endpoint fallback.
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				Gateway: v1alpha1.GatewaySpec{
					Ingress: &v1alpha1.GatewayNetworkSpec{
						External: &v1alpha1.GatewayEndpointSpec{
							Name: "dp-external",
							HTTPS: &v1alpha1.GatewayListenerSpec{
								Port: 443,
								Host: "*.dp.example.com",
							},
						},
						Internal: &v1alpha1.GatewayEndpointSpec{
							Name: "dp-internal",
							HTTP: &v1alpha1.GatewayListenerSpec{
								Port: 80,
								Host: "internal.dp.svc",
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
							Name: "env-external",
							HTTPS: &v1alpha1.GatewayListenerSpec{
								Port: 443,
								Host: "*.dev.example.com",
							},
						},
					},
				},
			},
		}

		got := BuildEnvironmentContext(env, dp)
		require.NotNil(t, got.Gateway)
		require.NotNil(t, got.Gateway.Ingress)
		require.NotNil(t, got.Gateway.Ingress.External)
		require.Equal(t, "*.dev.example.com", got.Gateway.Ingress.External.HTTPS.Host)
		require.Equal(t, "env-external", got.Gateway.Ingress.External.Name)
		require.NotNil(t, got.Gateway.Ingress.Internal)
		require.Equal(t, "internal.dp.svc", got.Gateway.Ingress.Internal.HTTP.Host)
		require.Equal(t, "dp-internal", got.Gateway.Ingress.Internal.Name)
	})

	t.Run("env_egress_falls_back_to_dataplane_when_env_only_sets_ingress", func(t *testing.T) {
		// Top-level ingress/egress merge independently: env-only ingress
		// must not erase dp-side egress.
		dp := &v1alpha1.DataPlane{
			Spec: v1alpha1.DataPlaneSpec{
				Gateway: v1alpha1.GatewaySpec{
					Ingress: &v1alpha1.GatewayNetworkSpec{
						External: &v1alpha1.GatewayEndpointSpec{
							Name: "dp-ingress",
						},
					},
					Egress: &v1alpha1.GatewayNetworkSpec{
						External: &v1alpha1.GatewayEndpointSpec{
							Name: "dp-egress",
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
							Name: "env-ingress",
						},
					},
				},
			},
		}

		got := BuildEnvironmentContext(env, dp)
		require.NotNil(t, got.Gateway)
		require.NotNil(t, got.Gateway.Ingress)
		require.Equal(t, "env-ingress", got.Gateway.Ingress.External.Name)
		require.NotNil(t, got.Gateway.Egress)
		require.Equal(t, "dp-egress", got.Gateway.Egress.External.Name)
	})
}
