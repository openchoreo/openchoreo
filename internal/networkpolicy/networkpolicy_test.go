// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package networkpolicy

import (
	"strings"
	"testing"

	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// assertYAMLEqual marshals actual to YAML and compares against the expected YAML string.
// It fails the test with a readable diff if they don't match.
func assertYAMLEqual(t *testing.T, name string, actual map[string]any, expectedYAML string) {
	t.Helper()

	actualYAML, err := yaml.Marshal(actual)
	if err != nil {
		t.Fatalf("%s: failed to marshal actual to YAML: %v", name, err)
	}

	// Normalize: unmarshal both sides and re-marshal to get consistent formatting
	var expectedObj, actualObj any
	if err := yaml.Unmarshal([]byte(expectedYAML), &expectedObj); err != nil {
		t.Fatalf("%s: failed to unmarshal expected YAML: %v", name, err)
	}
	if err := yaml.Unmarshal(actualYAML, &actualObj); err != nil {
		t.Fatalf("%s: failed to unmarshal actual YAML: %v", name, err)
	}

	expectedNorm, _ := yaml.Marshal(expectedObj)
	actualNorm, _ := yaml.Marshal(actualObj)

	if string(expectedNorm) != string(actualNorm) {
		t.Errorf("%s: YAML mismatch\n--- expected ---\n%s\n--- actual ---\n%s",
			name, string(expectedNorm), string(actualNorm))
	}
}

func TestMakeBaselinePolicies(t *testing.T) {
	policies := MakeBaselinePolicies(BaselinePolicyParams{
		Namespace:   "dp-acme-payment-dev-x1y2z3w4",
		CPNamespace: "acme-corp",
	})

	if len(policies) != 2 {
		t.Fatalf("expected 2 baseline policies, got %d", len(policies))
	}

	assertYAMLEqual(t, "deny-all-ingress", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-deny-all-ingress
  namespace: dp-acme-payment-dev-x1y2z3w4
spec:
  podSelector: {}
  policyTypes:
    - Ingress
`)

	assertYAMLEqual(t, "egress-isolation", policies[1], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-egress-isolation
  namespace: dp-acme-payment-dev-x1y2z3w4
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    - to:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: DoesNotExist
    - to:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: In
                values:
                  - acme-corp
`)
}

func TestMakeComponentPolicies_NoEndpoints(t *testing.T) {
	result := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "my-comp",
		PodSelectors:  map[string]string{"app": "test"},
		Endpoints:     nil,
	})
	if result != nil {
		t.Error("expected nil for component with nil endpoints")
	}

	result = MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "my-comp",
		PodSelectors:  map[string]string{"app": "test"},
		Endpoints:     map[string]openchoreov1alpha1.WorkloadEndpoint{},
	})
	if result != nil {
		t.Error("expected nil for component with empty endpoints map")
	}
}

func TestMakeComponentPolicies_ProjectOnly(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "web-app",
		PodSelectors: map[string]string{
			labels.LabelKeyComponentName: "web-app",
			labels.LabelKeyProjectName:   "my-project",
		},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"http": {Type: "HTTP", Port: 8080},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	assertYAMLEqual(t, "project-only", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-web-app-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      openchoreo.dev/component: web-app
      openchoreo.dev/project: my-project
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: TCP
          port: 8080
`)
}

func TestMakeComponentPolicies_NamespaceVisibility(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "api-svc",
		PodSelectors:  map[string]string{"app": "api-svc"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"grpc": {
				Type:       "gRPC",
				Port:       9090,
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityNamespace},
			},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	assertYAMLEqual(t, "namespace-visibility", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-api-svc-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      app: api-svc
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: TCP
          port: 9090
    - from:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: In
                values:
                  - cp-ns
      ports:
        - protocol: TCP
          port: 9090
`)
}

func TestMakeComponentPolicies_ExternalVisibility(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "public-api",
		PodSelectors:  map[string]string{"app": "public-api"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"http": {
				Type:       "HTTP",
				Port:       8080,
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	assertYAMLEqual(t, "external-visibility", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-public-api-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      app: public-api
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: TCP
          port: 8080
    - from:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: DoesNotExist
      ports:
        - protocol: TCP
          port: 8080
`)
}

func TestMakeComponentPolicies_InternalVisibility(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "internal-svc",
		PodSelectors:  map[string]string{"app": "internal-svc"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"http": {
				Type:       "HTTP",
				Port:       8080,
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityInternal},
			},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	assertYAMLEqual(t, "internal-visibility", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-internal-svc-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      app: internal-svc
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: TCP
          port: 8080
    - from:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: DoesNotExist
      ports:
        - protocol: TCP
          port: 8080
`)
}

func TestMakeComponentPolicies_MixedVisibility(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "mixed-svc",
		PodSelectors:  map[string]string{"app": "mixed-svc"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"internal-api": {
				Type:       "REST",
				Port:       8080,
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityNamespace},
			},
			"public-api": {
				Type:       "REST",
				Port:       8443,
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			},
			"health": {
				Type: "HTTP",
				Port: 9090,
			},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	// Endpoints are sorted by name, so ordering is deterministic:
	// "health" (9090), "internal-api" (8080), "public-api" (8443)
	assertYAMLEqual(t, "mixed-visibility", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-mixed-svc-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      app: mixed-svc
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: TCP
          port: 9090
        - protocol: TCP
          port: 8080
        - protocol: TCP
          port: 8443
    - from:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: In
                values:
                  - cp-ns
      ports:
        - protocol: TCP
          port: 8080
    - from:
        - namespaceSelector:
            matchExpressions:
              - key: openchoreo.dev/controlplane-namespace
                operator: DoesNotExist
      ports:
        - protocol: TCP
          port: 8443
`)
}

func TestMakeComponentPolicies_UDPEndpoint(t *testing.T) {
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: "dns-svc",
		PodSelectors:  map[string]string{"app": "dns-svc"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"dns": {Type: openchoreov1alpha1.EndpointTypeUDP, Port: 5353},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	assertYAMLEqual(t, "udp-endpoint", policies[0], `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openchoreo-dns-svc-ingress
  namespace: dp-ns
spec:
  podSelector:
    matchLabels:
      app: dns-svc
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
      ports:
        - protocol: UDP
          port: 5353
`)
}

func TestMakeComponentPolicies_NameTruncation(t *testing.T) {
	longName := strings.Repeat("a", 250)
	policies := MakeComponentPolicies(ComponentPolicyParams{
		Namespace:     "dp-ns",
		CPNamespace:   "cp-ns",
		ComponentName: longName,
		PodSelectors:  map[string]string{"app": "test"},
		Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
			"http": {Type: "HTTP", Port: 8080},
		},
	})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}

	meta := policies[0]["metadata"].(map[string]any)
	name := meta["name"].(string)
	if len(name) > 253 {
		t.Errorf("policy name exceeds 253 chars: %d", len(name))
	}
}
