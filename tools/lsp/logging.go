// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp

import (
	"fmt"
	"runtime/debug"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// notifyLog sends a window/logMessage notification to the client. Log messages
// are gated by the client's trace setting (see SetTrace).
func (s *Server) notifyLog(ctx *glsp.Context, messageType protocol.MessageType, message string) {
	if ctx == nil {
		return
	}
	if !protocol.HasTraceMessageType(messageType) {
		return
	}
	ctx.Notify(protocol.ServerWindowLogMessage, &protocol.LogMessageParams{
		Type:    messageType,
		Message: message,
	})
}

// notifyShow sends a window/showMessage notification to the client. These are
// always delivered so the user sees important error/info messages.
func (s *Server) notifyShow(ctx *glsp.Context, messageType protocol.MessageType, message string) {
	if ctx == nil {
		return
	}
	ctx.Notify(protocol.ServerWindowShowMessage, &protocol.ShowMessageParams{
		Type:    messageType,
		Message: message,
	})
}

// logAsync sends a window/logMessage via the retained connection, for use from
// background goroutines after handlers have returned. It is gated by the
// client's trace setting, same as notifyLog.
func (s *Server) logAsync(messageType protocol.MessageType, message string) {
	if !protocol.HasTraceMessageType(messageType) {
		return
	}
	s.SendAsync(protocol.ServerWindowLogMessage, &protocol.LogMessageParams{
		Type:    messageType,
		Message: message,
	})
}

// showAsync sends a window/showMessage via the retained connection, for use
// from background goroutines (glsp.Context notifies are only valid inside a
// handler).
func (s *Server) showAsync(messageType protocol.MessageType, message string) {
	s.SendAsync(protocol.ServerWindowShowMessage, &protocol.ShowMessageParams{
		Type:    messageType,
		Message: message,
	})
}

func (s *Server) logAt(messageType protocol.MessageType, message string, attrs []any) {
	if s.logger == nil {
		return
	}
	switch messageType {
	case protocol.MessageTypeError:
		s.logger.Error(message, attrs...)
	case protocol.MessageTypeWarning:
		s.logger.Warn(message, attrs...)
	case protocol.MessageTypeLog, protocol.MessageTypeInfo:
		s.logger.Info(message, attrs...)
	default:
		s.logger.Debug(message, attrs...)
	}
}

// reportPanic logs the recovered panic with its stack trace, sends a message to
// the client, and returns a normalized error so the caller can propagate it.
func (s *Server) reportPanic(ctx *glsp.Context, name string, recovered any) error {
	callstack := string(debug.Stack())
	msg := fmt.Sprintf("internal error in %s: %v", name, recovered)

	if s.logger != nil {
		s.logger.Error(msg, "stack", callstack, "handler", name)
	}
	s.notifyShow(ctx, protocol.MessageTypeError, "OpenChoreo language server hit an internal error in "+name+".")
	s.notifyLog(ctx, protocol.MessageTypeError, msg)

	return fmt.Errorf("internal error in %s: %v", name, recovered)
}

// WithRequestRecovery wraps a request handler so a panic is converted into an
// error response instead of crashing the server.
//
// It is exported so the tests in internal/lsp/tests can drive the wrapper
// directly; it is not part of the server's intended surface.
func WithRequestRecovery[P any, R any](s *Server, name string, fn func(ctx *glsp.Context, params *P) (R, error)) func(ctx *glsp.Context, params *P) (R, error) {
	return func(ctx *glsp.Context, params *P) (result R, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = *new(R)
				err = s.reportPanic(ctx, name, recovered)
			}
		}()
		return fn(ctx, params)
	}
}

// WithHandlerRecovery wraps a notification-style handler (returns only error)
// so a panic is caught and surfaced instead of crashing the server.
//
// Exported for the same reason as WithRequestRecovery.
func WithHandlerRecovery[P any](s *Server, name string, fn func(ctx *glsp.Context, params *P) error) func(ctx *glsp.Context, params *P) error {
	return func(ctx *glsp.Context, params *P) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = s.reportPanic(ctx, name, recovered)
			}
		}()
		return fn(ctx, params)
	}
}
