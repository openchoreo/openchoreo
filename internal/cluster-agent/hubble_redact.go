// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusteragent

import (
	"strings"

	"github.com/cilium/cilium/api/v1/flow"
	"github.com/cilium/cilium/api/v1/observer"

	"github.com/openchoreo/openchoreo/internal/labels"
)

// wirelogsScope is the caller-supplied scope (parsed from the gateway query)
// used to classify each endpoint as in-scope (the tenant's own workloads) or
// out-of-scope (peers — typically platform infra such as the otel-collector,
// control-plane services, or sidecars in other tenants' namespaces).
type wirelogsScope struct {
	environment string
	namespace   string
	project     string
	component   string
}

const k8sLabelPrefix = "k8s:"

// redactFlowResponse mutates resp in place to strip fields that are not safe or
// useful to forward to tenants. Returns false when the response should be
// suppressed entirely (e.g. hubble-relay NodeStatusEvent heartbeats, which
// leak the host node name and carry no flow payload).
//
// Filtering at the agent — rather than at the gateway or openchoreo-api — saves
// bandwidth on both downstream hops and is the only point where the raw
// flow.Flow proto is still in memory before protojson encoding.
func redactFlowResponse(resp *observer.GetFlowsResponse, scope wirelogsScope) bool {
	if resp == nil {
		return false
	}
	if resp.GetNodeStatus() != nil {
		return false
	}
	resp.NodeName = ""

	f := resp.GetFlow()
	if f == nil {
		return true
	}

	f.NodeName = ""
	f.NodeLabels = nil
	f.Ethernet = nil
	f.Interface = nil
	f.EventType = nil
	f.TraceObservationPoint = 0
	f.TraceReason = 0
	f.Reply = false //nolint:staticcheck // deprecated mirror of IsReply; clearing it is the point

	redactEndpoint(f.Source, scope)
	redactEndpoint(f.Destination, scope)
	return true
}

func redactEndpoint(ep *flow.Endpoint, scope wirelogsScope) {
	if ep == nil {
		return
	}
	ep.ID = 0
	ep.Identity = 0
	inScope := endpointInScope(ep, scope)
	ep.Labels = filterLabels(ep.Labels, inScope)
	if !inScope {
		ep.Workloads = nil
	}
}

// endpointInScope returns true when the endpoint carries every openchoreo label
// the caller requested. We match by label (not by k8s namespace) because the
// hubble whitelist already does, and DP-side pods live in derived
// `dp-<...>` namespaces that don't match the openchoreo namespace name.
func endpointInScope(ep *flow.Endpoint, scope wirelogsScope) bool {
	if ep == nil {
		return false
	}
	want := map[string]string{
		labels.LabelKeyNamespaceName:   scope.namespace,
		labels.LabelKeyEnvironmentName: scope.environment,
	}
	if scope.project != "" {
		want[labels.LabelKeyProjectName] = scope.project
	}
	if scope.component != "" {
		want[labels.LabelKeyComponentName] = scope.component
	}
	matched := 0
	for _, l := range ep.Labels {
		k, v, ok := parseLabel(l)
		if !ok {
			continue
		}
		if w, needs := want[k]; needs && w == v {
			matched++
		}
	}
	return matched == len(want)
}

func filterLabels(in []string, inScope bool) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0]
	for _, l := range in {
		if strings.HasPrefix(l, k8sLabelPrefix+"io.cilium.") {
			continue
		}
		if strings.Contains(l, "-uid=") {
			continue
		}
		if inScope {
			out = append(out, l)
			continue
		}
		if strings.HasPrefix(l, k8sLabelPrefix+"io.kubernetes.pod.namespace=") {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parseLabel splits a hubble label like "k8s:openchoreo.dev/namespace=default"
// into ("openchoreo.dev/namespace", "default", true). The "k8s:" source prefix
// is dropped; entries without an "=" return ok=false.
func parseLabel(l string) (key, value string, ok bool) {
	l = strings.TrimPrefix(l, k8sLabelPrefix)
	eq := strings.IndexByte(l, '=')
	if eq < 0 {
		return "", "", false
	}
	return l[:eq], l[eq+1:], true
}
