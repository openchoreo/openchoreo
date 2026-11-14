// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/openchoreo/openchoreo/tools/lsp"
)

func testLogger() (*slog.Logger, *strings.Builder) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, &buf
}

func fakeContext() *glsp.Context {
	return &glsp.Context{
		Method: "test",
		Notify: func(method string, params any) {},
	}
}

func TestWithRequestRecoveryCatchesPanic(t *testing.T) {
	s := lsp.NewServer()
	logger, buf := testLogger()
	s.SetLogger(logger)

	var calls int
	wrapped := lsp.WithRequestRecovery[protocol.DidOpenTextDocumentParams, any](s, "testRequest", func(ctx *glsp.Context, p *protocol.DidOpenTextDocumentParams) (any, error) {
		calls++
		panic("boom request")
	})

	_, err := wrapped(fakeContext(), &protocol.DidOpenTextDocumentParams{})
	if err == nil {
		t.Fatal("expected error from recovered panic, got nil")
	}
	if calls != 1 {
		t.Errorf("inner handler called %d times, want 1", calls)
	}
	if !strings.Contains(err.Error(), "boom request") {
		t.Errorf("error = %q, want to contain the panic value", err.Error())
	}
	if !strings.Contains(buf.String(), "internal error in testRequest") {
		t.Errorf("expected logged error, got:\n%s", buf.String())
	}
}

func TestWithHandlerRecoveryCatchesPanic(t *testing.T) {
	s := lsp.NewServer()
	logger, buf := testLogger()
	s.SetLogger(logger)

	var calls int
	wrapped := lsp.WithHandlerRecovery[protocol.DidOpenTextDocumentParams](s, "testNotify", func(ctx *glsp.Context, p *protocol.DidOpenTextDocumentParams) error {
		calls++
		panic("boom notify")
	})

	err := wrapped(fakeContext(), &protocol.DidOpenTextDocumentParams{})
	if err == nil {
		t.Fatal("expected error from recovered panic, got nil")
	}
	if calls != 1 {
		t.Errorf("inner handler called %d times, want 1", calls)
	}
	if !strings.Contains(err.Error(), "boom notify") {
		t.Errorf("error = %q, want to contain the panic value", err.Error())
	}
	if !strings.Contains(buf.String(), "internal error in testNotify") {
		t.Errorf("expected logged error, got:\n%s", buf.String())
	}
}

func TestWithRequestRecoveryPreservesResultOnSuccess(t *testing.T) {
	s := lsp.NewServer()
	s.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))

	wrapped := lsp.WithRequestRecovery[protocol.DidOpenTextDocumentParams, string](s, "okRequest", func(ctx *glsp.Context, p *protocol.DidOpenTextDocumentParams) (string, error) {
		return "hello", nil
	})

	result, err := wrapped(fakeContext(), &protocol.DidOpenTextDocumentParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q, want %q", result, "hello")
	}
}

func TestSetTraceUpdatesTraceValue(t *testing.T) {
	s := lsp.NewServer()
	defer protocol.SetTraceValue(protocol.TraceValueOff)

	protocol.SetTraceValue(protocol.TraceValueOff)
	if err := s.SetTrace(fakeContext(), &protocol.SetTraceParams{Value: protocol.TraceValueVerbose}); err != nil {
		t.Fatalf("SetTrace returned error: %v", err)
	}
	if got := protocol.GetTraceValue(); got != protocol.TraceValueVerbose {
		t.Errorf("trace value = %q, want %q", got, protocol.TraceValueVerbose)
	}
}
