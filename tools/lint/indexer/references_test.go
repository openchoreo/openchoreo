// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScopedComponentTypeReference(t *testing.T) {
	root := t.TempDir()
	component := filepath.Join(root, "namespaces", "default", "projects", "doclet", "components", "postgres", "component.yaml")
	definition := filepath.Join(root, "namespaces", "default", "platform", "component-types", "database.yaml")
	if err := os.MkdirAll(filepath.Dir(definition), 0700); err != nil {
		t.Fatal(err)
	}
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata:\n  name: database\n  namespace: default\nspec:\n  workloadType: deployment\n"
	if err := os.WriteFile(definition, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer()
	if err := idx.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	uri := pathToFileURI(component)
	defURI := pathToFileURI(definition)
	if got := idx.FindScopedDefinition("ComponentType", "default", "deployment/database", uri); len(got) != 1 || got[0].URI != defURI {
		t.Fatalf("logical reference not resolved: %+v", got)
	}
	if got := idx.FindScopedDefinition("ComponentType", "other", "deployment/database", uri); len(got) != 0 {
		t.Fatal("namespace crossed")
	}
	if got := idx.FindScopedDefinition("ComponentType", "default", "statefulset/database", uri); len(got) != 0 {
		t.Fatal("workload prefix ignored")
	}
	if err := idx.UpdateFile(defURI, []byte{}); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindScopedDefinition("ComponentType", "default", "deployment/database", uri); len(got) != 0 {
		t.Fatal("empty unsaved buffer resurrected disk definition")
	}
}

func TestScopedMultiDocumentClusterAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "types.yaml")
	text := "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata: {name: database, namespace: default}\nspec: {workloadType: deployment}\n---\napiVersion: openchoreo.dev/v1alpha1\nkind: ClusterComponentType\nmetadata: {name: database}\nspec: {workloadType: deployment}\n"
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	idx := NewIndexer()
	if err := idx.IndexWorkspace(root); err != nil {
		t.Fatal(err)
	}
	source := pathToFileURI(filepath.Join(root, "components", "postgres.yaml"))
	if got := idx.FindScopedDefinition("ClusterComponentType", "different", "deployment/database", source); len(got) != 1 {
		t.Fatalf("cluster lookup: %+v", got)
	}
	if got := idx.Snapshot(source); len(got) != 2 || got[0].Document.DocumentIndex == got[1].Document.DocumentIndex {
		t.Fatal("multi-document identity lost")
	}
	uri := pathToFileURI(filepath.Join(root, "duplicate.yaml"))
	if err := idx.UpdateFile(uri, []byte(text)); err != nil {
		t.Fatal(err)
	}
	if got := idx.FindScopedDefinition("ComponentType", "default", "deployment/database", source); len(got) != 2 {
		t.Fatal("ambiguous candidates were discarded")
	}
	idx.RemoveFile(uri)
	if got := idx.FindScopedDefinition("ComponentType", "default", "deployment/database", source); len(got) != 1 {
		t.Fatal("deleted duplicate remained cached")
	}
}
