// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/sourcegraph/jsonrpc2"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

// TestSendAsyncAfterHandlerReturn proves the retained connection can push a
// notification asynchronously (i.e. after a request handler has returned),
// which is what debounced diagnostics and $/progress rely on.
func TestSendAsyncAfterHandlerReturn(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	s := lsp.NewServer()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A real, running connection bound to the server side of the pipe, as
	// Start() would set up via GetStdio().
	conn := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(serverSide, jsonrpc2.VSCodeObjectCodec{}),
		jsonrpc2.HandlerWithError(func(ctx context.Context, c *jsonrpc2.Conn, req *jsonrpc2.Request) (any, error) {
			return nil, nil
		}),
	)
	defer conn.Close()
	s.SetConnection(conn)

	// Simulate an async task firing after the originating handler has returned.
	// net.Pipe is unbuffered, so the client must read concurrently with the send.
	clientStream := jsonrpc2.NewBufferedStream(clientSide, jsonrpc2.VSCodeObjectCodec{})
	if err := clientSide.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var frame struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	readErrCh := make(chan error, 1)
	go func() {
		readErrCh <- clientStream.ReadObject(&frame)
	}()

	sendDone := make(chan struct{})
	go func() {
		s.SendAsync(protocol.ServerWindowLogMessage, &protocol.LogMessageParams{
			Type:    protocol.MessageTypeInfo,
			Message: "async after handler return",
		})
		close(sendDone)
	}()
	<-sendDone

	if err := <-readErrCh; err != nil {
		t.Fatalf("failed to read async notification: %v", err)
	}

	if frame.Method != protocol.ServerWindowLogMessage {
		t.Errorf("method = %q, want %q", frame.Method, protocol.ServerWindowLogMessage)
	}
	var params protocol.LogMessageParams
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		t.Fatalf("failed to unmarshal params: %v", err)
	}
	if params.Message != "async after handler return" {
		t.Errorf("message = %q, want %q", params.Message, "async after handler return")
	}
}

// TestSendAsyncNoConnection is a no-op safety check: before Start() there is
// no connection, so SendAsync must not panic or block.
func TestSendAsyncNoConnection(t *testing.T) {
	s := lsp.NewServer()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SendAsync(protocol.ServerWindowLogMessage, &protocol.LogMessageParams{})
}
