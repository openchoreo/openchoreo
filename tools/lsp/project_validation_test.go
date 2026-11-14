// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestProjectDiagnosticsMergeAutomaticFindingsOnce(t *testing.T) {
	d := protocol.Diagnostic{Message: "invalid parameter", Code: &protocol.IntegerOrString{Value: testValueSchemaParametersMismatch}, Range: protocol.Range{Start: protocol.Position{Line: 4, Character: 2}}}
	manual := d
	manual.Source = stringPtr("openchoreo-post-render")
	other := d
	other.Range.Start.Line = 8
	got := uniqueProjectDiagnostics([]protocol.Diagnostic{d, manual, other})
	if len(got) != 2 {
		t.Fatalf("automatic/manual duplicates must merge, distinct locations must remain: %+v", got)
	}
}
func TestFirstProjectRunInvalidatedByNewFile(t *testing.T) {
	s := NewServer()
	root := t.TempDir()
	s.postRenderScope = root
	s.postRenderRunning = true
	s.invalidatePostRenderForEdit(pathToDocumentURI(filepath.Join(root, "new.yaml")))
	if s.postRenderEpoch != 1 {
		t.Fatal("first project run was not invalidated by a newly created file")
	}
}
func TestProjectWorkspaceBoundary(t *testing.T) {
	s := NewServer()
	root := t.TempDir()
	other := t.TempDir()
	s.workspaceRoots = []string{root, other}
	path := filepath.Join(root, "nested", "component.yaml")
	if got := s.postRenderRoot(path); got != root {
		t.Fatalf("root=%s want=%s", got, root)
	}
	standalone := filepath.Join(t.TempDir(), "component.yaml")
	if got := s.postRenderRoot(standalone); got != filepath.Dir(standalone) {
		t.Fatal("standalone fallback crossed its directory")
	}
}

func TestPlayRefreshesStaleProjectBuffer(t *testing.T) {
	s := NewServer()
	root := t.TempDir()
	s.workspaceRoots = []string{root}
	path := filepath.Join(root, "project.yaml")
	uri := pathToDocumentURI(path)
	old := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata: {name: doclet, namespace: default}\nspec:\n  deploymentPipelineRef: {name: standard}\n"
	fixed := old + "  type: {kind: ProjectType, name: default}\n"
	if err := os.WriteFile(path, []byte(fixed), 0600); err != nil {
		t.Fatal(err)
	}
	s.OpenDocument(uri, old, 1)
	_, err := s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{Command: postRenderCommand, Arguments: []any{uri, []Document{{URI: uri, Text: fixed, Version: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.GetDocument(uri); got.Text != fixed || got.Version != 2 {
		t.Fatalf("stale buffer retained: %+v", got)
	}
	for _, diags := range s.postRenderByURI {
		for _, diag := range diags {
			if strings.Contains(diag.Message, "spec.type is required") {
				t.Fatalf("stale diagnostic: %+v", diag)
			}
		}
	}
	// An older request snapshot must not overwrite a more recent edit.
	_, err = s.ExecuteCommand(nil, &protocol.ExecuteCommandParams{Command: postRenderCommand, Arguments: []any{uri, []Document{{URI: uri, Text: old, Version: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if s.GetDocument(uri).Text != fixed {
		t.Fatal("older snapshot overwrote current content")
	}
}
