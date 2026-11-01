// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreRenderUsesUnsavedDefinitionAndClearsFindings(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	definition := `apiVersion: openchoreo.dev/v1alpha1
kind: ComponentType
metadata:
  name: web-application
  namespace: default
spec:
  parameters:
    openAPIV3Schema:
      type: object
      properties:
        replicas:
          type: integer
`
	component := `apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: web-app
  namespace: default
spec:
  componentType:
    kind: ComponentType
    name: deployment/web-application
  parameters:
    replicas: 1
`
	dp, cp := filepath.Join(dir, "type.yaml"), filepath.Join(dir, "component.yaml")
	for path, text := range map[string]string{dp: definition, cp: component} {
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	other := filepath.Join(root, "other-project")
	if err := os.MkdirAll(other, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "type.yaml"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	s.OpenDocument(pathToDocumentURI(dp), strings.Replace(definition, "type: integer", "type: string", 1), 2)
	s.triggerPreRenderSchemaValidationForPath(dp)
	findings := s.preRenderSchemaByURI[pathToDocumentURI(cp)]
	if len(findings) != 1 {
		t.Fatalf("unsaved schema mismatch not reported: %v", findings)
	}
	if findings[0].Code == nil || findings[0].Code.Value != "schema-parameters-mismatch" {
		t.Fatalf("unexpected diagnostic: %v", findings)
	}
	s.OpenDocument(pathToDocumentURI(dp), definition, 3)
	s.triggerPreRenderSchemaValidationForPath(dp)
	if len(s.preRenderSchemaByURI) != 0 {
		t.Fatal("resolved mismatch was not cleared")
	}
}

func TestReleaseEnvironmentAndRuntimeLifecycle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "lint", "pre-render-validator", "testdata", "releases.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "release.yaml")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	uri := pathToDocumentURI(path)
	s.OpenDocument(uri, string(data), 1)
	s.triggerPreRenderSchemaValidationForPath(path)
	counts := map[string]int{}
	for _, f := range s.preRenderSchemaByURI[uri] {
		if f.Code != nil {
			counts[f.Code.Value.(string)]++
		}
	}
	if counts["schema-parameters-mismatch"] != 2 || counts["schema-environment-configs-mismatch"] != 1 || counts["pre-render-validation-failed"] != 1 {
		t.Fatalf("missing findings: %+v", counts)
	}
	// An invalid unsaved overlay must not keep obsolete schema findings. The
	// structural YAML parser owns syntax errors, avoiding duplicate diagnostics.
	s.OpenDocument(uri, "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentRelease\nspec: [broken", 2)
	s.triggerPreRenderSchemaValidationForPath(path)
	if len(s.preRenderSchemaByURI[uri]) != 0 {
		t.Fatal("malformed overlay retained stale schema diagnostics")
	}
	if len(Validate("apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentRelease\nspec: [broken")) != 1 {
		t.Fatal("parser diagnostic missing")
	}
	// Closing the buffer restores the disk snapshot.
	s.CloseDocument(uri)
	s.triggerPreRenderSchemaValidationForPath(path)
	if len(s.preRenderSchemaByURI[uri]) != 4 {
		t.Fatal("disk snapshot not restored")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	s.triggerPreRenderSchemaValidationForPath(path)
	if len(s.preRenderSchemaByURI[uri]) != 0 {
		t.Fatal("deleted file retained stale findings")
	}
}

func TestPreRenderIndependentProjectCaches(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "lint", "pre-render-validator", "testdata", "releases.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	paths := []string{filepath.Join(t.TempDir(), "a.yaml"), filepath.Join(t.TempDir(), "b.yaml")}
	for _, path := range paths {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		s.triggerPreRenderSchemaValidationForPath(path)
	}
	for _, path := range paths {
		if len(s.preRenderSchemaByURI[pathToDocumentURI(path)]) != 4 {
			t.Fatal("validation of another project cleared cached findings")
		}
	}
	s.clearSchemaFindings(paths[:1])
	if len(s.preRenderSchemaByURI[pathToDocumentURI(paths[1])]) != 4 {
		t.Fatal("clearing one project cleared another")
	}
}

func TestRuntimeDiagnosticAlreadyReportedByPipeline(t *testing.T) {
	s := NewServer()
	uri := "file:///component.yaml"
	s.preRenderSchemaByURI[uri] = []protocol.Diagnostic{{Code: &protocol.IntegerOrString{Value: "pre-render-validation-failed"}, Message: "rule evaluated to false"}}
	original := []protocol.Diagnostic{{Source: stringPtr("openchoreo-post-render"), Message: "component type validation failed: rule evaluated to false"}}
	got := s.mergePreRenderSchemaDiagnostics(uri, original)
	if len(got) != 1 {
		t.Fatalf("duplicate runtime failure: %+v", got)
	}
	got = s.mergePreRenderSchemaDiagnostics(uri, nil)
	if len(got) != 1 {
		t.Fatal("runtime finding was lost without a pipeline error")
	}
}
