// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// endpointInput builds a RenderInput whose ResourceType declares the given endpoints.
// At least one resources[] entry is required by the API, so a marker is always present.
func endpointInput(endpoints []v1alpha1.ResourceTypeEndpoint) *RenderInput {
	return makeRenderInput(v1alpha1.ResourceTypeSpec{
		Endpoints: endpoints,
		Resources: []v1alpha1.ResourceTypeManifest{{ID: "marker"}},
	})
}

func plainOutputs(pairs ...string) []ResolvedOutput {
	out := make([]ResolvedOutput, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, ResolvedOutput{Name: pairs[i], Value: pairs[i+1]})
	}
	return out
}

// One endpoint taking its address from plain-value host/port outputs.
func TestResolveEndpointsFromPlainOutputs(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, plainOutputs("host", "cache.ns.svc", "port", "6379"))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, ResolvedEndpoint{
		Name: "client", Host: "cache.ns.svc", Port: 6379, HostFrom: "host", PortFrom: "port",
	}, got[0])
}

// Two endpoints sharing one host output but with distinct port outputs.
func TestResolveEndpointsMultiplePorts(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
		{Name: "monitor", HostFrom: "host", PortFrom: "monitorPort"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil,
		plainOutputs("host", "nats.ns.svc", "port", "4222", "monitorPort", "8222"))
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int32(4222), got[0].Port)
	require.Equal(t, int32(8222), got[1].Port)
	require.Equal(t, got[0].Host, got[1].Host)
}

// A port output shared by two endpoints cannot be represented. The conflicting
// endpoint is dropped, the first is kept.
func TestResolveEndpointsRejectsSharedPortOutput(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "writer", HostFrom: "writerHost", PortFrom: "port"},
		{Name: "reader", HostFrom: "readerHost", PortFrom: "port"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil,
		plainOutputs("writerHost", "w.ns.svc", "readerHost", "r.ns.svc", "port", "5432"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "already used by endpoint \"writer\"")
	require.Len(t, got, 1)
	require.Equal(t, "writer", got[0].Name)
}

// Inline host/port still take precedence over the named outputs. Naming a secret-backed
// output is preferred over restating its value, but a type whose address IS knowable at
// render time may override.
func TestResolveEndpointsInlineHostPortOverride(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{{
		Name: "client", HostFrom: "host", PortFrom: "port",
		Host: "${metadata.name}.${metadata.namespace}.svc.cluster.local", Port: "6379",
	}})
	secretBacked := []ResolvedOutput{
		{Name: "host", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "conn", Key: "host"}},
		{Name: "port", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "conn", Key: "port"}},
	}
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, secretBacked)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "analytics-shared-db-dev-a1b2c3d4.dp-acme-payment-dev-x1y2z3w4.svc.cluster.local", got[0].Host)
	require.Equal(t, int32(6379), got[0].Port)
	// The role mapping survives even though neither value was readable.
	require.Equal(t, "host", got[0].HostFrom)
	require.Equal(t, "port", got[0].PortFrom)
}

// An address published only through a Secret is a valid declaration, not an error.
// The endpoint resolves with no address, while its mapping to a consumer's bindings
// survives.
func TestResolveEndpointsSecretBackedAddressIsNotAnError(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, []ResolvedOutput{
		{Name: "host", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "conn", Key: "host"}},
		{Name: "port", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "conn", Key: "port"}},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.False(t, got[0].Resolved())
	require.Empty(t, got[0].Host)
	require.Zero(t, got[0].Port)
	require.Equal(t, "host", got[0].HostFrom)
	require.Equal(t, "port", got[0].PortFrom)
}

// Half a secret-backed address is enough to leave the endpoint undialable: a readable
// host with a secret-backed port has no complete target.
func TestResolveEndpointsSecretBackedPortAlone(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, []ResolvedOutput{
		{Name: "host", Value: "cache.ns.svc.cluster.local"},
		{Name: "port", SecretKeyRef: &v1alpha1.SecretKeyRef{Name: "conn", Key: "port"}},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.False(t, got[0].Resolved())
}

// A ConfigMap-backed address is as unreadable from the control plane as a Secret-backed
// one, so it resolves to an addressless endpoint rather than failing the whole resource
// (which would take every consumer's ReleaseSynced down with it).
func TestResolveEndpointsConfigMapBackedAddressResolvesWithoutAddress(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, []ResolvedOutput{
		{Name: "host", ConfigMapKeyRef: &v1alpha1.ConfigMapKeyRef{Name: "cm", Key: "host"}},
		{Name: "port", Value: "6379"},
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.False(t, got[0].Resolved())
	// The output mapping survives, which is what lets a consumer's bindings still be
	// recognized as this endpoint's once the address can be read.
	require.Equal(t, "host", got[0].HostFrom)
	require.Equal(t, "port", got[0].PortFrom)
}

// Two endpoints may each set an inline port: the collision rule is about sharing one
// named port OUTPUT, and an inline port has no name to share. Keying the check on the
// empty string rejected the second endpoint and failed the whole resource.
func TestResolveEndpointsTwoInlinePortsDoNotCollide(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
		{Name: "monitor", Host: "mon.svc", Port: "8080"},
		{Name: "admin", Host: "admin.svc", Port: "9090"},
	})
	got, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, plainOutputs("host", "h", "port", "6379"))
	require.NoError(t, err)
	require.Len(t, got, 3)
	require.Equal(t, int32(6379), got[0].Port)
	require.Equal(t, int32(8080), got[1].Port)
	require.Equal(t, int32(9090), got[2].Port)
}

// An output that renders to nothing is reported as empty, not as the wrong kind: the
// two have different fixes, and a transient empty render (an unassigned load-balancer
// address) is not a malformed type.
func TestResolveEndpointsEmptyRenderIsReportedAsEmpty(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	_, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, plainOutputs("host", "", "port", "6379"))
	require.Error(t, err)
	require.Contains(t, err.Error(), `output "host" rendered an empty host`)
}

func TestResolveEndpointsUndeclaredOutput(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "nope", PortFrom: "port"},
	})
	_, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, plainOutputs("host", "h", "port", "6379"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "undeclared output \"nope\"")
}

func TestResolveEndpointsNonNumericPort(t *testing.T) {
	in := endpointInput([]v1alpha1.ResourceTypeEndpoint{
		{Name: "client", HostFrom: "host", PortFrom: "port"},
	})
	_, err := NewPipeline().ResolveEndpoints(context.Background(), in, nil, plainOutputs("host", "h", "port", "not-a-port"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not numeric")
}

// A resource type with nothing to dial resolves to no endpoints and no error.
func TestResolveEndpointsNoneDeclared(t *testing.T) {
	got, err := NewPipeline().ResolveEndpoints(context.Background(), endpointInput(nil), nil, plainOutputs("bucket", "assets"))
	require.NoError(t, err)
	require.Empty(t, got)
}
