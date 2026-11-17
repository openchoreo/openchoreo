// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package networkpolicy

import (
	"fmt"
	"sort"
	"strconv"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Provider selects which network policy API to use when generating component policies.
type Provider string

const (
	// ProviderKubernetes generates standard Kubernetes NetworkPolicy objects (default).
	ProviderKubernetes Provider = "kubernetes"
	// ProviderCilium generates CiliumNetworkPolicy objects with L7 HTTP rules for
	// HTTP-proxied endpoints, enabling Hubble flow metrics via Envoy.
	ProviderCilium Provider = "cilium"

	// ProviderAnnotation on a DataPlane or ClusterDataPlane selects the
	// network policy provider for everything rendered onto it.
	ProviderAnnotation = "openchoreo.dev/networkpolicyprovider"

	// KubernetesNamespaceKey is the key for Kubernetes namespace
	// used in CiliumNetworkPolicy to allow rules based on namespace.
	KubernetesNamespaceKey = "k8s:io.kubernetes.pod.namespace"
)

// ComponentPolicyParams holds parameters for generating per-component NetworkPolicies
// with ingress rules based on endpoint visibility.
type ComponentPolicyParams struct {
	Namespace     string                                         // data plane namespace name
	CPNamespace   string                                         // control plane namespace name
	Environment   string                                         // environment name (e.g., "development")
	ComponentName string                                         // for naming the policy
	PodSelectors  map[string]string                              // platform pod selectors
	Endpoints     map[string]openchoreov1alpha1.WorkloadEndpoint // from workload spec
	Provider      Provider                                       // network policy provider
}

// EndpointPolicyParams holds parameters for the policy an Endpoint renders for
// one of its backends. It admits the Endpoint's visibility to the backend's
// pods on the backend's port, alongside the backend component's own policy:
// policies are additive, so the component's policy keeps its project scope and
// whatever its deprecated workload visibility requests.
type EndpointPolicyParams struct {
	Name         string                                  // policy name
	Namespace    string                                  // data plane namespace name
	CPNamespace  string                                  // control plane namespace name
	Environment  string                                  // environment name (e.g., "development")
	PodSelectors map[string]string                       // the backend's pod selectors
	Port         int32                                   // the backend's container port
	Protocol     openchoreov1alpha1.EndpointProtocol     // the backend's protocol
	Visibility   []openchoreov1alpha1.EndpointVisibility // the Endpoint's visibility
	Provider     Provider                                // network policy provider
}

// MakeComponentPolicies returns a policy for a component with ingress rules based on
// declared endpoint visibility. Egress is unrestricted.
// When Provider is ProviderCilium a CiliumNetworkPolicy is returned with L7 HTTP rules
// for HTTP-proxied endpoints; otherwise a standard Kubernetes NetworkPolicy is returned.
func MakeComponentPolicies(params ComponentPolicyParams) []map[string]any {
	if params.Provider == ProviderCilium {
		return makeCiliumComponentPolicies(params)
	}

	// Build ingress rules from endpoints
	ingressRules := makeIngressRules(params)

	// Generate a policy name, truncated to k8s limits
	policyName := fmt.Sprintf("openchoreo-%s", params.ComponentName)
	if len(policyName) > dpkubernetes.MaxResourceNameLength {
		policyName = dpkubernetes.GenerateK8sNameWithLengthLimit(
			dpkubernetes.MaxResourceNameLength,
			"openchoreo", params.ComponentName,
		)
	}

	spec := map[string]any{
		"podSelector": map[string]any{
			"matchLabels": toAnyMap(params.PodSelectors),
		},
		"policyTypes": []any{"Ingress"},
	}
	if len(ingressRules) > 0 {
		spec["ingress"] = ingressRules
	}

	policy := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]any{
			"name":      policyName,
			"namespace": params.Namespace,
		},
		"spec": spec,
	}

	return []map[string]any{policy}
}

// makeIngressRules builds ingress rules from component endpoints.
func makeIngressRules(params ComponentPolicyParams) []any {
	if len(params.Endpoints) == 0 {
		return nil
	}

	// Collect endpoints into a slice and sort by name to ensure deterministic
	// output. Without this, map iteration order causes the serialized
	// NetworkPolicy to differ between reconcile cycles, triggering unnecessary
	// updates.
	type epEntry struct {
		name       string
		port       map[string]any
		visibility []openchoreov1alpha1.EndpointVisibility
	}
	entries := make([]epEntry, 0, len(params.Endpoints))
	for name, ep := range params.Endpoints {
		entries = append(entries, epEntry{
			name: name,
			port: map[string]any{
				"protocol": l4Protocol(ep.EffectiveProtocol()),
				"port":     int64(ep.Port),
			},
			visibility: ep.Visibility, //nolint:staticcheck // deprecated field still supported for backward compatibility
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	// Group ports by visibility level.
	// Every endpoint implicitly has project visibility (intra-namespace).
	// We collect additional visibility levels to build extra ingress rules.
	projectPorts := make([]any, 0, len(entries))
	var namespacePorts []any
	var broadAccessPorts []any

	for _, e := range entries {
		projectPorts = append(projectPorts, e.port)

		for _, vis := range e.visibility {
			switch vis {
			case openchoreov1alpha1.EndpointVisibilityNamespace:
				namespacePorts = append(namespacePorts, e.port)
			case openchoreov1alpha1.EndpointVisibilityInternal, openchoreov1alpha1.EndpointVisibilityExternal:
				broadAccessPorts = append(broadAccessPorts, e.port)
			}
		}
	}

	var ingressRules []any

	// Rule 1: intra-namespace (project visibility) — all declared ports
	if len(projectPorts) > 0 {
		ingressRules = append(ingressRules, map[string]any{
			"from": []any{
				map[string]any{
					"podSelector": map[string]any{},
				},
			},
			"ports": projectPorts,
		})
	}

	// Rule 2: cross-project, same CP namespace and same environment (namespace visibility)
	if len(namespacePorts) > 0 {
		ingressRules = append(ingressRules, namespaceIngressRule(params.CPNamespace, params.Environment, namespacePorts))
	}

	// Rule 3: system components (e.g., gateway) from any namespace (internal or external visibility)
	if len(broadAccessPorts) > 0 {
		ingressRules = append(ingressRules, systemComponentIngressRule(broadAccessPorts))
	}

	return ingressRules
}

// namespaceIngressRule admits pods of other projects in the same control plane
// namespace and environment (namespace visibility).
func namespaceIngressRule(cpNamespace, environment string, ports []any) map[string]any {
	return map[string]any{
		"from": []any{
			map[string]any{
				"namespaceSelector": map[string]any{
					"matchLabels": map[string]any{
						labels.LabelKeyNamespaceName:   cpNamespace,
						labels.LabelKeyEnvironmentName: environment,
					},
				},
			},
		},
		"ports": ports,
	}
}

// systemComponentIngressRule admits system components such as the gateways,
// from any namespace (internal or external visibility).
func systemComponentIngressRule(ports []any) map[string]any {
	return map[string]any{
		"from": []any{
			map[string]any{
				"namespaceSelector": map[string]any{},
				"podSelector": map[string]any{
					"matchExpressions": []any{
						map[string]any{
							"key":      labels.LabelKeySystemComponent,
							"operator": "Exists",
						},
					},
				},
			},
		},
		"ports": ports,
	}
}

// ciliumPortEntry holds per-endpoint data for Cilium CNP generation.
type ciliumPortEntry struct {
	port       string
	proto      string
	isL7       bool
	visibility []openchoreov1alpha1.EndpointVisibility
}

// makeCiliumComponentPolicies returns a CiliumNetworkPolicy for a component.
// HTTP-proxied endpoint types (HTTP, GraphQL, Websocket, gRPC) get L7 HTTP rules
// so Cilium redirects traffic through Envoy and Hubble emits flow metrics.
// Non-proxied types (TCP, UDP) get L4-only rules.
func makeCiliumComponentPolicies(params ComponentPolicyParams) []map[string]any {
	entries := make([]ciliumPortEntry, 0, len(params.Endpoints))
	for _, ep := range params.Endpoints {
		entries = append(entries, ciliumPortEntry{
			port:       strconv.Itoa(int(ep.Port)),
			proto:      l4Protocol(ep.EffectiveProtocol()),
			isL7:       isL7Proxied(ep.EffectiveProtocol()),
			visibility: ep.Visibility, //nolint:staticcheck // deprecated field still supported for backward compatibility
		})
	}
	// Sort by port for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].port < entries[j].port
	})

	projectPorts := make([]ciliumPortEntry, 0, len(entries))
	var namespacePorts, broadPorts []ciliumPortEntry
	for _, e := range entries {
		projectPorts = append(projectPorts, e)
		for _, vis := range e.visibility {
			switch vis {
			case openchoreov1alpha1.EndpointVisibilityNamespace:
				namespacePorts = append(namespacePorts, e)
			case openchoreov1alpha1.EndpointVisibilityInternal, openchoreov1alpha1.EndpointVisibilityExternal:
				broadPorts = append(broadPorts, e)
			}
		}
	}

	ingressRules := make([]any, 0)

	// Rule 1: intra-namespace (project visibility) — all pods in the data-plane namespace
	if len(projectPorts) > 0 {
		ingressRules = append(ingressRules, map[string]any{
			"fromEndpoints": []any{
				map[string]any{},
			},
			"toPorts": ciliumToPorts(projectPorts),
		})
	}

	// Rule 2: cross-project, same CP namespace and environment (namespace visibility)
	if len(namespacePorts) > 0 {
		ingressRules = append(ingressRules, ciliumNamespaceIngressRule(params.CPNamespace, params.Environment, namespacePorts))
	}

	// Rule 3: system components (e.g., gateway) from any namespace (internal or external visibility)
	if len(broadPorts) > 0 {
		ingressRules = append(ingressRules, ciliumSystemComponentIngressRule(broadPorts))
	}

	// No endpoints: emit a single empty ingress rule. Cilium rejects a rule with
	// no ingress/egress sections, while an empty rule enables ingress default-deny
	// without allowing anything, matching the Kubernetes provider's deny-all.
	if len(ingressRules) == 0 {
		ingressRules = append(ingressRules, map[string]any{})
	}

	policyName := fmt.Sprintf("openchoreo-%s", params.ComponentName)
	if len(policyName) > dpkubernetes.MaxResourceNameLength {
		policyName = dpkubernetes.GenerateK8sNameWithLengthLimit(
			dpkubernetes.MaxResourceNameLength,
			"openchoreo", params.ComponentName,
		)
	}

	spec := map[string]any{
		"endpointSelector": map[string]any{
			"matchLabels": toAnyMap(params.PodSelectors),
		},
		"ingress": ingressRules,
	}

	return []map[string]any{{
		"apiVersion": "cilium.io/v2",
		"kind":       "CiliumNetworkPolicy",
		"metadata": map[string]any{
			"name":      policyName,
			"namespace": params.Namespace,
		},
		"spec": spec,
	}}
}

// ciliumNamespaceIngressRule is namespaceIngressRule for Cilium.
func ciliumNamespaceIngressRule(cpNamespace, environment string, entries []ciliumPortEntry) map[string]any {
	return map[string]any{
		"fromEndpoints": []any{
			map[string]any{
				"matchLabels": map[string]any{
					labels.LabelKeyNamespaceName:   cpNamespace,
					labels.LabelKeyEnvironmentName: environment,
				},
				"matchExpressions": []any{
					map[string]any{ // Explicitly allow from any namespace
						"key":      KubernetesNamespaceKey,
						"operator": "Exists",
					},
				},
			},
		},
		"toPorts": ciliumToPorts(entries),
	}
}

// ciliumSystemComponentIngressRule is systemComponentIngressRule for Cilium.
func ciliumSystemComponentIngressRule(entries []ciliumPortEntry) map[string]any {
	return map[string]any{
		"fromEndpoints": []any{
			map[string]any{"matchExpressions": []any{
				map[string]any{
					"key":      labels.LabelKeySystemComponent,
					"operator": "Exists",
				},
				map[string]any{ // Explicitly allow from any namespace
					"key":      KubernetesNamespaceKey,
					"operator": "Exists",
				},
			}},
		},
		"toPorts": ciliumToPorts(entries),
	}
}

// MakeEndpointPolicy returns the policy admitting an Endpoint's visibility to
// one backend's pods on the backend's port: namespace visibility admits the
// other projects of the namespace and environment, internal and external admit
// the gateways. It returns nil when the visibility admits nothing beyond the
// project scope the backend component's own policy already admits.
func MakeEndpointPolicy(params EndpointPolicyParams) map[string]any {
	var namespaceScope, broadScope bool
	for _, vis := range params.Visibility {
		switch vis {
		case openchoreov1alpha1.EndpointVisibilityNamespace:
			namespaceScope = true
		case openchoreov1alpha1.EndpointVisibilityInternal, openchoreov1alpha1.EndpointVisibilityExternal:
			broadScope = true
		}
	}
	if !namespaceScope && !broadScope {
		return nil
	}

	proto := l4Protocol(params.Protocol)

	if params.Provider == ProviderCilium {
		entries := []ciliumPortEntry{{port: strconv.Itoa(int(params.Port)), proto: proto, isL7: isL7Proxied(params.Protocol)}}
		var ingressRules []any
		if namespaceScope {
			ingressRules = append(ingressRules, ciliumNamespaceIngressRule(params.CPNamespace, params.Environment, entries))
		}
		if broadScope {
			ingressRules = append(ingressRules, ciliumSystemComponentIngressRule(entries))
		}
		return map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumNetworkPolicy",
			"metadata": map[string]any{
				"name":      params.Name,
				"namespace": params.Namespace,
			},
			"spec": map[string]any{
				"endpointSelector": map[string]any{
					"matchLabels": toAnyMap(params.PodSelectors),
				},
				"ingress": ingressRules,
			},
		}
	}

	ports := []any{map[string]any{"protocol": proto, "port": int64(params.Port)}}
	var ingressRules []any
	if namespaceScope {
		ingressRules = append(ingressRules, namespaceIngressRule(params.CPNamespace, params.Environment, ports))
	}
	if broadScope {
		ingressRules = append(ingressRules, systemComponentIngressRule(ports))
	}
	return map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]any{
			"name":      params.Name,
			"namespace": params.Namespace,
		},
		"spec": map[string]any{
			"podSelector": map[string]any{
				"matchLabels": toAnyMap(params.PodSelectors),
			},
			"policyTypes": []any{"Ingress"},
			"ingress":     ingressRules,
		},
	}
}

// ProviderFromAnnotations returns the provider a data plane's
// "openchoreo.dev/networkpolicyprovider" annotation selects, defaulting to
// ProviderKubernetes.
func ProviderFromAnnotations(annotations map[string]string) Provider {
	if annotations[ProviderAnnotation] == string(ProviderCilium) {
		return ProviderCilium
	}
	return ProviderKubernetes
}

// ciliumToPorts builds the toPorts slice for a CNP ingress rule, grouping L7-proxied
// ports (with rules.http) separately from L4-only ports.
func ciliumToPorts(entries []ciliumPortEntry) []any {
	var l7Ports, l4Ports []any
	for _, e := range entries {
		p := map[string]any{"port": e.port, "protocol": e.proto}
		if e.isL7 {
			l7Ports = append(l7Ports, p)
		} else {
			l4Ports = append(l4Ports, p)
		}
	}
	var toPorts []any
	if len(l7Ports) > 0 {
		toPorts = append(toPorts, map[string]any{
			"ports": l7Ports,
			"rules": map[string]any{"http": []any{
				map[string]any{},
			}},
		})
	}
	if len(l4Ports) > 0 {
		toPorts = append(toPorts, map[string]any{"ports": l4Ports})
	}
	return toPorts
}

// l4Protocol returns the transport protocol a policy port uses for an
// endpoint protocol: UDP for UDP endpoints, TCP for everything else.
func l4Protocol(p openchoreov1alpha1.EndpointProtocol) string {
	if p == openchoreov1alpha1.EndpointProtocolUDP {
		return "UDP"
	}
	return "TCP"
}

// isL7Proxied reports whether the endpoint type is proxied through Envoy for L7 inspection.
func isL7Proxied(t openchoreov1alpha1.EndpointProtocol) bool {
	switch t {
	case openchoreov1alpha1.EndpointProtocolHTTP,
		openchoreov1alpha1.EndpointProtocolGraphQL,
		openchoreov1alpha1.EndpointProtocolWebsocket,
		openchoreov1alpha1.EndpointProtocolGRPC:
		return true
	}
	return false
}

// toAnyMap converts map[string]string to map[string]any for use in unstructured maps.
func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
