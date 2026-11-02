package lsp

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostgresWorkspaceDefinitionAndSchema(t *testing.T) {
	root := t.TempDir()
	relativeComponent := "namespaces/default/projects/doclet/components/postgres/component.yaml"
	relativeType := "namespaces/default/platform/component-types/database.yaml"
	source := filepath.Join("..", "..", "..", "..", "sample-gitops")
	var component, definition string
	for _, relative := range []string{relativeComponent, relativeType} {
		data, err := os.ReadFile(filepath.Join(source, relative))
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
		if relative == relativeComponent {
			component = string(data)
		} else {
			definition = string(data)
		}
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	if err := s.indexer.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, relativeComponent)
	uri := pathToDocumentURI(path)
	target := pathToDocumentURI(filepath.Join(root, relativeType))
	s.OpenDocument(uri, component, 1)
	var position protocol.Position
	for line, text := range strings.Split(component, "\n") {
		if column := strings.Index(text, "deployment/database"); column >= 0 {
			position = protocol.Position{Line: protocol.UInteger(line), Character: protocol.UInteger(column + 2)}
		}
	}
	result, err := s.Definition(nil, &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)}, Position: position}})
	if err != nil {
		t.Fatal(err)
	}
	locations, ok := result.([]protocol.Location)
	if !ok || len(locations) != 1 || string(locations[0].URI) != target {
		t.Fatalf("workspace definition not found: %+v", result)
	}
	invalid := strings.Replace(component, "port: 5432", "port: invalid", 1)
	if invalid == component {
		t.Fatal("sample port changed")
	}
	s.OpenDocument(uri, invalid, 2)
	s.triggerPreRenderSchemaValidationForPath(path)
	found := false
	for _, d := range s.preRenderSchemaByURI[uri] {
		if d.Code != nil && d.Code.Value == "schema-parameters-mismatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("sibling platform schema not applied: %+v", s.preRenderSchemaByURI)
	}
	// The unsaved definition changes the contract; the same value now passes.
	updated := strings.Replace(definition, "port:\n          type: integer", "port:\n          type: string", 1)
	if updated == definition {
		t.Fatal("sample schema layout changed")
	}
	s.OpenDocument(target, updated, 3)
	s.indexer.UpdateFile(target, []byte(updated))
	s.triggerPreRenderSchemaValidationForPath(filepath.Join(root, relativeType))
	for _, d := range s.preRenderSchemaByURI[uri] {
		if d.Code != nil && d.Code.Value == "schema-parameters-mismatch" {
			t.Fatalf("dependent error did not clear for unsaved definition: %+v", d)
		}
	}
}

func TestDefinitionUsesSecondDocumentNamespace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "database.yaml")
	definition := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: database, namespace: default}\nspec: {workloadType: deployment}\n"
	if err := os.WriteFile(path, []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	if err := s.indexer.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: unrelated, namespace: other}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: Component\nmetadata: {name: postgres, namespace: default}\nspec:\n  componentType:\n    kind: ComponentType\n    name: deployment/database\n"
	uri := pathToDocumentURI(filepath.Join(root, "component.yaml"))
	s.OpenDocument(uri, text, 1)
	result, err := s.Definition(nil, &protocol.DefinitionParams{TextDocumentPositionParams: protocol.TextDocumentPositionParams{TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)}, Position: protocol.Position{Line: 10, Character: 14}}})
	if err != nil {
		t.Fatal(err)
	}
	locations, ok := result.([]protocol.Location)
	if !ok || len(locations) != 1 || string(locations[0].URI) != pathToDocumentURI(path) {
		t.Fatalf("second document resolved using the wrong namespace: %+v", result)
	}
}
