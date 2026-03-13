// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package networkpolicy

import (
	"fmt"
	"sort"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	"github.com/openchoreo/openchoreo/internal/labels"
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
}

// MakeComponentPolicies returns a NetworkPolicy for a component with ingress rules
// based on declared endpoint visibility. Egress is unrestricted.
func MakeComponentPolicies(params ComponentPolicyParams) []map[string]any {
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

	// Rule 3: gateway / non-OpenChoreo namespaces (internal or external visibility)
	if len(broadAccessPorts) > 0 {
		ingressRules = append(ingressRules, map[string]any{
			"from": []any{
				map[string]any{
					"namespaceSelector": map[string]any{
						"matchExpressions": []any{
							map[string]any{
								"key":      labels.LabelKeyNamespaceName,
								"operator": "DoesNotExist",
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

// toAnyMap converts map[string]string to map[string]any for use in unstructured maps.
func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
