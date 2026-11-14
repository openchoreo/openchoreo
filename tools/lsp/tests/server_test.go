// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"fmt"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
	lsputil "github.com/openchoreo/openchoreo/tools/lsp/lsp-util"
)

func TestDidOpenStoresDocument(t *testing.T) {
	fmt.Println("Running TestDidOpenStoresDocument")
	s := lsp.NewServer()

	params := &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        protocol.DocumentUri("file:///test.yaml"),
			LanguageID: "yaml",
			Version:    1,
			Text:       "kind: Component\nmetadata:\n  name: demo\n",
		},
	}

	if err := s.DidOpen(nil, params); err != nil {
		t.Fatalf("DidOpen returned error: %v", err)
	}

	doc := s.Document(params.TextDocument.URI)
	if doc == nil {
		t.Fatal("expected document to be stored after DidOpen")
	}
	if doc.URI != params.TextDocument.URI {
		t.Errorf("URI = %q, want %q", doc.URI, params.TextDocument.URI)
	}
	if doc.Text != params.TextDocument.Text {
		t.Errorf("Text = %q, want %q", doc.Text, params.TextDocument.Text)
	}
	if doc.Version != int(params.TextDocument.Version) {
		t.Errorf("Version = %d, want %d", doc.Version, params.TextDocument.Version)
	}
}

func TestDidOpenOverwritesExistingDocument(t *testing.T) {
	s := lsp.NewServer()

	uri := protocol.DocumentUri("file:///test.yaml")

	first := &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    "kind: Component\n",
		},
	}
	if err := s.DidOpen(nil, first); err != nil {
		t.Fatalf("DidOpen returned error: %v", err)
	}

	second := &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 2,
			Text:    "kind: Component\nmetadata:\n  name: updated\n",
		},
	}
	if err := s.DidOpen(nil, second); err != nil {
		t.Fatalf("DidOpen returned error: %v", err)
	}

	doc := s.Document(uri)
	if doc.Text != second.TextDocument.Text {
		t.Errorf("Text = %q, want %q", doc.Text, second.TextDocument.Text)
	}
	if doc.Version != 2 {
		t.Errorf("Version = %d, want 2", doc.Version)
	}
}

func TestDidOpenSeparateDocuments(t *testing.T) {
	s := lsp.NewServer()

	uris := []protocol.DocumentUri{
		protocol.DocumentUri("file:///a.yaml"),
		protocol.DocumentUri("file:///b.yaml"),
	}
	for i, uri := range uris {
		params := &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:     uri,
				Version: lsputil.Integer(i + 1),
				Text:    "kind: Component\n",
			},
		}
		fmt.Printf("Opening document %s with version %d\n", uri, params.TextDocument.Version)
		if err := s.DidOpen(nil, params); err != nil {
			t.Fatalf("DidOpen(%s) returned error: %v", uri, err)
		}
	}

	if s.Document("file:///a.yaml").Version != 1 {
		t.Errorf("a.yaml version = %d, want 1", s.Document("file:///a.yaml").Version)
	}
	if s.Document("file:///b.yaml").Version != 2 {
		t.Errorf("b.yaml version = %d, want 2", s.Document("file:///b.yaml").Version)
	}
}

func TestDidChangeUpdatesDocument(t *testing.T) {
	s := lsp.NewServer()

	uri := protocol.DocumentUri("file:///test.yaml")
	if err := s.DidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    "kind: Component\nname: demo\n",
		},
	}); err != nil {
		t.Fatalf("DidOpen returned error: %v", err)
	}

	updated := "kind: Component\nmetadata:\n  name: changed\n"
	if err := s.DidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                2,
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEventWhole{Text: updated},
		},
	}); err != nil {
		t.Fatalf("DidChange returned error: %v", err)
	}

	doc := s.Document(uri)
	if doc.Text != updated {
		t.Errorf("Text = %q, want %q", doc.Text, updated)
	}
	if doc.Version != 2 {
		t.Errorf("Version = %d, want 2", doc.Version)
	}
}

func TestDidChangeOnUnopenedDocument(t *testing.T) {
	s := lsp.NewServer()

	err := s.DidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{
				URI: protocol.DocumentUri("file:///never-opened.yaml"),
			},
			Version: 1,
		},
		ContentChanges: []any{
			protocol.TextDocumentContentChangeEventWhole{Text: "kind: Component\n"},
		},
	})

	if err == nil {
		t.Fatal("expected error for DidChange on unopened document, got nil")
	}
}

func TestDidCloseRemovesDocument(t *testing.T) {
	s := lsp.NewServer()

	uri := protocol.DocumentUri("file:///test.yaml")
	if err := s.DidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: 1,
			Text:    "kind: Component\n",
		},
	}); err != nil {
		t.Fatalf("DidOpen returned error: %v", err)
	}

	if s.Document(uri) == nil {
		t.Fatal("expected document to be stored before DidClose")
	}

	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	}); err != nil {
		t.Fatalf("DidClose returned error: %v", err)
	}

	if doc := s.Document(uri); doc != nil {
		t.Errorf("expected document to be removed after DidClose, got %+v", doc)
	}
}

func TestDidCloseRemovesOnlyTargetDocument(t *testing.T) {
	s := lsp.NewServer()

	keepURI := protocol.DocumentUri("file:///keep.yaml")
	closeURI := protocol.DocumentUri("file:///close.yaml")

	for _, uri := range []protocol.DocumentUri{keepURI, closeURI} {
		if err := s.DidOpen(nil, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{URI: uri, Version: 1, Text: "kind: Component\n"},
		}); err != nil {
			t.Fatalf("DidOpen(%s) returned error: %v", uri, err)
		}
	}

	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: closeURI},
	}); err != nil {
		t.Fatalf("DidClose returned error: %v", err)
	}

	if s.Document(keepURI) == nil {
		t.Error("expected keep.yaml to remain open")
	}
	if s.Document(closeURI) != nil {
		t.Error("expected close.yaml to be removed")
	}
}
