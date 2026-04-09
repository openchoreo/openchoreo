// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openchoreo/openchoreo/internal/agent"
)

// ToolErrorHandler catches tool execution errors and returns them as
// error content to the model instead of failing the agent.
type ToolErrorHandler struct {
	logger *slog.Logger
}

func NewToolErrorHandler(logger *slog.Logger) *ToolErrorHandler {
	return &ToolErrorHandler{logger: logger}
}

func (m *ToolErrorHandler) Name() string { return "tool_error_handler" }

func (m *ToolErrorHandler) WrapToolCall(ctx context.Context, req *agent.ToolCallRequest, next agent.ToolCallHandler) (*agent.ToolCallResponse, error) {
	resp, err := next(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		m.logger.Warn("tool error",
			"tool", req.ToolCall.Function.Name,
			"error", err,
		)
		return &agent.ToolCallResponse{
			Content: fmt.Sprintf("Error: %v", err),
		}, nil
	}
	return resp, nil
}
