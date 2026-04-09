// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/mozilla-ai/any-llm-go/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/agent"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testToolCallRequest(name string) *agent.ToolCallRequest {
	return &agent.ToolCallRequest{
		ToolCall: providers.ToolCall{
			Function: providers.FunctionCall{Name: name},
		},
	}
}

func TestToolErrorHandler(t *testing.T) {
	t.Parallel()
	handler := NewToolErrorHandler(testLogger())

	t.Run("success passes through", func(t *testing.T) {
		t.Parallel()
		next := func(_ context.Context, _ *agent.ToolCallRequest) (*agent.ToolCallResponse, error) {
			return &agent.ToolCallResponse{Content: "ok"}, nil
		}
		resp, err := handler.WrapToolCall(context.Background(), testToolCallRequest("test"), next)
		require.NoError(t, err)
		assert.Equal(t, "ok", resp.Content)
	})

	t.Run("tool error converted to content", func(t *testing.T) {
		t.Parallel()
		next := func(_ context.Context, _ *agent.ToolCallRequest) (*agent.ToolCallResponse, error) {
			return nil, errors.New("connection refused")
		}
		resp, err := handler.WrapToolCall(context.Background(), testToolCallRequest("flaky"), next)
		require.NoError(t, err)
		assert.Contains(t, resp.Content, "connection refused")
	})

	t.Run("context canceled propagates", func(t *testing.T) {
		t.Parallel()
		next := func(_ context.Context, _ *agent.ToolCallRequest) (*agent.ToolCallResponse, error) {
			return nil, context.Canceled
		}
		_, err := handler.WrapToolCall(context.Background(), testToolCallRequest("test"), next)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("deadline exceeded propagates", func(t *testing.T) {
		t.Parallel()
		next := func(_ context.Context, _ *agent.ToolCallRequest) (*agent.ToolCallResponse, error) {
			return nil, context.DeadlineExceeded
		}
		_, err := handler.WrapToolCall(context.Background(), testToolCallRequest("test"), next)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("name returns correct value", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "tool_error_handler", handler.Name())
	})
}
