// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import "context"

// Middleware is the base interface all middleware must implement.
type Middleware interface {
	// Name returns a human-readable identifier for this middleware,
	// used in error messages and logging.
	Name() string
}

// ToolProvider is an optional interface middleware can implement to
// contribute additional tools to the agent. These tools are merged
// with the user-provided tools at agent creation time.
type ToolProvider interface {
	Middleware
	Tools() []Tool
}

// ModelCallMiddleware intercepts model invocations. The middleware receives
// the request and a next handler. It can inspect/modify the request, call
// next (possibly multiple times for retries), or short-circuit by returning
// a response without calling next.
type ModelCallMiddleware interface {
	Middleware
	WrapModelCall(ctx context.Context, req *ModelRequest, next ModelCallHandler) (*ModelResponse, error)
}

// ToolCallMiddleware intercepts tool executions. Same wrapping semantics
// as ModelCallMiddleware.
type ToolCallMiddleware interface {
	Middleware
	WrapToolCall(ctx context.Context, req *ToolCallRequest, next ToolCallHandler) (*ToolCallResponse, error)
}

// BeforeAgentHook runs once before the agent loop starts.
// It can modify state (e.g., inject messages).
type BeforeAgentHook interface {
	Middleware
	BeforeAgent(ctx context.Context, state *State) error
}

// AfterAgentHook runs once after the agent loop ends.
// It can inspect or modify the final state.
type AfterAgentHook interface {
	Middleware
	AfterAgent(ctx context.Context, state *State) error
}

// BeforeModelHook runs before each model call within the ReAct loop.
// The hook may set state.Done = true to exit the loop gracefully.
type BeforeModelHook interface {
	Middleware
	BeforeModel(ctx context.Context, state *State) error
}

// AfterModelHook runs after each model call within the ReAct loop.
// The hook may set state.Done = true to exit the loop gracefully.
type AfterModelHook interface {
	Middleware
	AfterModel(ctx context.Context, state *State) error
}

// chainModelCallMiddleware composes model call middleware into a single handler.
// The first middleware in the slice becomes the outermost wrapper:
// middlewares[0](middlewares[1](middlewares[2](..., innerHandler)))
func chainModelCallMiddleware(middlewares []ModelCallMiddleware, inner ModelCallHandler) ModelCallHandler {
	chain := inner
	for i := len(middlewares) - 1; i >= 0; i-- {
		chain = buildModelCallWrapper(middlewares[i], chain)
	}
	return chain
}

func buildModelCallWrapper(mw ModelCallMiddleware, next ModelCallHandler) ModelCallHandler {
	return func(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
		return mw.WrapModelCall(ctx, req, next)
	}
}

// chainToolCallMiddleware composes tool call middleware into a single handler.
// Same ordering semantics as chainModelCallMiddleware.
func chainToolCallMiddleware(middlewares []ToolCallMiddleware, inner ToolCallHandler) ToolCallHandler {
	chain := inner
	for i := len(middlewares) - 1; i >= 0; i-- {
		chain = buildToolCallWrapper(middlewares[i], chain)
	}
	return chain
}

func buildToolCallWrapper(mw ToolCallMiddleware, next ToolCallHandler) ToolCallHandler {
	return func(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error) {
		return mw.WrapToolCall(ctx, req, next)
	}
}
