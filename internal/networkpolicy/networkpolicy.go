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
		protocol := "TCP"
		if ep.Type == openchoreov1alpha1.EndpointTypeUDP {
			protocol = "UDP"
		}
		entries = append(entries, epEntry{
			name: name,
			port: map[string]any{
				"protocol": protocol,
				"port":     int64(ep.Port),
			},
			visibility: ep.Visibility,
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
		ingressRules = append(ingressRules, map[string]any{
			"from": []any{
				map[string]any{
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{
							labels.LabelKeyNamespaceName:   params.CPNamespace,
							labels.LabelKeyEnvironmentName: params.Environment,
						},
					},
				},
			},
			"ports": namespacePorts,
		})
	}

	// Rule 3: system components (e.g., gateway) from any namespace (internal or external visibility)
	if len(broadAccessPorts) > 0 {
		ingressRules = append(ingressRules, map[string]any{
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
			"ports": broadAccessPorts,
		})
	}

	return ingressRules
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
		proto := "TCP"
		if ep.Type == openchoreov1alpha1.EndpointTypeUDP {
			proto = "UDP"
		}
		entries = append(entries, ciliumPortEntry{
			port:       strconv.Itoa(int(ep.Port)),
			proto:      proto,
			isL7:       isL7Proxied(ep.Type),
			visibility: ep.Visibility,
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
		ingressRules = append(ingressRules, map[string]any{
			"fromEndpoints": []any{
				map[string]any{
					"matchLabels": map[string]any{
						labels.LabelKeyNamespaceName:   params.CPNamespace,
						labels.LabelKeyEnvironmentName: params.Environment,
					},
					"matchExpressions": []any{
						map[string]any{ // Explicitly allow from any namespace
							"key":      KubernetesNamespaceKey,
							"operator": "Exists",
						},
					},
				},
			},
			"toPorts": ciliumToPorts(namespacePorts),
		})
	}

	// Rule 3: system components (e.g., gateway) from any namespace (internal or external visibility)
	if len(broadPorts) > 0 {
		ingressRules = append(ingressRules, map[string]any{
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
			"toPorts": ciliumToPorts(broadPorts),
		})
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

// isL7Proxied reports whether the endpoint type is proxied through Envoy for L7 inspection.
func isL7Proxied(t openchoreov1alpha1.EndpointType) bool {
	switch t {
	case openchoreov1alpha1.EndpointTypeHTTP,
		openchoreov1alpha1.EndpointTypeGraphQL,
		openchoreov1alpha1.EndpointTypeWebsocket,
		openchoreov1alpha1.EndpointTypeGRPC:
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
