// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// loadDefaultComponentTypeSample reads a getting-started ClusterComponentType sample and
// returns it as a ComponentType. ClusterComponentTypeSpec and ComponentTypeSpec are
// structurally identical, so a JSON round-trip is a faithful conversion.
func loadDefaultComponentTypeSample(t *testing.T, file string) *v1alpha1.ComponentType {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "samples", "getting-started", "component-types", file))
	if err != nil {
		t.Fatalf("failed to read sample: %v", err)
	}
	var cct v1alpha1.ClusterComponentType
	if err := yaml.Unmarshal(raw, &cct); err != nil {
		t.Fatalf("failed to parse sample: %v", err)
	}

	b, err := json.Marshal(cct.Spec)
	if err != nil {
		t.Fatalf("failed to marshal spec: %v", err)
	}
	ct := &v1alpha1.ComponentType{}
	ct.Name = cct.Name
	if err := json.Unmarshal(b, &ct.Spec); err != nil {
		t.Fatalf("failed to convert spec: %v", err)
	}
	return ct
}

// TestDefaultComponentTypes_DedicatedServiceAccountWithoutToken checks that every default
// ComponentType renders its own ServiceAccount without an automounted token and that the
// workload's pod template runs as it, instead of the namespace's default ServiceAccount.
func TestDefaultComponentTypes_DedicatedServiceAccountWithoutToken(t *testing.T) {
	tests := []struct {
		file         string
		workloadKind string
		podSpecPath  []string
		workloadYAML string
	}{
		{
			file:         "service.yaml",
			workloadKind: "Deployment",
			podSpecPath:  []string{"spec", "template", "spec"},
			workloadYAML: `
spec:
  container: {image: registry.example.com/app:1.0.0}
  endpoints:
    http: {type: HTTP, port: 8080}
`,
		},
		{
			file:         "webapp.yaml",
			workloadKind: "Deployment",
			podSpecPath:  []string{"spec", "template", "spec"},
			workloadYAML: `
spec:
  container: {image: registry.example.com/app:1.0.0}
  endpoints:
    http: {type: HTTP, port: 8080}
`,
		},
		{
			file:         "worker.yaml",
			workloadKind: "Deployment",
			podSpecPath:  []string{"spec", "template", "spec"},
			workloadYAML: `
spec:
  container: {image: registry.example.com/app:1.0.0}
`,
		},
		{
			file:         "scheduled-task.yaml",
			workloadKind: "CronJob",
			podSpecPath:  []string{"spec", "jobTemplate", "spec", "template", "spec"},
			workloadYAML: `
spec:
  container: {image: registry.example.com/app:1.0.0}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			workload := &v1alpha1.Workload{}
			if err := yaml.Unmarshal([]byte(tt.workloadYAML), workload); err != nil {
				t.Fatalf("failed to parse workload: %v", err)
			}

			// The service type's validations reference the gateway, so configure one as a
			// real data plane would.
			dataPlane := &v1alpha1.DataPlane{}
			dataPlane.Spec.Gateway.Ingress = &v1alpha1.GatewayNetworkSpec{
				External: &v1alpha1.GatewayEndpointSpec{
					Name:      "gateway-default",
					Namespace: "openchoreo-data-plane",
					HTTP:      &v1alpha1.GatewayListenerSpec{ListenerName: "http", Port: 19080, Host: "openchoreoapis.localhost"},
				},
			}

			input := &RenderInput{
				ComponentType: loadDefaultComponentTypeSample(t, tt.file),
				Component:     &v1alpha1.Component{},
				Workload:      workload,
				Environment:   &v1alpha1.Environment{},
				DataPlane:     dataPlane,
				Metadata: context.MetadataContext{
					Name:               "app-dev-12345678",
					Namespace:          "dp-ns-proj-dev-12345678",
					ComponentName:      "app",
					ComponentUID:       "uid1",
					ComponentNamespace: "ns",
					ProjectName:        "proj",
					ProjectUID:         "uid2",
					DataPlaneName:      "dp",
					DataPlaneUID:       "uid3",
					EnvironmentName:    "dev",
					EnvironmentUID:     "uid4",
					Labels:             map[string]string{"openchoreo.dev/component": "app"},
					Annotations:        map[string]string{},
					PodSelectors:       map[string]string{"openchoreo.dev/component-uid": "uid1"},
				},
			}

			output, err := NewPipeline().Render(t.Context(), input)
			if err != nil {
				t.Fatalf("Render() error = %v", err)
			}

			var serviceAccount, workloadResource map[string]any
			for _, r := range output.Resources {
				switch r.Resource["kind"] {
				case "ServiceAccount":
					if serviceAccount != nil {
						t.Fatalf("expected a single ServiceAccount")
					}
					serviceAccount = r.Resource
				case tt.workloadKind:
					workloadResource = r.Resource
				}
			}
			if serviceAccount == nil {
				t.Fatalf("expected a ServiceAccount to be rendered")
			}
			if workloadResource == nil {
				t.Fatalf("expected a %s to be rendered", tt.workloadKind)
			}

			saName, _, _ := unstructured.NestedString(serviceAccount, "metadata", "name")
			if saName != input.Metadata.Name {
				t.Errorf("ServiceAccount name = %q, want %q", saName, input.Metadata.Name)
			}
			saNamespace, _, _ := unstructured.NestedString(serviceAccount, "metadata", "namespace")
			if saNamespace != input.Metadata.Namespace {
				t.Errorf("ServiceAccount namespace = %q, want %q", saNamespace, input.Metadata.Namespace)
			}
			saAutomount, found, _ := unstructured.NestedBool(serviceAccount, "automountServiceAccountToken")
			if !found || saAutomount {
				t.Errorf("ServiceAccount automountServiceAccountToken = %v (found %v), want false", saAutomount, found)
			}

			podSpec, found, err := unstructured.NestedMap(workloadResource, tt.podSpecPath...)
			if err != nil || !found {
				t.Fatalf("pod spec not found at %v: %v", tt.podSpecPath, err)
			}
			if got := podSpec["serviceAccountName"]; got != saName {
				t.Errorf("pod serviceAccountName = %v, want %q", got, saName)
			}
			if got, ok := podSpec["automountServiceAccountToken"].(bool); !ok || got {
				t.Errorf("pod automountServiceAccountToken = %v, want false", podSpec["automountServiceAccountToken"])
			}
			if _, found := serviceAccount["imagePullSecrets"]; found {
				t.Errorf("ServiceAccount imagePullSecrets = %v, want omitted by default", serviceAccount["imagePullSecrets"])
			}

			// Image pull secrets configured for the environment go on the ServiceAccount, so the
			// pods get them like they would from the namespace's default ServiceAccount.
			releaseBinding := &v1alpha1.ReleaseBinding{}
			if err := yaml.Unmarshal([]byte(`spec: {componentTypeEnvironmentConfigs: {imagePullSecrets: [regcred]}}`), releaseBinding); err != nil {
				t.Fatalf("failed to parse release binding: %v", err)
			}
			input.ReleaseBinding = releaseBinding
			output, err = NewPipeline().Render(t.Context(), input)
			if err != nil {
				t.Fatalf("Render() with imagePullSecrets error = %v", err)
			}
			serviceAccount = nil
			for _, r := range output.Resources {
				if r.Resource["kind"] == "ServiceAccount" {
					serviceAccount = r.Resource
				}
			}
			if serviceAccount == nil {
				t.Fatalf("expected a ServiceAccount to be rendered with imagePullSecrets")
			}
			pullSecrets, _, _ := unstructured.NestedSlice(serviceAccount, "imagePullSecrets")
			if len(pullSecrets) != 1 || pullSecrets[0].(map[string]any)["name"] != "regcred" {
				t.Errorf("ServiceAccount imagePullSecrets = %v, want [{name: regcred}]", pullSecrets)
			}
		})
	}
}
