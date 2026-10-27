// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package postrendervalidator

import (
	"testing"

	"github.com/openchoreo/openchoreo/tools/lint/parser"
	"github.com/openchoreo/openchoreo/tools/lint/ruleengine"
)

func parseSources(t *testing.T, files map[string]string) []postRenderSource {
	t.Helper()
	// Sort by name so the traversal order is deterministic.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	var sources []postRenderSource
	for _, name := range names {
		docs, err := parser.ParseYAML([]byte(files[name]))
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, d := range docs {
			sources = append(sources, postRenderSource{doc: d, file: name})
		}
	}
	return sources
}

func TestResolvePostRenderDiagnostic_PolicyField(t *testing.T) {
	componentType := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: web-application
spec:
  resources:
    - template:
        spec:
          containers:
            - name: app
              securityContext:
                privileged: true
`
	workload := `apiVersion: openchoreo.dev/v1alpha1
kind: Workload
metadata:
  name: web-app
spec:
  container:
    image: nginx:latest
`
	sources := parseSources(t, map[string]string{
		"component.yaml":     "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: web-app\n",
		"componenttype.yaml": componentType,
		"workload.yaml":      workload,
	})

	file, pos := resolvePostRenderDiagnostic(
		ruleengine.Diagnostic{Code: "no-privileged", Message: `Deployment/lint: container "app" runs as privileged`},
		sources,
		"component.yaml",
	)
	if file != "componenttype.yaml" {
		t.Fatalf("privileged file = %q, want componenttype.yaml", file)
	}
	if pos.Line != 12 || pos.Column != 17 {
		t.Fatalf("privileged position = %d:%d, want 12:17", pos.Line, pos.Column)
	}

	file, pos = resolvePostRenderDiagnostic(
		ruleengine.Diagnostic{Code: "image-latest", Message: `Deployment/lint: container "app" uses image "nginx:latest"`},
		sources,
		"component.yaml",
	)
	if file != "workload.yaml" {
		t.Fatalf("image file = %q, want workload.yaml", file)
	}
	if pos.Line != 7 {
		t.Fatalf("image line = %d, want 7", pos.Line)
	}
}

func TestResolvePostRenderDiagnostic_DeclaredFailure(t *testing.T) {
	trait := `apiVersion: openchoreo.dev/v1alpha1
kind: Trait
metadata:
  name: storage
spec:
  postRenderValidations:
    - rule: "replicas <= 1"
`
	sources := parseSources(t, map[string]string{"storage-trait.yaml": trait})
	file, pos := resolvePostRenderDiagnostic(
		ruleengine.Diagnostic{Code: "post-render-validation-failed", Message: "Trait storage: replicas must be one"},
		sources,
		"component.yaml",
	)
	if file != "storage-trait.yaml" {
		t.Fatalf("file = %q, want storage-trait.yaml", file)
	}
	if pos.Line != 6 {
		t.Fatalf("line = %d, want postRenderValidations at line 6", pos.Line)
	}
}

func TestResolvePostRenderDiagnostic_FallsBack(t *testing.T) {
	sources := parseSources(t, map[string]string{
		"component.yaml": "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata:\n  name: web-app\n",
	})
	file, pos := resolvePostRenderDiagnostic(
		ruleengine.Diagnostic{Code: "post-render-error", Message: "boom"},
		sources,
		"missing.yaml",
	)
	if file != "component.yaml" || pos.Line != 1 {
		t.Fatalf("fallback = %q:%d:%d, want component.yaml:1:1", file, pos.Line, pos.Column)
	}
}
