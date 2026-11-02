// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/tools/lsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// indexServer creates a server whose workspace index is populated from every
// YAML file under dir (mirrors the runtime rootUri indexing on initialize).
// Indexing runs asynchronously, so the returned server's index is guaranteed
// warm (Indexed closed).
func indexServer(t *testing.T, dir string) *lsp.Server {
	t.Helper()
	s := lsp.NewServer()
	rootURI := "file://" + dir
	if _, err := s.Initialize(nil, &protocol.InitializeParams{RootURI: &rootURI}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Indexed():
	case <-time.After(10 * time.Second):
		t.Fatal("workspace index did not finish in time")
	}
	return s
}

// codeActions opens a document on a fresh server and returns every quick fix
// the server offers for it. The request range spans the whole document so fixes
// anchored to any line are included.
func codeActions(t *testing.T, uri, text string) []protocol.CodeAction {
	t.Helper()
	s := lsp.NewServer()
	s.OpenDocument(uri, text, 1)
	res, err := s.CodeAction(nil, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(text, "\n"))), Character: 0},
		},
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, ok := res.([]protocol.CodeAction)
	if !ok {
		t.Fatalf("expected []CodeAction, got %T", res)
	}
	return actions
}

// hasTitle reports whether any action carries the given title.
func hasTitle(actions []protocol.CodeAction, title string) bool {
	for _, a := range actions {
		if a.Title == title {
			return true
		}
	}
	return false
}

func TestCodeActionMissingAPIVersion(t *testing.T) {
	// A file without an OpenChoreo apiVersion is not an OpenChoreo document, so
	// no validation runs and no apiVersion creation fix is offered.
	actions := codeActions(t, "file:///a.yaml", "kind: Project\nmetadata:\n  name: default\n")
	if hasTitle(actions, "Add apiVersion: openchoreo.dev/v1alpha1") {
		t.Fatalf("expected no apiVersion fix for a non-OpenChoreo file, got: %+v", actions)
	}
}

func TestCodeActionInvalidAPIVersion(t *testing.T) {
	// openchoreo.dev/... passes the apiVersion gate, but a version other than
	// v1alpha1 still triggers the invalid-apiVersion fix.
	actions := codeActions(t, "file:///a.yaml", "apiVersion: openchoreo.dev/v2\nkind: Project\nmetadata:\n  name: default\n")
	var edit *protocol.TextEdit
	for _, a := range actions {
		if a.Title == "Fix apiVersion to openchoreo.dev/v1alpha1" {
			edit = &a.Edit.Changes["file:///a.yaml"][0]
		}
	}
	if edit == nil {
		t.Fatalf("expected invalid apiVersion fix, got: %+v", actions)
	}
	if edit.NewText != "apiVersion: openchoreo.dev/v1alpha1" {
		t.Fatalf("bad edit: %+v", edit)
	}
}

func TestCodeActionMissingMetadataName(t *testing.T) {
	actions := codeActions(t, "file:///a.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  namespace: default\n")
	if !hasTitle(actions, "Add metadata.name") {
		t.Fatalf("expected missing metadata.name fix, got: %+v", actions)
	}
}

func TestCodeActionValidDocumentNoFixes(t *testing.T) {
	actions := codeActions(t, "file:///x.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: default\n  namespace: default\n")
	if len(actions) != 0 {
		t.Fatalf("expected no quick fixes for a valid document, got: %+v", actions)
	}
}

func TestCodeActionSchemaKindFix(t *testing.T) {
	actions := codeActions(t, "file:///k.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Projct\nmetadata:\n  name: p1\n")
	for _, a := range actions {
		if strings.Contains(a.Title, "Change kind") {
			edits := a.Edit.Changes["file:///k.yaml"]
			if len(edits) == 0 || edits[0].NewText != "Project" {
				t.Fatalf("expected kind fix to 'Project', got: %+v", a)
			}
			return
		}
	}
	t.Fatalf("expected a schema-driven kind fix for 'Projct', got: %+v", actions)
}

func TestCodeActionSchemaMetadataTypo(t *testing.T) {
	actions := codeActions(t, "file:///m.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  namess: p1\n")
	for _, a := range actions {
		if strings.Contains(a.Title, `change "namess" to "name"`) {
			return
		}
	}
	t.Fatalf("expected a metadata field spelling fix, got: %+v", actions)
}

func TestCodeActionSchemaRootFieldTypo(t *testing.T) {
	actions := codeActions(t, "file:///p.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: p1\nmetdata:\n  zzz: 1\n")
	for _, a := range actions {
		if strings.Contains(a.Title, `change "metdata" to "metadata"`) {
			return
		}
	}
	t.Fatalf("expected a root field spelling fix, got: %+v", actions)
}

func TestCodeActionSchemaMissingRequired(t *testing.T) {
	actions := codeActions(t, "file:///p.yaml", "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: p1\nspec:\n  parameters:\n    env: dev\n")
	if !hasTitle(actions, "Add deploymentPipelineRef") {
		t.Fatalf("expected a missing required field fix, got: %+v", actions)
	}
}

func TestCodeActionFileNameRefTypo(t *testing.T) {
	dir := t.TempDir()
	// Indexed resource files; their file names are the "correct" reference values.
	refs := map[string]string{
		"docker-gitops-release.yaml": "apiVersion: openchoreo.dev/v1alpha1\nkind: Workflow\nmetadata:\n  name: docker-gitops-release\n",
		"standard.yaml":              "apiVersion: openchoreo.dev/v1alpha1\nkind: DeploymentPipeline\nmetadata:\n  name: standard\n",
	}
	for name, content := range refs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := indexServer(t, dir)

	uri := fileURI(filepath.Join(dir, "p.yaml"))
	// Reference `name` fields at different nesting points are misspelt; the
	// document's own `metadata.name` (line 3) must NOT be "fixed".
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: p1\nspec:\n  allowedWorkflows:\n    - kind: Workflow\n      name: docker-gitopsdd-release\n  deploymentPipelineRef:\n    name: standrd\n"
	s.OpenDocument(uri, text, 1)
	// Request quick fixes ONLY over the metadata section (lines 0-4): the
	// file-name fixes on the reference lines must still be offered.
	res, err := s.CodeAction(nil, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 0},
			End:   protocol.Position{Line: 5, Character: 0},
		},
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, ok := res.([]protocol.CodeAction)
	if !ok {
		t.Fatalf("expected []CodeAction, got %T", res)
	}

	gotRef, gotWorkflow := false, false
	for _, a := range actions {
		switch a.Title {
		case `Change name to "docker-gitops-release"`:
			gotRef = true
			edits := a.Edit.Changes[uri]
			if len(edits) == 0 || edits[0].NewText != "docker-gitops-release" {
				t.Fatalf("expected name fix to 'docker-gitops-release', got: %+v", a)
			}
		case `Change name to "standard"`:
			gotWorkflow = true
		case `Change name to "p1"`:
			t.Fatalf("document's own metadata.name must not be given a file-name fix, got %+v", a)
		}
	}
	if !gotRef || !gotWorkflow {
		t.Fatalf("expected file-name fixes for both reference fields, got: %+v", actions)
	}
}

func TestCodeActionFileNameValidNoFix(t *testing.T) {
	dir := t.TempDir()
	existing := "apiVersion: openchoreo.dev/v1alpha1\nkind: DeploymentPipeline\nmetadata:\n  name: standard\n"
	if err := os.WriteFile(filepath.Join(dir, "standard.yaml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	s := indexServer(t, dir)

	uri := "file:///p2.yaml"
	// The ref already matches an indexed file name: no fix expected.
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Project\nmetadata:\n  name: p1\nspec:\n  deploymentPipelineRef:\n    name: standard\n"
	s.OpenDocument(uri, text, 1)
	res, err := s.CodeAction(nil, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(text, "\n"))), Character: 0},
		},
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, ok := res.([]protocol.CodeAction)
	if !ok {
		t.Fatalf("expected []CodeAction, got %T", res)
	}

	for _, a := range actions {
		if strings.HasPrefix(a.Title, "Change name to") {
			t.Fatalf("expected no file-name fix for a correct name, got %q", a.Title)
		}
	}
}

func TestCodeActionFileNameRefAllowedTraitsAndWorkflows(t *testing.T) {
	dir := t.TempDir()
	refs := map[string]string{
		"api-configuration.yaml":     "apiVersion: openchoreo.dev/v1alpha1\nkind: Trait\nmetadata:\n  name: api-configuration\n",
		"docker-gitops-release.yaml": "apiVersion: openchoreo.dev/v1alpha1\nkind: Workflow\nmetadata:\n  name: docker-gitops-release\n",
	}
	for name, content := range refs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := indexServer(t, dir)

	uri := fileURI(filepath.Join(dir, "dp.yaml"))
	// Mirrors the user's reported scenario: a `name` typo inside an
	// allowedTraits item (plain `name`) and a `name` typo inside an
	// allowedWorkflows item (with a sibling `kind`).
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: DeploymentPipeline\n" +
		"metadata:\n" +
		"  name: dp1\n" +
		"spec:\n" +
		"  allowedTraits:\n" +
		"    - name: api-configuraton\n" +
		"  allowedWorkflows:\n" +
		"    - kind: Workflow\n" +
		"      name: docker-gitops-rvelease\n"
	s.OpenDocument(uri, text, 1)
	res, err := s.CodeAction(nil, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(text, "\n"))), Character: 0},
		},
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, ok := res.([]protocol.CodeAction)
	if !ok {
		t.Fatalf("expected []CodeAction, got %T", res)
	}

	gotTrait, gotWorkflow := false, false
	for _, a := range actions {
		switch a.Title {
		case `Change name to "api-configuration"`:
			gotTrait = true
		case `Change name to "docker-gitops-release"`:
			gotWorkflow = true
		}
	}
	if !gotTrait || !gotWorkflow {
		t.Fatalf("expected file-name fixes for both allowedTraits and allowedWorkflows name typos, got: %+v", actions)
	}
}

// TestCodeActionFileNameRefMatchesMetadataName verifies the target candidate
// list includes indexed resource metadata.names, not only file basenames: a
// resource whose FILE is named differently from its metadata.name must still be
// a valid fix target for a misspelt reference.
func TestCodeActionFileNameRefMatchesMetadataName(t *testing.T) {
	dir := t.TempDir()
	// The file is NOT named docker-gitops-release.yaml.
	existing := "apiVersion: openchoreo.dev/v1alpha1\nkind: Workflow\nmetadata:\n  name: docker-gitops-release\n"
	if err := os.WriteFile(filepath.Join(dir, "my-workflow.yaml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	s := indexServer(t, dir)

	uri := fileURI(filepath.Join(dir, "dp.yaml"))
	text := "apiVersion: openchoreo.dev/v1alpha1\n" +
		"kind: DeploymentPipeline\n" +
		"metadata:\n" +
		"  name: dp1\n" +
		"spec:\n" +
		"  allowedWorkflows:\n" +
		"    - kind: Workflow\n" +
		"      name: docker-gitops-releasne\n"
	s.OpenDocument(uri, text, 1)
	res, err := s.CodeAction(nil, &protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(text, "\n"))), Character: 0},
		},
		Context: protocol.CodeActionContext{Diagnostics: []protocol.Diagnostic{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, ok := res.([]protocol.CodeAction)
	if !ok {
		t.Fatalf("expected []CodeAction, got %T", res)
	}

	for _, a := range actions {
		if a.Title == `Change name to "docker-gitops-release"` {
			return
		}
	}
	t.Fatalf("expected a file-name fix matching the indexed metadata.name, got: %+v", actions)
}
