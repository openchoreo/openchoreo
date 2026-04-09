// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"

	"github.com/mozilla-ai/any-llm-go/providers"
)

// ModelCallHandler executes a model call and returns the response.
type ModelCallHandler func(ctx context.Context, req *ModelRequest) (*ModelResponse, error)

// ToolCallHandler executes a tool call and returns the response.
type ToolCallHandler func(ctx context.Context, req *ToolCallRequest) (*ToolCallResponse, error)

// ModelRequest contains the parameters for a model invocation.
// Middleware receives this and can inspect or modify it before forwarding.
//
// The system message is kept separate from Messages so that middleware
// can inspect the conversation without the system prompt, and the system
// prompt cannot be accidentally removed by middleware that modifies messages.
type ModelRequest struct {
	// Provider is the LLM provider being used.
	Provider providers.Provider
	// Model is the model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514").
	Model string
	// SystemMessage is prepended to Messages at model invocation time.
	// Kept separate so middleware sees messages without the system prompt.
	SystemMessage *providers.Message
	// Messages is the conversation history (excluding system prompt).
	Messages []providers.Message
	// Tools is the list of tools available to the model for this call.
	Tools []providers.Tool
	// ResponseFormat is set when using provider-native structured output.
	ResponseFormat *providers.ResponseFormat
	// ToolChoice controls which tool the model should call, if any.
	ToolChoice any
	// State is the current agent state, provided for middleware introspection.
	State *State
}

// ModelResponse contains the result of a model invocation.
type ModelResponse struct {
	// Message is the assistant message returned by the model.
	Message providers.Message
	// StructuredResponse holds the parsed structured output when using
	// provider-native strategy. Nil if no structured output was extracted.
	StructuredResponse json.RawMessage
}

// ToolCallRequest contains the parameters for a tool execution.
type ToolCallRequest struct {
	// ToolCall is the tool call from the model's response.
	ToolCall providers.ToolCall
	// Tool is the resolved tool definition with its execute function.
	Tool *Tool
	// State is the current agent state, provided for middleware introspection.
	State *State
}

// ToolCallResponse contains the result of a tool execution.
type ToolCallResponse struct {
	// Content is the string content returned by the tool, sent back to the model.
	Content string
}

// State holds the mutable agent state during execution.
// Middleware hooks receive this and may modify Messages.
type State struct {
	// Messages is the conversation history built up during the agent run.
	// Does NOT include the system prompt (that's on the Agent).
	Messages []providers.Message
	// StructuredResponse holds the structured output once obtained.
	// Nil until the agent produces structured output via either strategy.
	StructuredResponse json.RawMessage
	// Done can be set by middleware hooks to exit the loop gracefully.
	Done bool
}

// Result is the final output of an agent run.
type Result struct {
	// Messages is the complete conversation history from the run.
	Messages []providers.Message
	// StructuredResponse holds the structured output if configured.
	// Nil if no structured output was requested or produced.
	StructuredResponse json.RawMessage
}

// StreamEventType identifies the kind of streaming event.
type StreamEventType int

const (
	// StreamEventTextDelta is emitted for each text chunk from the model.
	StreamEventTextDelta StreamEventType = iota
	// StreamEventToolCallStart is emitted when the model begins a tool call
	// (tool name is known).
	StreamEventToolCallStart
	// StreamEventToolResult is emitted after a tool finishes executing.
	StreamEventToolResult
	// StreamEventComplete is emitted once when the agent finishes, carrying
	// the final Result.
	StreamEventComplete
)

// StreamEvent is a single event emitted during Agent.Stream().
type StreamEvent struct {
	// Type identifies what kind of event this is.
	Type StreamEventType
	// Delta is the text chunk (StreamEventTextDelta only).
	Delta string
	// ToolName is the tool name (StreamEventToolCallStart, StreamEventToolResult).
	ToolName string
	// ToolCallID is the provider-assigned tool call ID.
	ToolCallID string
	// Args is the tool call arguments (StreamEventToolCallStart only).
	// May be partial during streaming.
	Args string
	// Content is the tool execution result (StreamEventToolResult only).
	Content string
	// Result is the final agent result (StreamEventComplete only).
	Result *Result
}
