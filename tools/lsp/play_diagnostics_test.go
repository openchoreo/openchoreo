// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestUnchangedWatchEventsPreservePlayFindings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "environment.yaml")
	uri := pathToDocumentURI(path)
	text := demoEnvironmentYAML
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	s.watchRoots = []string{root}
	if err := s.indexer.UpdateFile(uri, []byte(text)); err != nil {
		t.Fatal(err)
	}
	seed := func() {
		s.postRenderByURI = map[string][]protocol.Diagnostic{uri: {{Message: "manual failure"}}}
		s.postRenderFiles = map[string]struct{}{path: {}}
		s.postRenderScope = root
	}
	seed()
	revision, epoch := s.indexer.Version(), s.postRenderEpoch
	s.processWatchChange(path)
	s.resyncIndexWithDisk()
	if len(s.postRenderByURI[uri]) != 1 || s.postRenderEpoch != epoch || s.indexer.Version() != revision {
		t.Fatal("unchanged disk events erased Play results or changed index revision")
	}
	s.OpenDocument(uri, text, 1)
	s.processWatchChange(path)
	if len(s.postRenderByURI[uri]) != 1 {
		t.Fatal("save event for open document erased Play results")
	}
	s.CloseDocument(uri)
	if err := os.WriteFile(path, []byte(text+"spec: {isProduction: true}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s.processWatchChange(path)
	s.postRenderMu.Lock()
	remaining := len(s.postRenderByURI)
	s.postRenderMu.Unlock()
	if remaining != 0 {
		t.Fatal("actual content edit retained stale Play findings")
	}
}

func TestClosedPlayPublicationRefreshesStaleStaticLane(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "environment.yaml")
	uri := pathToDocumentURI(path)
	old := "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: demo}\nspec: {isProduction: wrong}\n"
	corrected := "apiVersion: openchoreo.dev/v1alpha1\nkind: Environment\nmetadata: {name: demo}\nspec: {isProduction: false}\n"
	if err := os.WriteFile(path, []byte(corrected), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	s.diagnosticsForCached(uri, old)
	s.workspaceStaticByURI = map[string][]protocol.Diagnostic{uri: s.diagnosticsForCached(uri, old)}
	if err := s.indexer.UpdateFile(uri, []byte(corrected)); err != nil {
		t.Fatal(err)
	}
	s.postRenderByURI = map[string][]protocol.Diagnostic{uri: {{Message: "manual failure"}}}
	if s.validationSnapshotCurrent(uri, 0) {
		t.Fatal("expected stale automatic cache")
	}
	s.republishCurrentDiagnostics(uri)
	if !s.validationSnapshotCurrent(uri, 0) {
		t.Fatal("Play publication would still be suppressed by stale cache")
	}
	for _, d := range s.workspaceStaticByURI[uri] {
		if d.Message != "" && d.Code != nil && d.Code.Value == "invalid-environment.spec.isProduction" {
			t.Fatal("stale structural diagnostic survived refresh")
		}
	}
	if len(s.postRenderByURI[uri]) != 1 {
		t.Fatal("automatic refresh removed manual findings")
	}
}

func TestClosingSavedDocumentPreservesPlayFindings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "environment.yaml")
	uri := pathToDocumentURI(path)
	text := demoEnvironmentYAML
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	seed := func() {
		s.postRenderMu.Lock()
		s.postRenderByURI = map[string][]protocol.Diagnostic{uri: {{Message: "manual failure"}}}
		s.postRenderFiles = map[string]struct{}{path: {}}
		s.postRenderScope = root
		s.postRenderMu.Unlock()
	}
	s.OpenDocument(uri, text, 1)
	_ = s.indexer.UpdateFile(uri, []byte(text))
	seed()
	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}}); err != nil {
		t.Fatal(err)
	}
	s.postRenderMu.Lock()
	retained := len(s.postRenderByURI[uri])
	s.postRenderMu.Unlock()
	if retained != 1 {
		t.Fatal("closing saved buffer erased Play findings")
	}
	s.OpenDocument(uri, text+"spec: {isProduction: true}\n", 2)
	seed()
	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{TextDocument: protocol.TextDocumentIdentifier{URI: uri}}); err != nil {
		t.Fatal(err)
	}
	s.postRenderMu.Lock()
	remaining := len(s.postRenderByURI)
	s.postRenderMu.Unlock()
	if remaining != 0 {
		t.Fatal("discarding unsaved content retained stale Play findings")
	}
}

func TestLateWatchEventMatchingRenderedSnapshotPreservesFindings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "environment.yaml")
	uri := pathToDocumentURI(path)
	old := demoEnvironmentYAML
	rendered := old + "spec: {isProduction: true}\n"
	if err := os.WriteFile(path, []byte(rendered), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewServer()
	s.workspaceRoots = []string{root}
	_ = s.indexer.UpdateFile(uri, []byte(old))
	s.postRenderByURI = map[string][]protocol.Diagnostic{uri: {{Message: "manual failure"}}}
	s.postRenderFiles = map[string]struct{}{path: {}}
	s.postRenderScope = root
	s.postRenderContentHashes = map[string][32]byte{path: sha256.Sum256([]byte(rendered))}
	s.processWatchChange(path)
	s.postRenderMu.Lock()
	retained := len(s.postRenderByURI[uri])
	s.postRenderMu.Unlock()
	if retained != 1 {
		t.Fatal("late index update erased a result rendered from the same content")
	}
}
