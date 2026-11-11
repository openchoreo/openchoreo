// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/openchoreo/openchoreo/tools/lsp"
	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Debounced diagnostics are published over the retained JSON-RPC connection
// after a 250ms quiet window, so these tests drive the server exactly as a
// real client would and read publishDiagnostics frames off the wire.

const invalidComponent = "apiVersion: openchoreo.dev/v1alpha1\nkind: Component\nname: [unclosed\n"

// validComponent is a schema-conforming Component document (from the
// detect-test fixtures) that validates cleanly, i.e. produces zero
// diagnostics.
func validComponent(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../lint/testData/detect-test/config/component.yaml")
	if err != nil {
		t.Fatalf("read component fixture: %v", err)
	}
	return string(b)
}

type rxFrame struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// newDebouncedServer wires a fresh server to the client side of a net.Pipe
// and returns the client connection for reading frames.
func newDebouncedServer(t *testing.T) (*lsp.Server, net.Conn) {
	t.Helper()
	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() {
		serverSide.Close()
		clientSide.Close()
	})

	s := lsp.NewServer()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	conn := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(serverSide, jsonrpc2.VSCodeObjectCodec{}),
		jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
			return nil, nil
		}),
	)
	s.SetConnection(conn)
	t.Cleanup(func() { serverSide.Close(); clientSide.Close(); conn.Close() })
	return s, clientSide
}

func openDoc(t *testing.T, s *lsp.Server, uri, text string, version int32) {
	t.Helper()
	if err := s.DidOpen(nil, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:     protocol.DocumentUri(uri),
			Version: version,
			Text:    text,
		},
	}); err != nil {
		t.Fatalf("DidOpen error: %v", err)
	}
}

func changeDoc(t *testing.T, s *lsp.Server, uri string, version int32, changes ...any) {
	t.Helper()
	if err := s.DidChange(nil, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
			Version:                version,
		},
		ContentChanges: changes,
	}); err != nil {
		t.Fatalf("DidChange error: %v", err)
	}
}

// expectNoFrame asserts that no object arrives within the given duration.
func expectNoFrame(t *testing.T, client net.Conn, within time.Duration) {
	t.Helper()
	client.SetReadDeadline(time.Now().Add(within))
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	var frame rxFrame
	if err := stream.ReadObject(&frame); err == nil {
		t.Fatalf("unexpected frame %q within %v", frame.Method, within)
	}
	client.SetReadDeadline(time.Time{})
}

// readPublish reads the next frame, asserting it is a publishDiagnostics
// notification, and returns its params.
func readPublish(t *testing.T, client net.Conn, within time.Duration) protocol.PublishDiagnosticsParams {
	t.Helper()
	client.SetReadDeadline(time.Now().Add(within))
	stream := jsonrpc2.NewBufferedStream(client, jsonrpc2.VSCodeObjectCodec{})
	defer client.SetReadDeadline(time.Time{})

	for {
		var frame rxFrame
		if err := stream.ReadObject(&frame); err != nil {
			t.Fatalf("readPublish within %v: %v", within, err)
		}
		if frame.Method != protocol.ServerTextDocumentPublishDiagnostics {
			continue
		}
		var params protocol.PublishDiagnosticsParams
		if err := json.Unmarshal(frame.Params, &params); err != nil {
			t.Fatalf("unmarshal publish params: %v", err)
		}
		return params
	}
}

func TestDidChangeDelaysPublish(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///delay.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	changeDoc(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})

	// Nothing may be published before the debounce window elapses.
	expectNoFrame(t, client, 100*time.Millisecond)

	// One publish arrives after the window with the final version.
	params := readPublish(t, client, 800*time.Millisecond)
	if string(params.URI) != uri {
		t.Errorf("URI = %q, want %q", params.URI, uri)
	}
	if params.Version == nil || *params.Version != 2 {
		t.Errorf("Version = %v, want 2", params.Version)
	}
}

func TestDidChangeBurstCollapsesToSinglePublish(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///burst.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	// Three edits inside the debounce window.
	changeDoc(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})
	changeDoc(t, s, uri, 3, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})
	changeDoc(t, s, uri, 4, protocol.TextDocumentContentChangeEventWhole{Text: validComponent(t)})

	params := readPublish(t, client, 800*time.Millisecond)
	if params.Version == nil || *params.Version != 4 {
		t.Errorf("Version = %v, want 4 (the latest edit)", params.Version)
	}
	if len(params.Diagnostics) != 0 {
		t.Errorf("got %d diagnostics, want 0 for the fixed final text", len(params.Diagnostics))
	}

	// No trailing publish for the earlier, superseded edits.
	expectNoFrame(t, client, 350*time.Millisecond)
}

func TestDidChangeFixClearsDiagnostics(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///fix.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	// Introduce a parse error: exactly one diagnostics entry is expected.
	changeDoc(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})
	params := readPublish(t, client, 800*time.Millisecond)
	if len(params.Diagnostics) != 1 {
		t.Errorf("got %d diagnostics, want 1 for the malformed YAML", len(params.Diagnostics))
	}

	// Fix the document: the next publish must clear all diagnostics.
	changeDoc(t, s, uri, 3, protocol.TextDocumentContentChangeEventWhole{Text: validComponent(t)})
	params = readPublish(t, client, 800*time.Millisecond)
	if params.Version == nil || *params.Version != 3 {
		t.Errorf("Version = %v, want 3", params.Version)
	}
	if len(params.Diagnostics) != 0 {
		t.Errorf("got %d diagnostics, want 0 after the fix", len(params.Diagnostics))
	}
}

func TestDidCloseCancelsPendingPublish(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///cancel.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	changeDoc(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})

	if err := s.DidClose(nil, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: protocol.DocumentUri(uri)},
	}); err != nil {
		t.Fatalf("DidClose error: %v", err)
	}

	// The pending publish must not fire for a closed document.
	expectNoFrame(t, client, 500*time.Millisecond)
}

func TestFlushDiagnosticsPublishesImmediately(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///flush.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	changeDoc(t, s, uri, 2, protocol.TextDocumentContentChangeEventWhole{Text: invalidComponent})

	// FlushDiagnostics writes synchronously on the pair, so it must run in a
	// goroutine: the pipe is unbuffered, and readPublish below provides the
	// concurrent reader that the write needs to make progress.
	flushDone := make(chan struct{})
	go func() {
		s.FlushDiagnostics()
		close(flushDone)
	}()

	params := readPublish(t, client, 500*time.Millisecond)
	if params.Version == nil || *params.Version != 2 {
		t.Errorf("Version = %v, want 2", params.Version)
	}
	<-flushDone

	// The debounce timer was consumed by the flush: no second publish.
	expectNoFrame(t, client, 350*time.Millisecond)
}

func TestFlushDiagnosticsNoOpWithoutPending(t *testing.T) {
	s, client := newDebouncedServer(t)
	uri := "file:///flush-noop.yaml"
	openDoc(t, s, uri, invalidComponent, 1)

	s.FlushDiagnostics()

	expectNoFrame(t, client, 150*time.Millisecond)
}
