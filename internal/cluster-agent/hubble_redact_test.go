// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"testing"

	"github.com/cilium/cilium/api/v1/flow"
	"github.com/cilium/cilium/api/v1/observer"
	relaypb "github.com/cilium/cilium/api/v1/relay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func tenantScope() wirelogsScope {
	return wirelogsScope{
		environment: "development",
		namespace:   "default",
		project:     "url-shortener",
		component:   "snip-api-service",
	}
}

// sampleFlow mirrors the shape of the live SSE events from hubble-relay: a
// platform-infra source (otel-collector) talking to a tenant destination.
func sampleFlow() *observer.GetFlowsResponse {
	return &observer.GetFlowsResponse{
		NodeName: "lima-rancher-desktop",
		ResponseTypes: &observer.GetFlowsResponse_Flow{
			Flow: &flow.Flow{
				Uuid:    "5728f17b-5b61-4210-9ea3-fd3c7bd5f18c",
				Verdict: flow.Verdict_FORWARDED,
				Ethernet: &flow.Ethernet{
					Source:      "4e:e6:10:a7:19:8e",
					Destination: "86:bb:98:4a:de:a9",
				},
				NodeName:              "lima-rancher-desktop",
				NodeLabels:            []string{"kubernetes.io/arch=arm64", "node-role.kubernetes.io/control-plane=true"},
				EventType:             &flow.CiliumEventType{Type: 4},
				TraceObservationPoint: flow.TraceObservationPoint_TO_ENDPOINT,
				TraceReason:           flow.TraceReason_REPLY,
				Reply:                 true,
				IsReply:               wrapperspb.Bool(true),
				Interface: &flow.NetworkInterface{
					Index: 225,
					Name:  "lxc6707a6f14d2f",
				},
				Source: &flow.Endpoint{
					ID:        666,
					Identity:  12476,
					Namespace: "openchoreo-observability-plane",
					PodName:   "opentelemetry-collector-74864bd658-z5nk6",
					Labels: []string{
						"k8s:app.kubernetes.io/instance=observability-traces-opensearch",
						"k8s:app.kubernetes.io/name=opentelemetry-collector",
						"k8s:io.cilium.k8s.namespace.labels.kubernetes.io/metadata.name=openchoreo-observability-plane",
						"k8s:io.cilium.k8s.policy.cluster=default",
						"k8s:io.cilium.k8s.policy.serviceaccount=opentelemetry-collector",
						"k8s:io.kubernetes.pod.namespace=openchoreo-observability-plane",
					},
					Workloads: []*flow.Workload{{Name: "opentelemetry-collector", Kind: "Deployment"}},
				},
				Destination: &flow.Endpoint{
					ID:        1154,
					Identity:  40143,
					Namespace: "dp-default-url-shortener-development-a16f373a",
					PodName:   "snip-api-service-development-53bed701-55687d7cd7-8zr7t",
					Labels: []string{
						"k8s:io.cilium.k8s.policy.cluster=default",
						"k8s:io.cilium.k8s.policy.serviceaccount=default",
						"k8s:io.kubernetes.pod.namespace=dp-default-url-shortener-development-a16f373a",
						"k8s:openchoreo.dev/component-uid=43f3d9e0-15c3-4599-aa9a-34710da07e6e",
						"k8s:openchoreo.dev/component=snip-api-service",
						"k8s:openchoreo.dev/environment-uid=cb6b3d47-f636-4e2d-aaa3-1b2b70283401",
						"k8s:openchoreo.dev/environment=development",
						"k8s:openchoreo.dev/namespace=default",
						"k8s:openchoreo.dev/project-uid=a0cf72ba-6573-4015-b8e3-f526e290a260",
						"k8s:openchoreo.dev/project=url-shortener",
					},
					Workloads: []*flow.Workload{{Name: "snip-api-service-development-53bed701", Kind: "Deployment"}},
				},
			},
		},
	}
}

func TestRedactFlowResponse_StripsHostAndCiliumInternals(t *testing.T) {
	resp := sampleFlow()
	require.True(t, redactFlowResponse(resp, tenantScope()))

	assert.Empty(t, resp.NodeName, "envelope node_name leaks the host identity")

	f := resp.GetFlow()
	require.NotNil(t, f)
	assert.Empty(t, f.NodeName)
	assert.Nil(t, f.NodeLabels)
	assert.Nil(t, f.Ethernet, "MAC addresses are pure noise to tenants")
	assert.Nil(t, f.Interface, "veth interface name leaks cilium internals")
	assert.Nil(t, f.EventType)
	assert.Equal(t, flow.TraceObservationPoint_UNKNOWN_POINT, f.TraceObservationPoint)
	assert.Equal(t, flow.TraceReason_TRACE_REASON_UNKNOWN, f.TraceReason)
	assert.False(t, f.Reply, "deprecated Reply field — IsReply is the source of truth") //nolint:staticcheck
	assert.NotNil(t, f.IsReply, "IsReply must survive: it carries reply direction")

	for _, ep := range []*flow.Endpoint{f.Source, f.Destination} {
		assert.Zero(t, ep.ID, "cilium endpoint ID is a cluster-internal handle")
		assert.Zero(t, ep.Identity, "cilium identity number is a cluster-internal handle")
	}
}

func TestRedactFlowResponse_InScopePeerKeepsOpenChoreoLabels(t *testing.T) {
	resp := sampleFlow()
	require.True(t, redactFlowResponse(resp, tenantScope()))

	dst := resp.GetFlow().GetDestination()
	require.NotNil(t, dst)
	assert.Equal(t, "snip-api-service-development-53bed701-55687d7cd7-8zr7t", dst.PodName)
	assert.NotEmpty(t, dst.Workloads, "tenant's own workload metadata must remain")

	for _, l := range dst.Labels {
		assert.NotContains(t, l, "io.cilium.", "all cilium labels must be stripped")
		assert.NotContains(t, l, "-uid=", "object UIDs are noise; not for the wire")
	}
	assert.Contains(t, dst.Labels, "k8s:openchoreo.dev/project=url-shortener")
	assert.Contains(t, dst.Labels, "k8s:openchoreo.dev/component=snip-api-service")
	assert.Contains(t, dst.Labels, "k8s:openchoreo.dev/environment=development")
}

func TestRedactFlowResponse_OutOfScopePeerCollapsesLabels(t *testing.T) {
	resp := sampleFlow()
	require.True(t, redactFlowResponse(resp, tenantScope()))

	// The otel-collector source is platform infra, not in the tenant's scope.
	// It must retain pod_name+namespace (so the tenant can see what they're
	// talking to) but lose SA, instance name, and workload kind — those would
	// leak platform implementation details.
	src := resp.GetFlow().GetSource()
	require.NotNil(t, src)
	assert.Equal(t, "opentelemetry-collector-74864bd658-z5nk6", src.PodName)
	assert.Equal(t, "openchoreo-observability-plane", src.Namespace)
	assert.Nil(t, src.Workloads, "peer workload kind/name not exposed when out-of-scope")

	assert.Equal(t,
		[]string{"k8s:io.kubernetes.pod.namespace=openchoreo-observability-plane"},
		src.Labels,
		"out-of-scope peer keeps only its k8s namespace; SA + Helm labels stripped")
}

func TestRedactFlowResponse_SuppressesNodeStatusHeartbeat(t *testing.T) {
	// hubble-relay emits these on connect/disconnect; they leak node_names and
	// carry no flow payload — drop entirely so they never reach the gateway.
	resp := &observer.GetFlowsResponse{
		NodeName: "lima-rancher-desktop",
		ResponseTypes: &observer.GetFlowsResponse_NodeStatus{
			NodeStatus: &relaypb.NodeStatusEvent{
				StateChange: relaypb.NodeState_NODE_CONNECTED,
				NodeNames:   []string{"lima-rancher-desktop"},
			},
		},
	}
	assert.False(t, redactFlowResponse(resp, tenantScope()))
}

func TestRedactFlowResponse_NilSafe(t *testing.T) {
	assert.False(t, redactFlowResponse(nil, tenantScope()))
}

func TestEndpointInScope_EnvironmentWide(t *testing.T) {
	// Scope without project/component must accept any endpoint matching
	// namespace+environment — used when callers tail a whole environment.
	scope := wirelogsScope{environment: "development", namespace: "default"}
	ep := &flow.Endpoint{Labels: []string{
		"k8s:openchoreo.dev/namespace=default",
		"k8s:openchoreo.dev/environment=development",
		"k8s:openchoreo.dev/project=url-shortener",
	}}
	assert.True(t, endpointInScope(ep, scope))
}

func TestEndpointInScope_ProjectMismatchRejects(t *testing.T) {
	scope := tenantScope()
	ep := &flow.Endpoint{Labels: []string{
		"k8s:openchoreo.dev/namespace=default",
		"k8s:openchoreo.dev/environment=development",
		"k8s:openchoreo.dev/project=other-project",
		"k8s:openchoreo.dev/component=snip-api-service",
	}}
	assert.False(t, endpointInScope(ep, scope),
		"a project mismatch is enough to mark the peer out-of-scope even if other labels line up")
}

func TestEndpointInScope_DuplicateLabelsDoNotInflateMatch(t *testing.T) {
	// Regression: duplicate matching labels must not be counted twice.
	// Here only two of the four required keys are actually present, but each
	// appears twice — naive counting would treat it as a full match.
	scope := tenantScope()
	ep := &flow.Endpoint{Labels: []string{
		"k8s:openchoreo.dev/namespace=default",
		"k8s:openchoreo.dev/namespace=default",
		"k8s:openchoreo.dev/environment=development",
		"k8s:openchoreo.dev/environment=development",
	}}
	assert.False(t, endpointInScope(ep, scope),
		"missing project+component must still mark endpoint out-of-scope despite duplicate matches on namespace/environment")
}

func TestFilterLabels_DropsCiliumAndUIDsForInScope(t *testing.T) {
	got := filterLabels([]string{
		"k8s:io.cilium.k8s.policy.cluster=default",
		"k8s:openchoreo.dev/project=url-shortener",
		"k8s:openchoreo.dev/project-uid=abc-123",
		"k8s:app.kubernetes.io/name=snip",
	}, true)
	assert.ElementsMatch(t, []string{
		"k8s:openchoreo.dev/project=url-shortener",
		"k8s:app.kubernetes.io/name=snip",
	}, got)
}

func TestFilterLabels_OutOfScopeKeepsOnlyNamespace(t *testing.T) {
	got := filterLabels([]string{
		"k8s:io.cilium.k8s.policy.serviceaccount=opentelemetry-collector",
		"k8s:app.kubernetes.io/instance=observability-traces-opensearch",
		"k8s:io.kubernetes.pod.namespace=openchoreo-observability-plane",
		"k8s:openchoreo.dev/namespace=openchoreo-observability-plane",
	}, false)
	assert.Equal(t, []string{"k8s:io.kubernetes.pod.namespace=openchoreo-observability-plane"}, got)
}

func TestParseLabel(t *testing.T) {
	k, v, ok := parseLabel("k8s:openchoreo.dev/project=url-shortener")
	require.True(t, ok)
	assert.Equal(t, "openchoreo.dev/project", k)
	assert.Equal(t, "url-shortener", v)

	_, _, ok = parseLabel("no-equals-sign")
	assert.False(t, ok)
}
