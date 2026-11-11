// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0
package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	clivalidator "github.com/openchoreo/openchoreo/tools/lint/cli-validator"
	"github.com/openchoreo/openchoreo/tools/lint/targetschema"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func schemaWorkspace(t *testing.T) (string, string, string, string, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{"project/componenttype.yaml": "componenttype.yaml", "platform/widget-crd.yaml": "widget-crd.yaml"}
	contents := map[string]string{}
	for relative, name := range files {
		data, err := os.ReadFile(filepath.Join("..", "lint", "targetschema", "testdata", "workspace", relative))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		contents[name] = string(data)
	}
	return root, filepath.Join(root, "project", "componenttype.yaml"), contents["componenttype.yaml"], filepath.Join(root, "platform", "widget-crd.yaml"), contents["widget-crd.yaml"]
}
func celTypeDiagnostics(diags []protocol.Diagnostic) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if d.Code != nil && d.Code.Value == "cel-type-error" {
			out = append(out, d)
		}
	}
	return out
}
func TestSchemaDiagnosticsSavedCLIParityAndUnsavedLifecycle(t *testing.T) {
	root, path, text, crdPath, crd := schemaWorkspace(t)
	s := NewServer()
	s.workspaceRoots = []string{root}
	if err := s.indexer.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	uri, crdURI := pathToDocumentURI(path), pathToDocumentURI(crdPath)
	s.OpenDocument(uri, text, 1)
	s.OpenDocument(crdURI, crd, 1)
	original := celTypeDiagnostics(s.DiagnosticsForText(uri, text))
	if len(original) != 1 {
		t.Fatalf("expected strict CRD field diagnostic: %+v", original)
	}
	sources, err := targetschema.ReadSources(root, true)
	if err != nil {
		t.Fatal(err)
	}
	results, err := clivalidator.ValidateYAML([]byte(text), targetschema.NewScope(root, sources))
	if err != nil {
		t.Fatal(err)
	}
	var cli []protocol.Diagnostic
	for _, r := range results {
		for _, d := range r.Diagnostics {
			if d.Code == "cel-type-error" {
				cli = append(cli, toLSPDiagnostic(d))
			}
		}
	}
	if len(cli) != 1 || cli[0].Message != original[0].Message || cli[0].Range != original[0].Range {
		t.Fatalf("CLI/LSP parity: CLI=%+v LSP=%+v", cli, original)
	}
	updated := strings.Replace(crd, "count:", "coutn:", 1)
	if err := s.DidChange(nil, &protocol.DidChangeTextDocumentParams{TextDocument: protocol.VersionedTextDocumentIdentifier{TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(crdURI)}, Version: 2}, ContentChanges: []any{protocol.TextDocumentContentChangeEventWhole{Text: updated}}}); err != nil {
		t.Fatal(err)
	}
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 0 {
		t.Fatalf("unsaved CRD edit did not clear dependent error: %+v", got)
	}
	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(crdURI)}}); err != nil {
		t.Fatal(err)
	}
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 1 {
		t.Fatalf("closing must restore the saved CRD contract: %+v", got)
	}
	// Invalid unsaved schema text must not resurrect the saved schema.
	s.OpenDocument(crdURI, "apiVersion: apiextensions.k8s.io/v1\nspec: [", 3)
	_ = s.indexer.UpdateFile(crdURI, []byte("apiVersion: apiextensions.k8s.io/v1\nspec: ["))
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 0 {
		t.Fatalf("malformed overlay resurrected saved schema: %+v", got)
	}
}

func TestSchemaWorkspaceIsolation(t *testing.T) {
	first, path, text, _, _ := schemaWorkspace(t)
	second, _, _, crdPath, crd := schemaWorkspace(t)
	if err := os.WriteFile(crdPath, []byte(strings.Replace(crd, "count:", "coutn:", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{first, second}
	if err := s.indexer.IndexWorkspaces([]string{first, second}); err != nil {
		t.Fatal(err)
	}
	if got := celTypeDiagnostics(s.DiagnosticsForText(pathToDocumentURI(path), text)); len(got) != 1 {
		t.Fatalf("schema leaked across workspace roots: %+v", got)
	}
}

func TestSchemaCatalogDiskChangesInvalidateConsumer(t *testing.T) {
	root, path, text, _, _ := schemaWorkspace(t)
	catalogPath := filepath.Join(t.TempDir(), "widget.json")
	catalog := `{"swagger":"2.0","definitions":{"Widget":{"type":"object","x-kubernetes-group-version-kind":[{"group":"example.com","version":"v1","kind":"Widget"}],"properties":{"spec":{"type":"object","properties":{"count":{"type":"integer"}}}}}}}`
	if err := os.WriteFile(catalogPath, []byte(catalog), 0600); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, targetschema.ConfigName)
	config := "version: 1\nschemaCatalogs:\n  - " + catalogPath + "\n"
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	if err := s.indexer.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	uri := pathToDocumentURI(path)
	s.OpenDocument(uri, text, 1)
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 1 {
		t.Fatalf("initial catalog diagnostic: %+v", got)
	}
	if !s.isSchemaDependency(catalogPath) {
		t.Fatal("external catalog was not registered as a dependency")
	}
	if err := os.WriteFile(catalogPath, []byte(strings.Replace(catalog, `"count"`, `"coutn"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	s.processWatchChange(catalogPath)
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 0 {
		t.Fatalf("catalog change retained stale error: %+v", got)
	}
	if err := os.WriteFile(configPath, []byte("version: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.processWatchChange(configPath)
	if got := celTypeDiagnostics(s.DiagnosticsForText(uri, text)); len(got) != 1 {
		t.Fatalf("config change failed to restore workspace schema: %+v", got)
	}
}

func TestSchemaIndexRevisionPreservesParseErrors(t *testing.T) {
	s := NewServer()
	uri := "file:///schema-parse-error.yaml"
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nname: [unclosed\n"
	first := s.DiagnosticsForText(uri, text)
	if len(first) != 1 {
		t.Fatalf("initial parse error missing: %+v", first)
	}
	_ = s.IndexFile("file:///unrelated.yaml", []byte("apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: example}\n"))
	after := s.DiagnosticsForText(uri, text)
	if len(after) != 1 || after[0].Message != first[0].Message {
		t.Fatalf("schema revision cleared parse error: %+v", after)
	}
}
