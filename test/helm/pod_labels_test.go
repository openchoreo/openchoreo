// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package helm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"testing"

	"gopkg.in/yaml.v3"
)

type renderedManifest struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Metadata struct {
				Labels map[string]string `yaml:"labels"`
			} `yaml:"metadata"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

var observabilityHelmOverrides = []string{
	"--set", "observer.secretName=observer-secret",
	"--set", "observer.controlPlaneApiUrl=http://api.openchoreo:8080",
	"--set", "observer.extraEnvs[0].value=http://observer:11080",
	"--set", "rca.enabled=true",
	"--set", "rca.secretName=rca-secret",
	"--set", "rca.observerApiUrl=http://observer:8080",
	"--set", "rca.openchoreoApiUrl=http://api.openchoreo:8080",
	"--set", "finOpsAgent.enabled=true",
	"--set", "finOpsAgent.secretName=finops-secret",
	"--set", "finOpsAgent.opencostMcpServerUrl=http://opencost:8081",
	"--set", "finOpsAgent.openchoreoApiUrl=http://api.openchoreo:8080",
	"--set", "security.oidc.tokenUrl=https://idp.example.com/token",
	"--set", "finOpsAgent.oauth.clientId=finops-client",
}

func TestHelmPodLabels_DoNotDuplicateChartOwnedKeys(t *testing.T) {
	const customKey = "example.com/custom"

	cases := []struct {
		name   string
		labels map[string]string
	}{
		{name: "empty"},
		{name: "non-conflicting", labels: map[string]string{customKey: "kept"}},
		{name: "conflicting", labels: map[string]string{
			customKey:                     "kept",
			"app.kubernetes.io/name":      "evil",
			"app.kubernetes.io/instance":  "evil",
			"app.kubernetes.io/component": "evil",
			"openchoreo.dev/plane":        "evil",
			"openchoreo.dev/plane-id":     "evil",
		}},
	}

	for _, tc := range cases {
		t.Run("control-plane/"+tc.name, func(t *testing.T) {
			args := append([]string{}, helmGuardOverrides...)
			args = append(args,
				"--set", "portalAssistant.enabled=true",
				"--set", "portalAssistant.llm.secretName=llm-secret",
				"--set", "portalAssistant.llm.modelName=openai:gpt-4o-mini",
			)
			if tc.labels != nil {
				args = append(args, "--set-json", "portalAssistant.podLabels="+jsonValue(t, tc.labels))
			}

			rendered := renderHelmChart(t, controlPlaneChart, args...)
			assertNoDuplicateYAMLKeys(t, rendered)

			deployment := mustRenderedDeployment(t, decodeRenderedManifests(t, rendered), "portal-assistant")
			assertChartOwnedLabels(t, deployment.Spec.Template.Metadata.Labels, map[string]string{
				"app.kubernetes.io/name":      "openchoreo-control-plane",
				"app.kubernetes.io/instance":  "openchoreo",
				"app.kubernetes.io/component": "portal-assistant",
				"openchoreo.dev/plane":        "controlplane",
			}, tc.labels, customKey)
		})

		t.Run("observability/"+tc.name, func(t *testing.T) {
			args := append([]string{}, observabilityHelmOverrides...)
			if tc.labels != nil {
				for _, key := range []string{"observer.podLabels", "rca.podLabels", "finOpsAgent.podLabels"} {
					args = append(args, "--set-json", key+"="+jsonValue(t, tc.labels))
				}
			}

			rendered := renderHelmChart(t, observabilityPlaneChart, args...)
			assertNoDuplicateYAMLKeys(t, rendered)

			manifests := decodeRenderedManifests(t, rendered)
			for deploymentName, component := range map[string]string{
				"observer":     "observer",
				"sre-agent":    "sre-agent",
				"finops-agent": "finops-agent",
			} {
				deployment := mustRenderedDeployment(t, manifests, deploymentName)
				assertChartOwnedLabels(t, deployment.Spec.Template.Metadata.Labels, map[string]string{
					"app.kubernetes.io/name":      "openchoreo-observability-plane",
					"app.kubernetes.io/instance":  "openchoreo",
					"app.kubernetes.io/component": component,
					"openchoreo.dev/plane":        "observabilityplane",
					"openchoreo.dev/plane-id":     "default",
				}, tc.labels, customKey)
			}
		})
	}
}

func renderHelmChart(t *testing.T, chart string, args ...string) []byte {
	t.Helper()

	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed; skipping the rendered pod label check")
	}

	base := []string{"template", "openchoreo", chart, "--namespace", "openchoreo"}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(t.Context(), helm, append(base, args...)...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to render %s: %v\n%s", chart, err, stderr.String())
	}
	return rendered
}

func decodeRenderedManifests(t *testing.T, rendered []byte) []renderedManifest {
	t.Helper()

	decoder := yaml.NewDecoder(bytes.NewReader(rendered))
	var manifests []renderedManifest
	for {
		var manifest renderedManifest
		err := decoder.Decode(&manifest)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to decode rendered manifest: %v", err)
		}
		if manifest.Kind != "" {
			manifests = append(manifests, manifest)
		}
	}
	return manifests
}

func mustRenderedDeployment(t *testing.T, manifests []renderedManifest, name string) renderedManifest {
	t.Helper()

	for _, manifest := range manifests {
		if manifest.Kind == "Deployment" && manifest.Metadata.Name == name {
			return manifest
		}
	}
	t.Fatalf("deployment %q not found in rendered manifests", name)
	return renderedManifest{}
}

func assertChartOwnedLabels(t *testing.T, got, want map[string]string, customLabels map[string]string, customKey string) {
	t.Helper()

	for key, value := range want {
		if got[key] != value {
			t.Errorf("label %q = %q, want %q", key, got[key], value)
		}
	}
	if customLabels != nil {
		if value := customLabels[customKey]; value != "" && got[customKey] != value {
			t.Errorf("custom label %q = %q, want %q", customKey, got[customKey], value)
		}
	}
}

func assertNoDuplicateYAMLKeys(t *testing.T, rendered []byte) {
	t.Helper()

	decoder := yaml.NewDecoder(bytes.NewReader(rendered))
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to decode rendered YAML: %v", err)
		}
		if err := findDuplicateKey(&document); err != nil {
			t.Error(err)
		}
	}
}

func findDuplicateKey(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		seen := map[string]int{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if first, ok := seen[key.Value]; ok {
				return fmt.Errorf("duplicate key %q at line %d (first at line %d)", key.Value, key.Line, first)
			}
			seen[key.Value] = key.Line
			if err := findDuplicateKey(node.Content[i+1]); err != nil {
				return err
			}
		}
	}
	for _, child := range node.Content {
		if err := findDuplicateKey(child); err != nil {
			return err
		}
	}
	return nil
}

func jsonValue(t *testing.T, value map[string]string) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("failed to encode labels: %v", err)
	}
	return string(raw)
}
