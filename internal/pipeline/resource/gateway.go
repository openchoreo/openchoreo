// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import "github.com/openchoreo/openchoreo/api/v1alpha1"

// BuildDataPlaneContext extracts the dataplane CEL surface from a
// v1alpha1.DataPlane CR. Pure function, no controller-runtime imports.
// Returns a zero-valued context when dataPlane is nil so callers don't have to
// guard.
func BuildDataPlaneContext(dataPlane *v1alpha1.DataPlane) DataPlaneContext {
	if dataPlane == nil {
		return DataPlaneContext{}
	}
	dpCtx := DataPlaneContext{}
	if dataPlane.Spec.SecretStoreRef != nil {
		dpCtx.SecretStore = dataPlane.Spec.SecretStoreRef.Name
	}
	if dataPlane.Spec.ObservabilityPlaneRef != nil {
		dpCtx.ObservabilityPlaneRef = &ObservabilityPlaneRefContext{
			Kind: string(dataPlane.Spec.ObservabilityPlaneRef.Kind),
			Name: dataPlane.Spec.ObservabilityPlaneRef.Name,
		}
	}
	dpCtx.Gateway = toGatewayData(&dataPlane.Spec.Gateway)
	return dpCtx
}

// BuildEnvironmentContext extracts the per-environment CEL surface, merging
// environment-level gateway overrides with dataplane-level fallbacks at each
// leaf. Mirrors the component pipeline's mergeGatewayData semantics: ingress
// and egress merge independently; within each, external and internal endpoints
// fall back independently. Returns a zero-valued context (no gateway) when
// both inputs are nil.
func BuildEnvironmentContext(env *v1alpha1.Environment, dataPlane *v1alpha1.DataPlane) EnvironmentContext {
	var envGW, dpGW *v1alpha1.GatewaySpec
	if env != nil {
		envGW = &env.Spec.Gateway
	}
	if dataPlane != nil {
		dpGW = &dataPlane.Spec.Gateway
	}
	return EnvironmentContext{
		Gateway: mergeGatewayData(envGW, dpGW),
	}
}

// toGatewayData converts a v1alpha1.GatewaySpec into the GatewayData shape used
// in CEL templates. Returns nil when neither ingress nor egress is set so
// templates can use has(...) guards.
func toGatewayData(gw *v1alpha1.GatewaySpec) *GatewayData {
	if gw == nil {
		return nil
	}
	data := &GatewayData{}
	if gw.Ingress != nil {
		data.Ingress = toGatewayNetworkData(gw.Ingress)
	}
	if gw.Egress != nil {
		data.Egress = toGatewayNetworkData(gw.Egress)
	}
	if data.Ingress == nil && data.Egress == nil {
		return nil
	}
	return data
}

func toGatewayNetworkData(t *v1alpha1.GatewayNetworkSpec) *GatewayNetworkData {
	if t == nil {
		return nil
	}
	data := &GatewayNetworkData{}
	if t.External != nil {
		data.External = toGatewayEndpointData(t.External)
	}
	if t.Internal != nil {
		data.Internal = toGatewayEndpointData(t.Internal)
	}
	if data.External == nil && data.Internal == nil {
		return nil
	}
	return data
}

func toGatewayEndpointData(e *v1alpha1.GatewayEndpointSpec) *GatewayEndpointData {
	if e == nil {
		return nil
	}
	data := &GatewayEndpointData{
		Name:      e.Name,
		Namespace: e.Namespace,
	}
	if e.HTTP != nil {
		data.HTTP = toGatewayListenerData(e.HTTP)
	}
	if e.HTTPS != nil {
		data.HTTPS = toGatewayListenerData(e.HTTPS)
	}
	if e.TLS != nil {
		data.TLS = toGatewayListenerData(e.TLS)
	}
	return data
}

func toGatewayListenerData(l *v1alpha1.GatewayListenerSpec) *GatewayListenerData {
	if l == nil {
		return nil
	}
	return &GatewayListenerData{
		ListenerName: l.ListenerName,
		Port:         l.Port,
		Host:         l.Host,
	}
}

// mergeGatewayData merges environment-level and dataplane-level gateway specs.
// Ingress and egress are merged independently, so specifying only one at the
// environment level still falls back to the dataplane value for the other.
func mergeGatewayData(envGW, dpGW *v1alpha1.GatewaySpec) *GatewayData {
	var envIngress, dpIngress *v1alpha1.GatewayNetworkSpec
	var envEgress, dpEgress *v1alpha1.GatewayNetworkSpec

	if envGW != nil {
		envIngress = envGW.Ingress
		envEgress = envGW.Egress
	}
	if dpGW != nil {
		dpIngress = dpGW.Ingress
		dpEgress = dpGW.Egress
	}

	data := &GatewayData{
		Ingress: mergeGatewayNetworkData(envIngress, dpIngress),
		Egress:  mergeGatewayNetworkData(envEgress, dpEgress),
	}
	if data.Ingress == nil && data.Egress == nil {
		return nil
	}
	return data
}

// mergeGatewayNetworkData merges environment-level and dataplane-level gateway
// network specs. External and internal endpoints are merged independently,
// preferring environment values and falling back to dataplane values for any
// unspecified endpoint.
func mergeGatewayNetworkData(envNetwork, dpNetwork *v1alpha1.GatewayNetworkSpec) *GatewayNetworkData {
	if envNetwork == nil && dpNetwork == nil {
		return nil
	}

	var envExternal, dpExternal *v1alpha1.GatewayEndpointSpec
	var envInternal, dpInternal *v1alpha1.GatewayEndpointSpec

	if envNetwork != nil {
		envExternal = envNetwork.External
		envInternal = envNetwork.Internal
	}
	if dpNetwork != nil {
		dpExternal = dpNetwork.External
		dpInternal = dpNetwork.Internal
	}

	external := envExternal
	if external == nil {
		external = dpExternal
	}
	internal := envInternal
	if internal == nil {
		internal = dpInternal
	}

	data := &GatewayNetworkData{
		External: toGatewayEndpointData(external),
		Internal: toGatewayEndpointData(internal),
	}
	if data.External == nil && data.Internal == nil {
		return nil
	}
	return data
}
