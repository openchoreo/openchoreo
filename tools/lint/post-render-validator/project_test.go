// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/tools/lint/parser"
)

func writeProject(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	collected, err := parser.CollectYAMLFiles(dir, false)
	if err != nil {
		t.Fatalf("collect files: %v", err)
	}
	return dir, collected
}

func findFinding(findings []Finding, code string) *Finding {
	for i := range findings {
		if findings[i].Code == code {
			return &findings[i]
		}
	}
	return nil
}

func hasFinding(findings []Finding, code string) bool {
	return findFinding(findings, code) != nil
}

func componentYAML(name string, ctKind string, ctName string, traits ...string) string {
	project := valueDefault

	var b strings.Builder
	b.WriteString("apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: " + name + "\n  namespace: default\nspec:\n  owner:\n    projectName: " + project + "\n  componentType:\n    kind: " + ctKind + "\n    name: " + ctName + "\n  parameters:\n    replicas: 1\n    port: 8080\n")
	if len(traits) > 0 {
		b.WriteString("  traits:\n")
		for i, trait := range traits {
			parts := strings.SplitN(trait, ":", 2)
			kind, traitName := "Trait", parts[0]
			if len(parts) == 2 {
				kind, traitName = parts[0], parts[1]
			}
			b.WriteString("    - kind: " + kind + "\n      name: " + traitName + "\n      instanceName: " + traitName + string(rune('a'+i)) + "\n")
		}
	}
	return b.String()
}

func workloadYAML(component string, image string) string {
	project := valueDefault

	return "apiVersion: openchoreo.dev/v1alpha1\nkind: Workload\nmetadata:\n  name: " + component + "-workload\n  namespace: default\nspec:\n  owner:\n    projectName: " + project + "\n    componentName: " + component + "\n  container:\n    image: " + image + "\n"
}

func componentTypeYAML(kind, name string) string {
	return `apiVersion: openchoreo.dev/v1alpha1
kind: ` + kind + `
metadata:
  name: ` + name + `
  namespace: default
spec:
  workloadType: deployment
  parameters:
    openAPIV3Schema:
      replicas: "integer | default=1"
      port: "integer | default=8080"
  resources:
    - id: deployment
      template:
        apiVersion: apps/v1
        kind: Deployment
        metadata:
          name: ${metadata.name}
          namespace: ${metadata.namespace}
          labels: ${metadata.labels}
        spec:
          replicas: ${parameters.replicas}
          selector:
            matchLabels: ${metadata.podSelectors}
          template:
            metadata:
              labels: ${metadata.podSelectors}
            spec:
              containers:
                - name: app
                  image: ${workload.container.image}
                  ports:
                    - containerPort: ${parameters.port}
                      protocol: TCP
                  resources:
                    limits:
                      cpu: 100m
                      memory: 128Mi
                    requests:
                      cpu: 50m
                      memory: 64Mi
`
}

func traitYAML(kind, name string) string {
	return `apiVersion: openchoreo.dev/v1alpha1
kind: ` + kind + `
metadata:
  name: ` + name + `
  namespace: default
spec:
  creates:
    - template:
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: ${metadata.name}-` + name + `
          namespace: ${metadata.namespace}
        data:
          tier: standard
`
}

func releaseBindingYAML(name string, component string) string {
	environment := "dev"

	project := valueDefault

	return "apiVersion: openchoreo.dev/v1alpha1\nkind: ReleaseBinding\nmetadata:\n  name: " + name + "\n  namespace: default\nspec:\n  owner:\n    projectName: " + project + "\n    componentName: " + component + "\n  environment: " + environment + "\n"
}

func environmentYAML(dpKind string, dpName string) string {
	name := "dev"

	return "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata:\n  name: " + name + "\n  namespace: default\nspec:\n  dataPlaneRef:\n    kind: " + dpKind + "\n    name: " + dpName + "\n"
}

func dataPlaneYAML(kind, name string) string {
	return "apiVersion: openchoreo.dev/v1alpha1\nkind: " + kind + "\nmetadata:\n  name: " + name + "\n  namespace: default\nspec:\n  planeID: " + name + "\n  clusterAgent:\n    clientCA:\n      secretKeyRef:\n        name: ca\n        key: ca.crt\n"
}

func TestValidateProject_PrefersActiveComponent(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"backend-ct.yaml":  componentTypeYAML("ComponentType", "backend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"backend.yaml":     componentYAML("backend", "ComponentType", "deployment/backend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx"),
		"backend-wl.yaml":  workloadYAML("backend", "nginx:1.25"),
	})
	active := filepath.Join(dir, "frontend.yaml")

	findings := ValidateProject(context.Background(), files, active)
	if !hasFinding(findings, valueImageLatest) {
		t.Fatalf("expected image-latest, got %v", findings)
	}
	for _, finding := range findings {
		if finding.Code == valueImageLatest && !strings.Contains(finding.Path, "frontend-wl.yaml") {
			t.Fatalf("image-latest attributed to %s, want frontend workload", finding.Path)
		}
		if strings.Contains(finding.Path, "backend-wl.yaml") {
			t.Fatalf("backend resource leaked into frontend validation: %v", finding)
		}
	}
}

func TestValidateProject_IndexesMultiDocActiveFile(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"component.yaml": componentYAML("frontend", "ComponentType", "deployment/frontend-ct") + "---\n" + workloadYAML("frontend", "nginx:1.25"),
		"ct.yaml":        componentTypeYAML("ComponentType", "frontend-ct"),
	})

	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "component.yaml"))
	if hasFinding(findings, testValuePostRenderError) {
		t.Fatalf("resources in the active file should be indexed, got %+v", findings)
	}
}

func TestValidateProject_ResolvesFromWorkload(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend-wl.yaml"))
	if finding := findFinding(findings, valueImageLatest); finding == nil {
		t.Fatalf("expected image-latest, got %v", findings)
	} else if !strings.Contains(finding.Path, "frontend-wl.yaml") {
		t.Fatalf("image-latest path = %s, want frontend workload", finding.Path)
	}
}

func TestValidateProject_ResolvesFromTrait(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"trait.yaml":       traitYAML("Trait", "storage"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct", "storage"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "trait.yaml"))
	if !hasFinding(findings, valueImageLatest) {
		t.Fatalf("expected image-latest, got %v", findings)
	}
}

func TestValidateProject_AmbiguousTrait(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"trait.yaml":       traitYAML("Trait", "storage"),
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"backend-ct.yaml":  componentTypeYAML("ComponentType", "backend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct", "storage"),
		"backend.yaml":     componentYAML("backend", "ComponentType", "deployment/backend-ct", "storage"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx:1.25"),
		"backend-wl.yaml":  workloadYAML("backend", "nginx:1.25"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "trait.yaml"))
	finding := findFinding(findings, testValuePostRenderError)
	if finding == nil || !strings.Contains(finding.Message, "multiple Components") {
		t.Fatalf("expected ambiguous Component error, got %v", findings)
	}
}

func TestValidateProject_MissingWorkload(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend.yaml"))
	finding := findFinding(findings, testValuePostRenderError)
	if finding == nil || !strings.Contains(finding.Message, "no Workload") {
		t.Fatalf("expected missing Workload error, got %v", findings)
	}
}

func TestValidateProject_MultipleReleaseBindings(t *testing.T) {
	base := map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx"),
		"dev.yaml":         releaseBindingYAML("frontend-dev", "frontend"),
		"prod.yaml":        releaseBindingYAML("frontend-prod", "frontend"),
		"dev-env.yaml":     environmentYAML("DataPlane", valueDefault),
		"dp.yaml":          dataPlaneYAML("DataPlane", valueDefault),
	}
	dir, files := writeProject(t, base)
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend.yaml"))
	finding := findFinding(findings, testValuePostRenderError)
	if finding == nil || !strings.Contains(finding.Message, "multiple ReleaseBindings") {
		t.Fatalf("expected multiple ReleaseBindings error, got %v", findings)
	}

	// Opening the desired ReleaseBinding disambiguates.
	findings = ValidateProject(context.Background(), files, filepath.Join(dir, "prod.yaml"))
	if hasFinding(findings, testValuePostRenderError) {
		t.Fatalf("opening a ReleaseBinding should disambiguate, got %v", findings)
	}
}

func TestValidateProject_ClusterVariants(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"cluster-ct.yaml":    componentTypeYAML("ClusterComponentType", "cluster-frontend-ct"),
		"cluster-trait.yaml": traitYAML("ClusterTrait", "cluster-storage"),
		"frontend.yaml":      componentYAML("frontend", "ClusterComponentType", "deployment/cluster-frontend-ct", "ClusterTrait:cluster-storage"),
		"frontend-wl.yaml":   workloadYAML("frontend", "nginx"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend.yaml"))
	if hasFinding(findings, testValuePostRenderError) {
		t.Fatalf("unexpected selection error: %v", findings)
	}
	if !hasFinding(findings, valueImageLatest) {
		t.Fatalf("expected image-latest, got %v", findings)
	}
}

func TestValidateProject_ClusterDataPlane(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx:1.25"),
		"frontend-rb.yaml": releaseBindingYAML("frontend-dev", "frontend"),
		"dev.yaml":         environmentYAML("ClusterDataPlane", "shared"),
		"cluster-dp.yaml":  dataPlaneYAML("ClusterDataPlane", "shared"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend-rb.yaml"))
	if hasFinding(findings, testValuePostRenderError) {
		t.Fatalf("unexpected ClusterDataPlane error: %v", findings)
	}
}

func TestValidateProject_MissingDataPlaneFixLocation(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx:1.25"),
		"frontend-rb.yaml": releaseBindingYAML("frontend-dev", "frontend"),
		"dev.yaml":         environmentYAML("ClusterDataPlane", "shared"),
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend.yaml"))
	finding := findFinding(findings, testValuePostRenderError)
	if finding == nil {
		t.Fatalf("expected missing DataPlane error, got %v", findings)
	}
	if !strings.Contains(finding.Path, "dev.yaml") || finding.Line == 0 {
		t.Fatalf("missing DataPlane should point at the Environment, got %+v", finding)
	}
	if !strings.Contains(finding.Fix, "ClusterDataPlane") ||
		finding.FixPath != finding.Path || finding.FixLine != finding.Line || finding.FixColumn != finding.Column {
		t.Fatalf("fix should point at the Environment dataPlaneRef, got %+v", finding)
	}
}

func TestValidateProject_InvalidActiveYAML(t *testing.T) {
	dir, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    "apiVersion: openchoreo.dev/v1alpha1\nkind: [\n",
	})
	findings := ValidateProject(context.Background(), files, filepath.Join(dir, "frontend.yaml"))
	finding := findFinding(findings, testValuePostRenderError)
	if finding == nil {
		t.Fatalf("expected post-render-error for invalid active YAML, got %v", findings)
	}
}

func TestValidateFiles_PreservesCLIShape(t *testing.T) {
	_, files := writeProject(t, map[string]string{
		"frontend-ct.yaml": componentTypeYAML("ComponentType", "frontend-ct"),
		"frontend.yaml":    componentYAML("frontend", "ComponentType", "deployment/frontend-ct"),
		"frontend-wl.yaml": workloadYAML("frontend", "nginx"),
	})
	findings := ValidateFiles(context.Background(), files)
	finding := findFinding(findings, valueImageLatest)
	if finding == nil || finding.Line == 0 || finding.Column == 0 {
		t.Fatalf("expected located image-latest finding, got %v", findings)
	}
	if finding.Fix == "" || finding.FixPath != finding.Path || finding.FixLine != finding.Line {
		t.Fatalf("expected image-latest fix at the same location, got %+v", finding)
	}
}

func TestClusterDataPlaneToDataPlane(t *testing.T) {
	cdp := parseClusterDataPlane(t, dataPlaneYAML("ClusterDataPlane", "shared"))
	dp := clusterDataPlaneToDataPlane(cdp)
	if dp.Name != "shared" || dp.Spec.PlaneID != "shared" {
		t.Fatalf("converted data plane = %#v", dp)
	}
	if dp.Kind != "DataPlane" {
		t.Fatalf("converted kind = %q, want DataPlane", dp.Kind)
	}
}

func parseClusterDataPlane(t *testing.T, text string) v1alpha1.ClusterDataPlane {
	t.Helper()
	docs, err := parser.ParseYAML([]byte(text))
	if err != nil || len(docs) == 0 {
		t.Fatalf("parse ClusterDataPlane: %v", err)
	}
	obj, err := decodeTyped[v1alpha1.ClusterDataPlane](docs[0])
	if err != nil {
		t.Fatalf("decode ClusterDataPlane: %v", err)
	}
	return obj
}
