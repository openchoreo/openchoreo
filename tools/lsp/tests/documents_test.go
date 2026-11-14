// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

func TestPositionToByteOffset(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		pos     protocol.Position
		want    int
		wantErr bool
	}{
		{name: "first line start", text: "abc\ndef\n", pos: p(0, 0), want: 0},
		{name: "first line middle", text: "abc\ndef\n", pos: p(0, 2), want: 2},
		{name: "second line start", text: "abc\ndef\n", pos: p(1, 0), want: 4},
		{name: "last line no trailing newline", text: "abc\ndef", pos: p(1, 3), want: 7},
		{name: "position after last char clamps", text: "abc\n", pos: p(0, 99), want: 3},
		{name: "line out of range", text: "abc\n", pos: p(5, 0), wantErr: true},
		{name: "crlf content counting", text: "a\r\nb\r\n", pos: p(1, 1), want: 4},
		{name: "crlf first line", text: "ab\r\nc\r\n", pos: p(0, 1), want: 1},
		{name: "utf16 ascii rune", text: "héllo\n", pos: p(0, 1), want: 1},
		{name: "utf16 surrogate pair before", text: "a\U0001F600b\n", pos: p(0, 3), want: 5},
		{name: "utf16 inside surrogate pair", text: "a\U0001F600b\n", pos: p(0, 1), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := lsp.PositionToByteOffset(tt.text, lsp.LineStartOffsets(tt.text), tt.pos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("PositionToByteOffset(%q, %+v) = %d, want error", tt.text, tt.pos, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("PositionToByteOffset(%q, %+v) error: %v", tt.text, tt.pos, err)
			}
			if got != tt.want {
				t.Errorf("PositionToByteOffset(%q, %+v) = %d, want %d", tt.text, tt.pos, got, tt.want)
			}
		})
	}
}

func TestDidChangeWhole(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///whole.yaml"
	open(t, s, uri, "kind: Component\nname: demo\n")

	updated := "kind: Deployment\nname: demo\n"
	if err := change(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: updated}); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	if doc.Text != updated {
		t.Errorf("Text = %q, want %q", doc.Text, updated)
	}
	if doc.Version != 2 {
		t.Errorf("Version = %d, want 2", doc.Version)
	}
}

func TestDidChangeRangedMissingRangeIsWhole(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///whole.yaml"
	open(t, s, uri, "kind: Component\n")

	updated := "kind: Deployment\n"
	if err := change(t, s, uri, 2, protocol.TextDocumentContentChangeEvent{Text: updated}); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	if doc.Text != updated {
		t.Errorf("Text = %q, want %q", doc.Text, updated)
	}
}

func TestDidChangeSingleLineRange(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///ranged.yaml"
	open(t, s, uri, "kind: Component\nname: demo\n")

	if err := change(t, s, uri, 2,
		ranged(p(0, 6), p(0, 15), "Deployment"),
	); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "kind: Deployment\nname: demo\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeMultiLineRange(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///multi.yaml"
	open(t, s, uri, "kind: Component\nname: demo\n")

	if err := change(t, s, uri, 2,
		// Replace "kind: Component\n" (through the newline) with new content.
		ranged(p(0, 0), p(1, 0), "version: 1\n"),
	); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "version: 1\nname: demo\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeDeleteToEOF(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///eof.yaml"
	open(t, s, uri, "kind: Component\nname: demo\n")

	// Delete from "name:" to the end of the document.
	if err := change(t, s, uri, 2, ranged(p(1, 0), p(2, 0), "")); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "kind: Component\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeInsertAtEOF(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///append.yaml"
	open(t, s, uri, "abc\n")

	if err := change(t, s, uri, 2, ranged(p(1, 0), p(1, 0), "def")); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "abc\ndef"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeUTF16(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///utf16.yaml"
	// \U0001F600 counts as two UTF-16 code units in the LSP position scheme.
	open(t, s, uri, "a\U0001F600b\n")

	if err := change(t, s, uri, 2, ranged(p(0, 1), p(0, 3), "X")); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "aXb\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeCRLF(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///crlf.yaml"
	open(t, s, uri, "a\r\nb\r\n")

	if err := change(t, s, uri, 2, ranged(p(0, 0), p(0, 1), "z")); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "z\r\nb\r\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
}

func TestDidChangeSequential(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///seq.yaml"
	open(t, s, uri, "kind: Component\nname: demo\n")

	if err := change(t, s, uri, 3,
		ranged(p(0, 6), p(0, 15), "Deployment"),
		ranged(p(1, 6), p(1, 10), "updated"),
	); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}

	doc := s.Document(uri)
	want := "kind: Deployment\nname: updated\n"
	if doc.Text != want {
		t.Errorf("Text = %q, want %q", doc.Text, want)
	}
	if doc.Version != 3 {
		t.Errorf("Version = %d, want 3", doc.Version)
	}
}

func TestDidChangeInvalidRange(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///invalid.yaml"
	open(t, s, uri, "kind: Component\n")

	err := change(t, s, uri, 2, ranged(p(99, 0), p(99, 0), "x"))
	if err == nil {
		t.Fatal("expected error for out-of-range line, got nil")
	}
	if !strings.Contains(err.Error(), "out of range") {
		t.Errorf("error = %v, want mention of out of range", err)
	}
}

func TestDidChangeReversedRange(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///reversed.yaml"
	open(t, s, uri, "kind: Component\n")

	err := change(t, s, uri, 2, ranged(p(0, 5), p(0, 2), "x"))
	if err == nil {
		t.Fatal("expected error for reversed range, got nil")
	}
}

func TestDidChangeUnknownType(t *testing.T) {
	s := lsp.NewServer()
	uri := "file:///unknown.yaml"
	open(t, s, uri, "kind: Component\n")

	err := change(t, s, uri, 2, "not a change event")
	if err == nil {
		t.Fatal("expected error for unknown change type, got nil")
	}
}

func TestDidChangeUnopenedDocument(t *testing.T) {
	s := lsp.NewServer()

	err := change(t, s, "file:///never-opened.yaml", 1,
		protocol.TextDocumentContentChangeEventWhole{Text: "kind: Component\n"})
	if err == nil {
		t.Fatal("expected error for unopened document, got nil")
	}
}

func p(line, character uint32) protocol.Position {
	return protocol.Position{Line: line, Character: character}
}

func ranged(start, end protocol.Position, text string) protocol.TextDocumentContentChangeEvent {
	return protocol.TextDocumentContentChangeEvent{
		Range: &protocol.Range{Start: start, End: end},
		Text:  text,
	}
}

func open(t *testing.T, s *lsp.Server, uri string, text string) {
	const version = 1

	t.Helper()
	if err := s.DidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     uri,
			Version: version,
			Text:    text,
		},
	}); err != nil {
		t.Fatalf("DidOpen error: %v", err)
	}
}

func change(t *testing.T, s *lsp.Server, uri string, version int32, changes ...any) error {
	t.Helper()
	return s.DidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: changes,
	})
}
